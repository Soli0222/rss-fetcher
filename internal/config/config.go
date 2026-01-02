package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type FeedsConfig struct {
	Feeds    []string      `yaml:"feeds"`
	Interval time.Duration `yaml:"interval"`
	Store    StoreConfig   `yaml:"store"`
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
			Interval: 10 * time.Minute,
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
	if len(c.Webhooks.Webhooks) == 0 {
		return nil, fmt.Errorf("no webhooks configured")
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
