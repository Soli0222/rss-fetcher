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

func (s *ValkeyStore) GetFeedState(feedURL string) (FeedState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := s.client.Get(ctx, "feed:"+feedURL).Result()
	if err == redis.Nil {
		return FeedState{}, ErrNoState
	} else if err != nil {
		return FeedState{}, fmt.Errorf("valkey get failed: %w", err)
	}

	st, err := DecodeFeedState(val)
	if err != nil {
		return FeedState{}, fmt.Errorf("invalid stored feed state %q: %w", val, err)
	}
	return st, nil
}

func (s *ValkeyStore) SetFeedState(feedURL string, st FeedState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	encoded, err := EncodeFeedState(st)
	if err != nil {
		return fmt.Errorf("encode feed state failed: %w", err)
	}
	if err := s.client.Set(ctx, "feed:"+feedURL, encoded, 0).Err(); err != nil {
		return fmt.Errorf("valkey set failed: %w", err)
	}
	return nil
}
