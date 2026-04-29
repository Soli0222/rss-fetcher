package state

import (
	"testing"
	"time"
)

func TestDecodeFeedStateMigratesLegacyTimestamp(t *testing.T) {
	ts := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	st, err := DecodeFeedState(ts.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}

	if st.Status != StatusReady {
		t.Fatalf("status = %q, want %q", st.Status, StatusReady)
	}
	if !st.LastPublishedAt.Equal(ts) {
		t.Fatalf("last published at = %s, want %s", st.LastPublishedAt, ts)
	}
}

func TestDecodeFeedStateRejectsMalformedJSONState(t *testing.T) {
	if _, err := DecodeFeedState(`{"version":1}`); err == nil {
		t.Fatal("DecodeFeedState returned nil error for malformed JSON state")
	}
}
