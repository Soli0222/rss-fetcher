package state

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ValkeyStore struct {
	client *redis.Client
}

func NewValkeyStore(addr, password string) (*ValkeyStore, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password, // no password set
		DB:       0,        // use default DB
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to valkey: %w", err)
	}

	return &ValkeyStore{client: rdb}, nil
}

func (s *ValkeyStore) GetLastPublishedAt(feedURL string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := s.client.Get(ctx, "feed:"+feedURL).Result()
	if err == redis.Nil {
		return time.Time{}, ErrNoState
	} else if err != nil {
		return time.Time{}, fmt.Errorf("valkey get failed: %w", err)
	}

	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid stored timestamp %q: %w", val, err)
	}
	return t, nil
}

func (s *ValkeyStore) SetLastPublishedAt(feedURL string, t time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Store as RFC3339 string
	s.client.Set(ctx, "feed:"+feedURL, t.Format(time.RFC3339), 0)
}
