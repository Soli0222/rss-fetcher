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

// DiscordPayload represents the structure for Discord Webhooks
type DiscordPayload struct {
	Content string `json:"content"`
	// We could add embeds for fancier display, but content is safest for now.
}

// MisskeyPayload represents the structure for Misskey note creation
type MisskeyPayload struct {
	I          string `json:"i"`                    // API token
	Text       string `json:"text"`                 // Note content
	Visibility string `json:"visibility,omitempty"` // public, home, followers, specified
}

func (c *Client) SendWithRateLimit(ctx context.Context, wh config.Webhook, payload Payload) error {
	var body []byte
	var err error
	var url string = wh.URL

	switch wh.Provider {
	case "discord":
		// Format for Discord
		dp := DiscordPayload{
			Content: fmt.Sprintf("**%s**\n%s\n%s", payload.FeedTitle, payload.ItemTitle, payload.ItemURL),
		}
		body, err = json.Marshal(dp)
	case "misskey":
		// Format for Misskey - post as a note
		mp := MisskeyPayload{
			I:          wh.APIToken,
			Text:       fmt.Sprintf("%s\n%s\n%s", payload.FeedTitle, payload.ItemTitle, payload.ItemURL),
			Visibility: "public",
		}
		body, err = json.Marshal(mp)
		// Misskey API endpoint for creating notes
		url = wh.URL + "/api/notes/create"
	default:
		// Generic JSON
		body, err = json.Marshal(payload)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "rss-fetcher/1.2")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook responded with status: %d", resp.StatusCode)
	}

	// Rate Limit Wait
	if wh.PostInterval > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wh.PostInterval):
		}
	}

	return nil
}
