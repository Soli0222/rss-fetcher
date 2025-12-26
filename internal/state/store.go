package state

import (
	"sync"
	"time"
)

// Store defines the interface for keeping track of processed items
type Store interface {
	// IsNew checks if an item (identified by GUID or URL) has been seen before for a given feed.
	// Returns true if new, false if already processed.
	// Also marks it as seen if it is new.
	// Ideally we want to just Ask "GetLastPublishedAt" or "HaveWeSeen(guid)".
	// For RSS, typically we check if the item's PublishedDate > LastFetchedDate OR if we have not seen the GUID.
	// Let's go with: UpdateLastProcessed(feedURL string, itemGUID string, publishedAt time.Time) error
	// And Check(feedURL string, itemGUID string) bool
	// But simpler: Store the latest PublishedAt for the feed. Any item newer than that is new.
	// EXCEPT: if multiple items have same time.
	// Let's stick to the plan: "LastRead(feedURL) time.Time".
	// And maybe a dedupe cache for GUIDs if needed.
	// For simplicity V1: Store LastPublishedAt for each feed.
	GetLastPublishedAt(feedURL string) time.Time
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

func (s *MemoryStore) GetLastPublishedAt(feedURL string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[feedURL]
}

func (s *MemoryStore) SetLastPublishedAt(feedURL string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Monotonic increase only? Not necessarily, but for this usecase usually yes.
	// But we just overwrite with the latest we successfully processed.
	s.data[feedURL] = t
}
