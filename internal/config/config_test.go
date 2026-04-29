package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSupportsStringAndNamedFeeds(t *testing.T) {
	dir := t.TempDir()
	feedsPath := filepath.Join(dir, "feeds.yaml")
	webhooksPath := filepath.Join(dir, "webhooks.yaml")

	if err := os.WriteFile(feedsPath, []byte(`
feeds:
  - https://example.com/rss.xml
  - name: Example Blog
    url: https://example.com/blog.xml
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webhooksPath, []byte(`
webhooks:
  - name: test
    url: https://example.com/webhook
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(feedsPath, webhooksPath)
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.Feeds.Feeds[0].URL; got != "https://example.com/rss.xml" {
		t.Fatalf("first feed URL = %q", got)
	}
	if got := cfg.Feeds.Feeds[0].Label(); got != "https://example.com/rss.xml" {
		t.Fatalf("first feed label = %q", got)
	}
	if got := cfg.Feeds.Feeds[1].Name; got != "Example Blog" {
		t.Fatalf("second feed name = %q", got)
	}
	if got := cfg.Feeds.Feeds[1].Label(); got != "Example Blog" {
		t.Fatalf("second feed label = %q", got)
	}
}

func TestLoadRejectsNamedFeedWithoutURL(t *testing.T) {
	dir := t.TempDir()
	feedsPath := filepath.Join(dir, "feeds.yaml")
	webhooksPath := filepath.Join(dir, "webhooks.yaml")

	if err := os.WriteFile(feedsPath, []byte(`
feeds:
  - name: Missing URL
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webhooksPath, []byte(`
webhooks:
  - name: test
    url: https://example.com/webhook
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(feedsPath, webhooksPath); err == nil {
		t.Fatal("Load returned nil error for feed without URL")
	}
}
