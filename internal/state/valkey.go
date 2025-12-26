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

func (s *ValkeyStore) GetLastPublishedAt(feedURL string) time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := s.client.Get(ctx, "feed:"+feedURL).Result()
	if err == redis.Nil {
		return time.Time{}
	} else if err != nil {
		// Log error? For interface simplicity we just return zero time, meaning "fetch all"
		// Ideally we should handle error, but for V2 let's stick to interface.
		// A potential improvement: Log via slog if we had access to logger here.
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (s *ValkeyStore) SetLastPublishedAt(feedURL string, t time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Store as RFC3339 string
	s.client.Set(ctx, "feed:"+feedURL, t.Format(time.RFC3339), 0)
}
