package scenes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"familydash/internal/familyapp"
)

// scenesServer serves scenesHandler and records the Authorization header.
func scenesServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *string) {
	t.Helper()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/dashboard/scenes" {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &gotAuth
}

func scenesService(t *testing.T, srv *httptest.Server, token string) *Service {
	t.Helper()
	c := familyapp.NewClient(srv.URL, "", "")
	return NewService(c, token, filepath.Join(t.TempDir(), "scenes-cache.json"))
}

func refreshScenes(t *testing.T, s *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
}

func TestRefreshServesFeed(t *testing.T) {
	fixture := loadFeedFixture(t)
	srv, gotAuth := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	})
	s := scenesService(t, srv, "tok")
	refreshScenes(t, s)

	if *gotAuth != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", *gotAuth)
	}
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Feed.Revision != 12 || len(snap.Feed.Scenes) != 3 {
		t.Errorf("snapshot = rev %d, %d scenes", snap.Feed.Revision, len(snap.Feed.Scenes))
	}
	if snap.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set")
	}
}

func TestRefreshKeepsLastGood(t *testing.T) {
	fixture := loadFeedFixture(t)
	body := append([]byte(nil), fixture...)
	status := http.StatusOK
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(body)
	})
	s := scenesService(t, srv, "tok")
	refreshScenes(t, s)
	good, err := s.Snapshot()
	if err != nil {
		t.Fatalf("setup snapshot: %v", err)
	}

	status = http.StatusInternalServerError
	refreshScenes(t, s)
	status = http.StatusOK
	body = []byte("{broken")
	refreshScenes(t, s)
	body = []byte(`{"version":2,"revision":1,"grid":{"cols":12,"rows":24},"scenes":[]}`)
	refreshScenes(t, s)

	snap, err := s.Snapshot()
	if err != nil {
		t.Fatalf("last-good snapshot: %v", err)
	}
	if snap.Feed.Revision != good.Feed.Revision || !snap.FetchedAt.Equal(good.FetchedAt) {
		t.Errorf("last-good changed: rev %d at %v", snap.Feed.Revision, snap.FetchedAt)
	}
}

func TestSnapshotUnconfigured(t *testing.T) {
	if _, err := NewService(nil, "tok", "").Snapshot(); err == nil {
		t.Error("missing backend URL should fail")
	} else if want := "FAMILY_APP_SITE_URL"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should mention %s", err, want)
	}
	c := familyapp.NewClient("http://example.com", "", "")
	if _, err := NewService(c, "", "").Snapshot(); err == nil {
		t.Error("missing token should fail")
	} else if want := "FAMILY_APP_SCENES_TOKEN"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should mention %s", err, want)
	}
}

func TestSnapshotNeverLoaded(t *testing.T) {
	c := familyapp.NewClient("http://example.com", "", "")
	if _, err := NewService(c, "tok", "").Snapshot(); err == nil {
		t.Error("expected an error before the first refresh")
	}
}

func TestSnapshotDeepCopy(t *testing.T) {
	fixture := loadFeedFixture(t)
	srv, _ := scenesServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	})
	s := scenesService(t, srv, "tok")
	refreshScenes(t, s)

	first, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	first.Feed.Scenes[0].Name = "mutated"
	first.Feed.Scenes[0].Widgets[0].X = 99
	second, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if second.Feed.Scenes[0].Name != "Morgen" || second.Feed.Scenes[0].Widgets[0].X != 0 {
		t.Errorf("snapshot shares memory with the service: %+v", second.Feed.Scenes[0])
	}
}
