package state

import (
	"errors"
	"sync"
	"time"
)

// ErrNoState is returned by Store.GetLastPublishedAt when no state has been
// recorded yet for the given feed. Other errors indicate a transient or
// data-integrity failure and must NOT be treated as "first run".
var ErrNoState = errors.New("no stored state for feed")

// Store defines the interface for keeping track of processed items.
// GetLastPublishedAt returns ErrNoState if no entry exists; any other
// non-nil error indicates a backend failure.
type Store interface {
	GetLastPublishedAt(feedURL string) (time.Time, error)
	SetLastPublishedAt(feedURL string, t time.Time)
}

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]time.Time),
	}
}

func (s *MemoryStore) GetLastPublishedAt(feedURL string) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[feedURL]
	if !ok {
		return time.Time{}, ErrNoState
	}
	return t, nil
}

func (s *MemoryStore) SetLastPublishedAt(feedURL string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Monotonic increase only? Not necessarily, but for this usecase usually yes.
	// But we just overwrite with the latest we successfully processed.
	s.data[feedURL] = t
}
