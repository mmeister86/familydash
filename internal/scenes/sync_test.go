package scenes

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"familydash/internal/familyapp"
	"familydash/internal/scene"
)

func fallbackProvider(t *testing.T) *scene.Provider {
	t.Helper()
	school, err := scene.Build(func(id string) string { return scene.DefaultSchoolDay[id] })
	if err != nil {
		t.Fatal(err)
	}
	weekend, err := scene.Build(func(id string) string {
		if v, ok := scene.DefaultWeekend[id]; ok {
			return v
		}
		return scene.DefaultSchoolDay[id]
	})
	if err != nil {
		t.Fatal(err)
	}
	return scene.NewProvider(scene.Schedule{SchoolDay: school, Weekend: weekend})
}

// Refresh pushes accepted feeds into the provider: the wall follows the feed.
func TestRefreshSyncsProvider(t *testing.T) {
	fixture := loadFeedFixture(t)
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	loc, _ := time.LoadLocation("Europe/Berlin")
	provider := fallbackProvider(t)
	s := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", filepath.Join(t.TempDir(), "cache.json"), provider)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)

	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 16:00", loc)
	if got := provider.At(mon, loc).ID; got != "s-k3f9q2" {
		t.Errorf("provider after sync = %q, want s-k3f9q2", got)
	}
}

// Failures keep the last synced schedule; they never revert to SCENE_*.
func TestFailedRefreshKeepsProvider(t *testing.T) {
	fixture := loadFeedFixture(t)
	body := append([]byte(nil), fixture...)
	status := http.StatusOK
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(body)
	})
	loc, _ := time.LoadLocation("Europe/Berlin")
	provider := fallbackProvider(t)
	s := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", filepath.Join(t.TempDir(), "cache.json"), provider)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)

	status = http.StatusServiceUnavailable
	s.Refresh(ctx)
	body = []byte(`{"version":2}`)
	s.Refresh(ctx)

	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 16:00", loc)
	if got := provider.At(mon, loc).ID; got != "s-k3f9q2" {
		t.Errorf("provider after failures = %q, want s-k3f9q2", got)
	}
}

// A restart preloads the provider from the cache file.
func TestCacheRestartSyncsProvider(t *testing.T) {
	fixture := loadFeedFixture(t)
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	cache := filepath.Join(t.TempDir(), "cache.json")
	s := NewService(familyapp.NewClient(srv.URL, "", ""), "tok", cache, fallbackProvider(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)

	loc, _ := time.LoadLocation("Europe/Berlin")
	restarted := fallbackProvider(t)
	_ = NewService(familyapp.NewClient(srv.URL, "", ""), "tok", cache, restarted)
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 16:00", loc)
	if got := restarted.At(mon, loc).ID; got != "s-k3f9q2" {
		t.Errorf("restarted provider = %q, want s-k3f9q2", got)
	}
}
