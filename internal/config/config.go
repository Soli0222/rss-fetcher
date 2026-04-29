package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type FeedsConfig struct {
	Feeds                           []Feed        `yaml:"feeds"`
	Interval                        time.Duration `yaml:"interval"`
	Store                           StoreConfig   `yaml:"store"`
	SkipInitialNotify               bool          `yaml:"skip_initial_notify"`
	InitialWarmupStableObservations int           `yaml:"initial_warmup_stable_observations"`
	MaxNotificationsPerFeedPerRun   int           `yaml:"max_notifications_per_feed_per_run"`
}

type Feed struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

func (f Feed) Label() string {
	if f.Name != "" {
		return f.Name
	}
	return f.URL
}

func (f *Feed) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var url string
		if err := value.Decode(&url); err != nil {
			return err
		}
		f.URL = url
		return nil
	case yaml.MappingNode:
		type feed Feed
		var decoded feed
		if err := value.Decode(&decoded); err != nil {
			return err
		}
		*f = Feed(decoded)
		return nil
	default:
		return fmt.Errorf("feed must be a URL string or mapping with url/name")
	}
}

type StoreConfig struct {
	Type     string `yaml:"type"` // "memory" or "valkey"
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
}

type WebhooksConfig struct {
	Webhooks []Webhook `yaml:"webhooks"`
}

type Webhook struct {
	Name         string        `yaml:"name"`
	URL          string        `yaml:"url"`
	Provider     string        `yaml:"provider"` // "generic" (default), "discord", or "misskey"
	PostInterval time.Duration `yaml:"post_interval"`
	APIToken     string        `yaml:"api_token"` // Required for misskey
}

type AppConfig struct {
	Feeds    *FeedsConfig
	Webhooks *WebhooksConfig
}

func Load(feedsPath, webhooksPath string) (*AppConfig, error) {
	// Defaults
	c := &AppConfig{
		Feeds: &FeedsConfig{
			Interval:                        10 * time.Minute,
			SkipInitialNotify:               true,
			InitialWarmupStableObservations: 2,
			MaxNotificationsPerFeedPerRun:   10,
			Store: StoreConfig{
				Type: "memory",
			},
		},
		Webhooks: &WebhooksConfig{},
	}

	// Load Feeds
	if err := loadYaml(feedsPath, c.Feeds); err != nil {
		return nil, fmt.Errorf("failed to load feeds config: %w", err)
	}

	// Load Webhooks
	if err := loadYaml(webhooksPath, c.Webhooks); err != nil {
		return nil, fmt.Errorf("failed to load webhooks config: %w", err)
	}

	if len(c.Feeds.Feeds) == 0 {
		return nil, fmt.Errorf("no feeds configured")
	}
	for i, feed := range c.Feeds.Feeds {
		if feed.URL == "" {
			return nil, fmt.Errorf("feeds[%d].url is required", i)
		}
	}
	if len(c.Webhooks.Webhooks) == 0 {
		return nil, fmt.Errorf("no webhooks configured")
	}
	if c.Feeds.InitialWarmupStableObservations < 1 {
		return nil, fmt.Errorf("initial_warmup_stable_observations must be >= 1")
	}
	if c.Feeds.MaxNotificationsPerFeedPerRun < 0 {
		return nil, fmt.Errorf("max_notifications_per_feed_per_run must be >= 0")
	}

	// Set default provider
	for i := range c.Webhooks.Webhooks {
		if c.Webhooks.Webhooks[i].Provider == "" {
			c.Webhooks.Webhooks[i].Provider = "generic"
		}
	}

	return c, nil
}

func loadYaml(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
