package scenes

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"familydash/internal/familyapp"
)

// A restart reloads the last-good feed from the persistent cache, keeping
// the original fetch time so the data never looks fresher than it is.
func TestScenesCacheRestart(t *testing.T) {
	fixture := loadFeedFixture(t)
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	cache := filepath.Join(t.TempDir(), "scenes-cache.json")
	s := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", cache, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
	good, err := s.Snapshot()
	if err != nil {
		t.Fatalf("setup snapshot: %v", err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	// Restart: same backend and cache path, no refresh. Snapshot from disk.
	restarted := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", cache, nil)
	snap, err := restarted.Snapshot()
	if err != nil {
		t.Fatalf("restarted snapshot: %v", err)
	}
	if snap.Feed.Revision != 12 || len(snap.Feed.Scenes) != 3 {
		t.Errorf("restarted = rev %d, %d scenes", snap.Feed.Revision, len(snap.Feed.Scenes))
	}
	if !snap.FetchedAt.Equal(good.FetchedAt) {
		t.Errorf("restarted FetchedAt = %v, want %v", snap.FetchedAt, good.FetchedAt)
	}
}

// A cache written for another backend must be rejected, never reused.
func TestScenesWrongBackendCacheRejected(t *testing.T) {
	fixture := loadFeedFixture(t)
	srvA, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	cache := filepath.Join(t.TempDir(), "scenes-cache.json")
	a := NewService(familyapp.NewClient(srvA.URL, "", ""), "tok", cache, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.Refresh(ctx)
	if _, err := a.Snapshot(); err != nil {
		t.Fatalf("setup snapshot: %v", err)
	}

	other, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	b := NewService(familyapp.NewClient(other.URL, "", ""), "tok", cache, nil)
	if _, err := b.Snapshot(); err == nil {
		t.Error("foreign-backend cache should be rejected")
	}
}

// A failed or invalid response preserves the cache file byte for byte.
func TestScenesInvalidResponsePreservesCache(t *testing.T) {
	fixture := loadFeedFixture(t)
	body := append([]byte(nil), fixture...)
	status := http.StatusOK
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(body)
	})
	cache := filepath.Join(t.TempDir(), "scenes-cache.json")
	s := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", cache, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
	before, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	status = http.StatusServiceUnavailable
	s.Refresh(ctx)
	status = http.StatusOK
	body = []byte(`{"version":2}`)
	s.Refresh(ctx)

	after, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
	if string(after) != string(before) {
		t.Error("cache file changed after failed refreshes")
	}
}
