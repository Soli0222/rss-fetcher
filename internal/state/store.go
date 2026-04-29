package state

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

const (
	StatusWarming = "warming"
	StatusReady   = "ready"
)

// ErrNoState is returned by Store.GetFeedState when no state has been
// recorded yet for the given feed. Other errors indicate a transient or
// data-integrity failure and must NOT be treated as "first run".
var ErrNoState = errors.New("no stored state for feed")

type FeedState struct {
	Version                  int       `json:"version"`
	Status                   string    `json:"status"`
	LastPublishedAt          time.Time `json:"last_published_at"`
	NotifyAfter              time.Time `json:"notify_after"`
	WarmupStableObservations int       `json:"warmup_stable_observations"`
}

// Store defines the interface for keeping track of processed items.
// GetFeedState returns ErrNoState if no entry exists; any other
// non-nil error indicates a backend failure.
type Store interface {
	GetFeedState(feedURL string) (FeedState, error)
	SetFeedState(feedURL string, state FeedState) error
}

func NewWarmingState(lastPublishedAt, notifyAfter time.Time) FeedState {
	return FeedState{
		Version:                  1,
		Status:                   StatusWarming,
		LastPublishedAt:          lastPublishedAt,
		NotifyAfter:              notifyAfter,
		WarmupStableObservations: 1,
	}
}

func NewReadyState(lastPublishedAt time.Time) FeedState {
	return NewReadyStateAfter(lastPublishedAt, time.Time{})
}

func NewReadyStateAfter(lastPublishedAt, notifyAfter time.Time) FeedState {
	return FeedState{
		Version:                  1,
		Status:                   StatusReady,
		LastPublishedAt:          lastPublishedAt,
		NotifyAfter:              notifyAfter,
		WarmupStableObservations: 0,
	}
}

func DecodeFeedState(raw string) (FeedState, error) {
	var st FeedState
	if err := json.Unmarshal([]byte(raw), &st); err == nil {
		if st.Version == 0 {
			st.Version = 1
		}
		if st.Status == "" {
			return FeedState{}, errors.New("missing feed state status")
		}
		if st.Status != StatusWarming && st.Status != StatusReady {
			return FeedState{}, errors.New("unknown feed state status")
		}
		if st.LastPublishedAt.IsZero() {
			return FeedState{}, errors.New("missing feed state baseline")
		}
		return st, nil
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return FeedState{}, err
	}
	return NewReadyState(t), nil
}

func EncodeFeedState(st FeedState) (string, error) {
	if st.Version == 0 {
		st.Version = 1
	}
	data, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]FeedState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]FeedState),
	}
}

func (s *MemoryStore) GetFeedState(feedURL string) (FeedState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.data[feedURL]
	if !ok {
		return FeedState{}, ErrNoState
	}
	return st, nil
}

func (s *MemoryStore) SetFeedState(feedURL string, st FeedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Version == 0 {
		st.Version = 1
	}
	s.data[feedURL] = st
	return nil
}
