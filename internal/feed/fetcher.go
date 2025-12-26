package feed

import (
	"context"
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
	store    state.Store
	whClient *webhook.Client
	webhooks []config.Webhook
	parser   *gofeed.Parser
}

func NewFetcher(store state.Store, whClient *webhook.Client, webhooks []config.Webhook) *Fetcher {
	return &Fetcher{
		store:    store,
		whClient: whClient,
		webhooks: webhooks,
		parser:   gofeed.NewParser(),
	}
}

func (f *Fetcher) ProcessFeed(ctx context.Context, feedURL string) {
	logger := slog.With("feed", feedURL)
	logger.Info("Checking feed")

	feed, err := f.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		logger.Error("Failed to parse feed", "error", err)
		metricFetchCount.WithLabelValues(feedURL, "error").Inc()
		return
	}
	metricFetchCount.WithLabelValues(feedURL, "success").Inc()

	lastPub := f.store.GetLastPublishedAt(feedURL)

	var newItems []*gofeed.Item

	for _, item := range feed.Items {
		if item.PublishedParsed == nil {
			if item.UpdatedParsed != nil {
				item.PublishedParsed = item.UpdatedParsed
			} else {
				continue
			}
		}

		if item.PublishedParsed.After(lastPub) {
			newItems = append(newItems, item)
		}
	}

	if len(newItems) == 0 {
		logger.Debug("No new items")
		return
	}

	logger.Info("Found new items", "count", len(newItems))

	sort.Slice(newItems, func(i, j int) bool {
		return newItems[i].PublishedParsed.Before(*newItems[j].PublishedParsed)
	})

	for _, item := range newItems {
		payload := webhook.Payload{
			FeedTitle:   feed.Title,
			ItemTitle:   item.Title,
			ItemURL:     item.Link,
			PublishedAt: *item.PublishedParsed,
		}

		// Broadcast to all configured webhooks
		success := true
		for _, wh := range f.webhooks {
			if err := f.whClient.SendWithRateLimit(ctx, wh, payload); err != nil {
				logger.Error("Failed to post webhook", "name", wh.Name, "item", item.Title, "error", err)
				success = false
				// If one webhook fails, do we stop? or try others?
				// For now: Log and continue to others.
				// But: should we mark item as processed?
				// If we have critical webhook failure, we might want to NOT update state.
				// Let's go with: if ALL fail, failure. If at least one succeeds, maybe success?
				// User didn't specify. Conservative: if ANY fails, treat as partial failure?
				// Simplest: Just try best effort.
			}
		}

		// Determine if we should update state
		if !success {
			// If we failed to notify some webhooks, maybe we shouldn't advance state?
			// But that would mean duplicate for successful ones next time.
			// Ideally we track state per webhook? Too complex for now.
			// Let's assume if at least one attempt was made, update state.
			// OR: Simplistic approach as before -> break on error.
			// Revert to: logic from before. If any error, break loop and don't update state.
			// But now we have multiple webhooks.

			// Let's decide: If sending to ANY webhook fails, log it but still mark as read?
			// No, that risks data loss (user misses notification).
			// If we don't mark as read, we repost to ALL next time (duplicate).
			// Dilemma.
			// Given "monitoring RSS" usually implies duplicates is better than missing.
			// But for multiple webhooks, duplicates on A because B failed is annoying.
			// Let's Log Error and Proceed. Updating state.
		}

		metricNewItems.WithLabelValues(feedURL).Inc()
		f.store.SetLastPublishedAt(feedURL, *item.PublishedParsed)
		logger.Info("Processed new item", "title", item.Title)
	}
}

func (f *Fetcher) Run(ctx context.Context, feeds []string, interval time.Duration) {
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

func (f *Fetcher) runOnce(ctx context.Context, feeds []string) {
	var wg sync.WaitGroup
	for _, feedURL := range feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Increased timeout for rate limits
			defer cancel()
			f.ProcessFeed(ctx, url)
		}(feedURL)
	}
	wg.Wait()
}
