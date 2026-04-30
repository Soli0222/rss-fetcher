package feed

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"rss-fetcher/internal/config"
	"rss-fetcher/internal/state"
	"rss-fetcher/internal/webhook"
)

var (
	metricFetchCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rss_fetch_count_total",
		Help: "The total number of feed fetches",
	}, []string{"feed", "status"})

	metricNewItems = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rss_new_items_total",
		Help: "The total number of new items found",
	}, []string{"feed"})
)

type Fetcher struct {
	store                           state.Store
	whClient                        *webhook.Client
	webhooks                        []config.Webhook
	parser                          *gofeed.Parser
	skipInitialNotify               bool
	initialWarmupStableObservations int
	maxNotificationsPerFeedPerRun   int
}

func NewFetcher(store state.Store, whClient *webhook.Client, webhooks []config.Webhook, feedsConfig *config.FeedsConfig) *Fetcher {
	return &Fetcher{
		store:                           store,
		whClient:                        whClient,
		webhooks:                        webhooks,
		parser:                          gofeed.NewParser(),
		skipInitialNotify:               feedsConfig.SkipInitialNotify,
		initialWarmupStableObservations: feedsConfig.InitialWarmupStableObservations,
		maxNotificationsPerFeedPerRun:   feedsConfig.MaxNotificationsPerFeedPerRun,
	}
}

func (f *Fetcher) ProcessFeed(ctx context.Context, feedConfig config.Feed) {
	feedURL := feedConfig.URL
	feedLabel := feedConfig.Label()
	logger := slog.With("feed", feedLabel, "feed_url", feedURL)
	logger.Info("Checking feed")

	feed, err := f.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		logger.Error("Failed to parse feed", "error", err)
		metricFetchCount.WithLabelValues(feedLabel, "error").Inc()
		return
	}
	metricFetchCount.WithLabelValues(feedLabel, "success").Inc()

	feedState, stateErr := f.store.GetFeedState(feedURL)
	if stateErr != nil && !errors.Is(stateErr, state.ErrNoState) {
		logger.Error("Failed to read feed state; skipping notification because baseline is not comparable", "error", stateErr)
		return
	}

	items := itemsWithPublishedTime(feed.Items)
	if len(items) == 0 {
		logger.Debug("No comparable items")
		return
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].PublishedParsed.Before(*items[j].PublishedParsed)
	})
	latest := *items[len(items)-1].PublishedParsed

	if errors.Is(stateErr, state.ErrNoState) {
		if !f.skipInitialNotify {
			feedState = state.NewReadyState(time.Time{})
		} else {
			feedState = state.NewWarmingState(latest, time.Now())
			if err := f.store.SetFeedState(feedURL, feedState); err != nil {
				logger.Error("Failed to record initial warming state", "error", err)
			} else {
				logger.Info("Starting feed warmup; notification skipped", "latest", latest, "stable_observations", feedState.WarmupStableObservations)
			}
			return
		}
	}

	if feedState.Status == state.StatusWarming {
		f.processWarmingFeed(feedURL, logger, feedState, latest)
		return
	}

	if feedState.Status != state.StatusReady {
		logger.Error("Unknown feed state status; skipping notification because baseline is not comparable", "status", feedState.Status)
		return
	}

	newItems := itemsAfter(items, feedState.LastPublishedAt, feedState.NotifyAfter)
	if len(newItems) == 0 {
		logger.Debug("No new items")
		return
	}

	if f.maxNotificationsPerFeedPerRun > 0 && len(newItems) > f.maxNotificationsPerFeedPerRun {
		nextState := state.NewReadyStateAfter(*newItems[len(newItems)-1].PublishedParsed, feedState.NotifyAfter)
		if err := f.store.SetFeedState(feedURL, nextState); err != nil {
			logger.Error("Failed to advance state after suppressing notification burst", "error", err, "count", len(newItems))
			return
		}
		logger.Warn("Suppressed notification burst and advanced baseline", "count", len(newItems), "limit", f.maxNotificationsPerFeedPerRun, "latest", nextState.LastPublishedAt)
		return
	}

	logger.Info("Found new items", "count", len(newItems))

	for _, item := range newItems {
		payload := webhook.Payload{
			FeedTitle:   feed.Title,
			ItemTitle:   item.Title,
			ItemURL:     item.Link,
			PublishedAt: *item.PublishedParsed,
		}

		for _, wh := range f.webhooks {
			if err := f.whClient.SendWithRateLimit(ctx, wh, payload); err != nil {
				logger.Error("Failed to post webhook", "name", wh.Name, "item", item.Title, "error", err)
			}
		}

		metricNewItems.WithLabelValues(feedLabel).Inc()
		nextState := state.NewReadyStateAfter(*item.PublishedParsed, feedState.NotifyAfter)
		if err := f.store.SetFeedState(feedURL, nextState); err != nil {
			logger.Error("Failed to update feed state after notification", "error", err, "title", item.Title)
			return
		}
		logger.Info("Processed new item", "title", item.Title)
	}
}

func (f *Fetcher) processWarmingFeed(feedURL string, logger *slog.Logger, feedState state.FeedState, latest time.Time) {
	if latest.After(feedState.LastPublishedAt) {
		feedState.LastPublishedAt = latest
		feedState.WarmupStableObservations = 1
	} else {
		feedState.WarmupStableObservations++
	}

	if feedState.WarmupStableObservations >= f.initialWarmupStableObservations {
		feedState = state.NewReadyStateAfter(feedState.LastPublishedAt, feedState.NotifyAfter)
		if err := f.store.SetFeedState(feedURL, feedState); err != nil {
			logger.Error("Failed to mark feed warmup complete", "error", err)
			return
		}
		logger.Info("Feed warmup complete; baseline ready", "latest", feedState.LastPublishedAt)
		return
	}

	if err := f.store.SetFeedState(feedURL, feedState); err != nil {
		logger.Error("Failed to update feed warmup state", "error", err)
		return
	}
	logger.Info("Feed warmup continuing; notification skipped", "latest", feedState.LastPublishedAt, "stable_observations", feedState.WarmupStableObservations, "required", f.initialWarmupStableObservations)
}

func itemsWithPublishedTime(items []*gofeed.Item) []*gofeed.Item {
	out := make([]*gofeed.Item, 0, len(items))
	for _, item := range items {
		if item.PublishedParsed == nil {
			if item.UpdatedParsed == nil {
				continue
			}
			item.PublishedParsed = item.UpdatedParsed
		}
		out = append(out, item)
	}
	return out
}

func itemsAfter(items []*gofeed.Item, baseline, notifyAfter time.Time) []*gofeed.Item {
	out := make([]*gofeed.Item, 0, len(items))
	for _, item := range items {
		if item.PublishedParsed.After(baseline) && (notifyAfter.IsZero() || item.PublishedParsed.After(notifyAfter)) {
			out = append(out, item)
		}
	}
	return out
}

func (f *Fetcher) Run(ctx context.Context, feeds []config.Feed, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	f.runOnce(ctx, feeds)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.runOnce(ctx, feeds)
		}
	}
}

func (f *Fetcher) runOnce(ctx context.Context, feeds []config.Feed) {
	var wg sync.WaitGroup
	for _, feedConfig := range feeds {
		wg.Add(1)
		go func(feedConfig config.Feed) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Increased timeout for rate limits
			defer cancel()
			f.ProcessFeed(ctx, feedConfig)
		}(feedConfig)
	}
	wg.Wait()
}
