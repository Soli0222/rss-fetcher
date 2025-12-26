package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"rss-fetcher/internal/config"
	"rss-fetcher/internal/feed"
	"rss-fetcher/internal/state"
	"rss-fetcher/internal/webhook"
)

func main() {
	feedsPath := flag.String("feeds", "config/feeds.yaml", "Path to feeds configuration file")
	webhooksPath := flag.String("webhooks", "config/webhooks.yaml", "Path to webhooks configuration file")
	flag.Parse()

	// Setup Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load Config
	cfg, err := config.Load(*feedsPath, *webhooksPath)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Init Store
	var store state.Store
	if cfg.Feeds.Store.Type == "valkey" {
		logger.Info("Using Valkey Store", "address", cfg.Feeds.Store.Address)
		s, err := state.NewValkeyStore(cfg.Feeds.Store.Address, cfg.Feeds.Store.Password)
		if err != nil {
			logger.Error("Failed to initialize Valkey store", "error", err)
			os.Exit(1)
		}
		store = s
	} else {
		logger.Info("Using Memory Store")
		store = state.NewMemoryStore()
	}

	// Init Components
	whClient := webhook.NewClient()
	fetcher := feed.NewFetcher(store, whClient, cfg.Webhooks.Webhooks)

	// Metrics Server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logger.Info("Starting metrics server on :9090")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			logger.Error("Metrics server failed", "error", err)
		}
	}()

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down...")
		cancel()
	}()

	// Run Fetcher
	logger.Info("Starting RSS Fetcher",
		"interval", cfg.Feeds.Interval,
		"feeds", len(cfg.Feeds.Feeds),
		"webhooks", len(cfg.Webhooks.Webhooks))

	fetcher.Run(ctx, cfg.Feeds.Feeds, cfg.Feeds.Interval)
	logger.Info("Fetcher stopped")
}
