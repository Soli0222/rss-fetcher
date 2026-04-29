package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"rss-fetcher/internal/config"
	"rss-fetcher/internal/state"
	"rss-fetcher/internal/webhook"
)

func TestWarmupSuppressesItemsPublishedBeforeWarmup(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	lateVisible := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	newItem := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

	var rss atomic.Value
	rss.Store(rssFeed([]rssItem{{Title: "old", PublishedAt: old}}))

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rss.Load().(string))
	}))
	defer feedServer.Close()

	var webhookCalls atomic.Int64
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	store := state.NewMemoryStore()
	fetcher := NewFetcher(store, webhook.NewClient(), []config.Webhook{{
		Name: "test",
		URL:  webhookServer.URL,
	}}, &config.FeedsConfig{
		SkipInitialNotify:               true,
		InitialWarmupStableObservations: 2,
		MaxNotificationsPerFeedPerRun:   10,
	})

	feedConfig := config.Feed{URL: feedServer.URL}

	fetcher.ProcessFeed(context.Background(), feedConfig)
	fetcher.ProcessFeed(context.Background(), feedConfig)

	rss.Store(rssFeed([]rssItem{
		{Title: "old", PublishedAt: old},
		{Title: "late-visible", PublishedAt: lateVisible},
	}))
	fetcher.ProcessFeed(context.Background(), feedConfig)

	if got := webhookCalls.Load(); got != 0 {
		t.Fatalf("webhook calls after late-visible historical item = %d, want 0", got)
	}

	rss.Store(rssFeed([]rssItem{
		{Title: "old", PublishedAt: old},
		{Title: "late-visible", PublishedAt: lateVisible},
		{Title: "new", PublishedAt: newItem},
	}))
	fetcher.ProcessFeed(context.Background(), feedConfig)

	if got := webhookCalls.Load(); got != 1 {
		t.Fatalf("webhook calls after new item = %d, want 1", got)
	}
}

func TestBurstSuppressionAdvancesBaselineWithoutNotifications(t *testing.T) {
	baseline := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	items := []rssItem{
		{Title: "one", PublishedAt: baseline.Add(1 * time.Minute)},
		{Title: "two", PublishedAt: baseline.Add(2 * time.Minute)},
		{Title: "three", PublishedAt: baseline.Add(3 * time.Minute)},
	}

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssFeed(items))
	}))
	defer feedServer.Close()

	var webhookCalls atomic.Int64
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	store := state.NewMemoryStore()
	if err := store.SetFeedState(feedServer.URL, state.NewReadyState(baseline)); err != nil {
		t.Fatal(err)
	}

	fetcher := NewFetcher(store, webhook.NewClient(), []config.Webhook{{
		Name: "test",
		URL:  webhookServer.URL,
	}}, &config.FeedsConfig{
		InitialWarmupStableObservations: 2,
		MaxNotificationsPerFeedPerRun:   2,
	})

	fetcher.ProcessFeed(context.Background(), config.Feed{URL: feedServer.URL})

	if got := webhookCalls.Load(); got != 0 {
		t.Fatalf("webhook calls after suppressed burst = %d, want 0", got)
	}

	st, err := store.GetFeedState(feedServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !st.LastPublishedAt.Equal(items[len(items)-1].PublishedAt) {
		t.Fatalf("baseline = %s, want %s", st.LastPublishedAt, items[len(items)-1].PublishedAt)
	}
}

func TestConfiguredFeedNameIsUsedInWebhookPayload(t *testing.T) {
	baseline := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	newItem := baseline.Add(1 * time.Minute)

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssFeed([]rssItem{{Title: "new", PublishedAt: newItem}}))
	}))
	defer feedServer.Close()

	payloads := make(chan webhook.DiscordPayload, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhook.DiscordPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode webhook payload: %v", err)
		}
		payloads <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhookServer.Close()

	store := state.NewMemoryStore()
	if err := store.SetFeedState(feedServer.URL, state.NewReadyState(baseline)); err != nil {
		t.Fatal(err)
	}

	fetcher := NewFetcher(store, webhook.NewClient(), []config.Webhook{{
		Name:     "test",
		URL:      webhookServer.URL,
		Provider: "discord",
	}}, &config.FeedsConfig{
		InitialWarmupStableObservations: 2,
		MaxNotificationsPerFeedPerRun:   10,
	})

	fetcher.ProcessFeed(context.Background(), config.Feed{Name: "Release Notes", URL: feedServer.URL})

	select {
	case got := <-payloads:
		want := "**Release Notes**\nnew\nhttps://example.com/0"
		if got.Content != want {
			t.Fatalf("webhook content = %q, want %q", got.Content, want)
		}
	case <-time.After(time.Second):
		t.Fatal("webhook was not called")
	}
}

type rssItem struct {
	Title       string
	PublishedAt time.Time
}

func rssFeed(items []rssItem) string {
	out := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>test feed</title>`
	for i, item := range items {
		out += fmt.Sprintf(
			`<item><title>%s</title><link>https://example.com/%d</link><guid>https://example.com/%d</guid><pubDate>%s</pubDate></item>`,
			item.Title,
			i,
			i,
			item.PublishedAt.Format(time.RFC1123Z),
		)
	}
	return out + `</channel></rss>`
}
