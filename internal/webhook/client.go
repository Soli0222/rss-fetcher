package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"rss-fetcher/internal/config"
)

type Payload struct {
	FeedTitle   string    `json:"feed_title"`
	ItemTitle   string    `json:"item_title"`
	ItemURL     string    `json:"item_url"`
	PublishedAt time.Time `json:"published_at"`
}

type Client struct {
	client *http.Client
}

func NewClient() *Client {
	return &Client{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Post sends the payload to a specific webhook URL.
func (c *Client) Post(ctx context.Context, url string, payload Payload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "rss-fetcher/1.1")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook responded with status: %d", resp.StatusCode)
	}

	return nil
}

// Manager handles distributing posts to multiple webhooks with rate limiting.
type Manager struct {
	client   *Client
	webhooks []config.Webhook
	queues   map[string]chan Payload
}

// Actually, simplistic approach: The Fetcher calls "Broadcast" or "Notify" and this manager handles the loop?
// Or: Fetcher calls Post, and Post iterates?
// If we want Rate Limit, we need sequential processing per webhook.

// Let's implement a Broadcast method that takes payload, and for each webhook, sends it.
// To handle rate limit, simple `time.Sleep` after post is blocking if we do it inline.
// But we want to block the fetcher loop? "webhookにpostするとき、レートリミットに引っかかる可能性があります。postするときのインターバルを設定できるようにしたいです"
// This implies we should slow down posting.
// If we have 10 items, and interval is 1s, it takes 10s to process that feed. That is acceptable.

func (c *Client) SendWithRateLimit(ctx context.Context, wh config.Webhook, payload Payload) error {
	// Just post. Rate limit is handled by caller (Fetcher) or we wrap it here?
	// User asked "postするときのインターバルを設定できるようにしたいです" (I want to be able to set the interval when posting).
	// If we just sleep AFTER post, that satisfies it.

	if err := c.Post(ctx, wh.URL, payload); err != nil {
		return err
	}

	if wh.PostInterval > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wh.PostInterval):
		}
	}
	return nil
}
