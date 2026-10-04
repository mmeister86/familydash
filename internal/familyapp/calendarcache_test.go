package familyapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A restart reloads the last-good snapshot from the persistent cache,
// keeping the providers' original success times.
func TestCacheRestart(t *testing.T) {
	fixture := loadFixture(t)
	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	})
	cache := filepath.Join(t.TempDir(), "calendar-cache.json")
	c := NewClient(srv.URL, "", "")
	s := NewCalendarService(c, "tok", cache, berlin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
	if n := len(s.Snapshot().Events); n != 3 {
		t.Fatalf("events = %d, want 3", n)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	// Restart: backend gone, same cache path. No Refresh, snapshot from disk.
	restarted := NewCalendarService(NewClient(srv.URL, "", ""), "tok", cache, berlin)
	snap := restarted.Snapshot()
	if len(snap.Events) != 3 || len(snap.Calendars) != 2 {
		t.Fatalf("restarted snapshot: %d events %d calendars, want 3/2", len(snap.Events), len(snap.Calendars))
	}
	if snap.UpdatedAt.UnixMilli() != 1791028680000 {
		t.Errorf("restarted UpdatedAt = %v, want original provider success time", snap.UpdatedAt)
	}
	if !snap.Central || snap.ConfigurationRevision != 3 {
		t.Errorf("restarted snapshot lost central metadata: %+v", snap)
	}
}

// A cache written for another backend must be rejected, never reused.
func TestWrongBackendCacheRejected(t *testing.T) {
	fixture := loadFixture(t)
	srvA, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	cache := filepath.Join(t.TempDir(), "calendar-cache.json")
	a := NewCalendarService(NewClient(srvA.URL, "", ""), "tok", cache, berlin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.Refresh(ctx)
	if n := len(a.Snapshot().Events); n != 3 {
		t.Fatalf("setup: events = %d", n)
	}

	other, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	b := NewCalendarService(NewClient(other.URL, "", ""), "tok", cache, berlin)
	snap := b.Snapshot()
	if len(snap.Events) != 0 || len(snap.Calendars) != 0 {
		t.Errorf("foreign-backend cache reused: %d events %d calendars", len(snap.Events), len(snap.Calendars))
	}
	if len(snap.Errors) == 0 {
		t.Error("rejected cache should surface a visible unavailable state")
	}
}

// An invalid response preserves both the RAM snapshot and the cache file.
func TestInvalidResponsePreservesCache(t *testing.T) {
	fixture := loadFixture(t)
	var body = append([]byte(nil), fixture...)
	var status = http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "boom", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	cache := filepath.Join(t.TempDir(), "calendar-cache.json")
	s := NewCalendarService(NewClient(srv.URL, "", ""), "tok", cache, berlin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
	before := s.Snapshot()
	cached, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}

	for name, bad := range map[string][]byte{
		"not json":       []byte("this is no json"),
		"empty object":   []byte("{}"),
		"missing arrays": []byte(`{"version":1,"scope":"family-calendars"}`),
		"http error":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			if bad == nil {
				status = http.StatusInternalServerError
			} else {
				status = http.StatusOK
				body = append([]byte(nil), bad...)
			}
			s.Refresh(ctx)
			after := s.Snapshot()
			if len(after.Events) != len(before.Events) || after.UpdatedAt.UnixMilli() != before.UpdatedAt.UnixMilli() {
				t.Errorf("invalid response replaced last-good: %d events vs %d", len(after.Events), len(before.Events))
			}
			if now, _ := os.ReadFile(cache); string(now) != string(cached) {
				t.Error("invalid response rewrote the cache file")
			}
		})
	}
}

// A cache write failure warns but keeps the accepted RAM data.
func TestCacheWriteFailureKeepsRAM(t *testing.T) {
	fixture := loadFixture(t)
	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	// Parent directory does not exist: persisting must fail.
	cache := filepath.Join(t.TempDir(), "no-such-dir", "calendar-cache.json")
	s := NewCalendarService(NewClient(srv.URL, "", ""), "tok", cache, berlin)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
	snap := s.Snapshot()
	if len(snap.Events) != 3 {
		t.Errorf("write failure discarded RAM data: %d events", len(snap.Events))
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Errorf("cache file should not exist, stat err = %v", err)
	}
}
