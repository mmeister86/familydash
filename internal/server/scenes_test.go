package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"familydash/internal/briefing"
	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/familyapp"
	"familydash/internal/scene"
	"familydash/internal/scenes"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func fallbackSceneProvider(t *testing.T) *scene.Provider {
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

func feedStub(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dashboard/scenes" {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func loadScenesFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "dashboard-scenes-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// sceneTestServer wires a Server to a stubbed scenes feed.
func sceneTestServer(t *testing.T, body []byte) *Server {
	t.Helper()
	srv := feedStub(t, body)
	provider := fallbackSceneProvider(t)
	svc := scenes.NewService(familyapp.NewClient(srv.URL, "", ""), "tok", filepath.Join(t.TempDir(), "cache.json"), provider)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc.Refresh(ctx)
	return &Server{
		cfg:           &config.Config{Location: berlin(t)},
		scenes:        svc,
		sceneProvider: provider,
	}
}

func TestResolveSceneFeed(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 07:00", berlin(t))
	got := s.resolveScene(mon, "")
	if got.Scene.ID != "morning" || got.Scene.Name != "Morgen" || got.Scene.Preview {
		t.Errorf("scene = %+v", got.Scene)
	}
	if got.Layout.Revision != 12 {
		t.Errorf("revision = %d, want 12", got.Layout.Revision)
	}
	if len(got.Layout.Widgets) != 3 {
		t.Errorf("widgets = %d, want 3", len(got.Layout.Widgets))
	}
}

func TestResolveScenePreview(t *testing.T) {
	now := time.Now()
	until := now.Add(90 * time.Second).UnixMilli()
	body := fmt.Sprintf(`{"version":1,"revision":3,"grid":{"cols":12,"rows":24},"scenes":[
		{"id":"morning","name":"Morgen","kind":"layout","start":{"schoolDay":"06:45","weekend":"07:30"},
		 "widgets":[{"id":"w1","type":"clock","x":0,"y":0,"w":12,"h":4}]},
		{"id":"night","name":"Nacht","kind":"night","start":{"schoolDay":"21:30","weekend":"21:30"}}],
		"preview":{"sceneId":"morning","until":%d,
		 "widgets":[{"id":"p1","type":"news","x":0,"y":0,"w":12,"h":4}]}}`, until)
	s := sceneTestServer(t, []byte(body))
	got := s.resolveScene(now, "")
	if got.Scene.ID != "morning" || !got.Scene.Preview {
		t.Fatalf("preview scene = %+v", got.Scene)
	}
	if got.Scene.Name != "Morgen" || got.Scene.Kind != "layout" {
		t.Errorf("preview identity = %+v", got.Scene)
	}
	if len(got.Layout.Widgets) != 1 || got.Layout.Widgets[0].ID != "p1" {
		t.Errorf("preview widgets = %+v", got.Layout.Widgets)
	}
	if got.Layout.Revision != 3 {
		t.Errorf("revision = %d, want 3", got.Layout.Revision)
	}
	if d := got.Scene.Until.Sub(got.Scene.Since); d != 2*time.Minute {
		t.Errorf("preview window = %v, want 2m", d)
	}
	if !got.Scene.Until.After(now) || !got.Scene.Until.Before(now.Add(5*time.Minute)) {
		t.Errorf("preview until = %v", got.Scene.Until)
	}
}

func TestResolveSceneFallback(t *testing.T) {
	provider := fallbackSceneProvider(t)
	svc := scenes.NewService(familyapp.NewClient("http://example.com", "", ""), "", "", nil)
	s := &Server{
		cfg:           &config.Config{Location: berlin(t)},
		scenes:        svc,
		sceneProvider: provider,
	}
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 07:00", berlin(t))
	got := s.resolveScene(mon, "")
	if got.Scene.ID != "morning" || got.Scene.Preview {
		t.Errorf("scene = %+v", got.Scene)
	}
	if got.Layout.Revision != 0 {
		t.Errorf("revision = %d, want 0", got.Layout.Revision)
	}
	if len(got.Layout.Widgets) != 5 {
		t.Errorf("fallback widgets = %d, want 5", len(got.Layout.Widgets))
	}
}

func TestResolveSceneOverride(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 16:00", berlin(t))
	got := s.resolveScene(mon, "morning")
	if got.Scene.ID != "morning" || !got.Scene.Forced || got.Scene.Preview {
		t.Errorf("override = %+v", got.Scene)
	}
	if len(got.Layout.Widgets) != 3 {
		t.Errorf("override widgets = %d, want 3", len(got.Layout.Widgets))
	}
}

func TestResolveSceneFollowsFeedNotStaleProvider(t *testing.T) {
	// Provider still holds the SCENE_* fallback (afternoon at 16:00) while
	// the feed snapshot has the custom scene: resolve from the feed.
	srv := feedStub(t, loadScenesFixture(t))
	provider := fallbackSceneProvider(t)
	svc := scenes.NewService(familyapp.NewClient(srv.URL, "", ""), "tok", filepath.Join(t.TempDir(), "cache.json"), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc.Refresh(ctx)
	s := &Server{cfg: &config.Config{Location: berlin(t)}, scenes: svc, sceneProvider: provider}

	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 16:00", berlin(t))
	got := s.resolveScene(mon, "")
	if got.Scene.ID != "s-k3f9q2" {
		t.Errorf("scene = %q, want s-k3f9q2 from the feed snapshot", got.Scene.ID)
	}
	if len(got.Layout.Widgets) != 0 {
		t.Errorf("widgets = %d, want the feed scene's (empty) list", len(got.Layout.Widgets))
	}
}

func TestResolveSceneFallbackUsesConfiguredTimes(t *testing.T) {
	provider := scene.NewProvider(scene.Schedule{
		SchoolDay: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 5 * 60},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
		Weekend: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 5 * 60},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
	})
	svc := scenes.NewService(familyapp.NewClient("http://example.com", "", ""), "", "", nil)
	s := &Server{cfg: &config.Config{Location: berlin(t)}, scenes: svc, sceneProvider: provider}
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 05:30", berlin(t))
	got := s.resolveScene(at, "")
	if got.Scene.ID != "morning" {
		t.Errorf("scene = %q, want morning (configured 05:00 start)", got.Scene.ID)
	}
	if len(got.Layout.Widgets) != 5 {
		t.Errorf("widgets = %d, want built-in morning layout", len(got.Layout.Widgets))
	}
}

func TestResolveSceneKeepsProviderForce(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	s.sceneProvider.SetForce("morning")
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 16:00", berlin(t))
	got := s.resolveScene(mon, "")
	if got.Scene.ID != "morning" || !got.Scene.Forced {
		t.Errorf("forced scene = %+v", got.Scene)
	}
}

func TestResolveSceneOverrideUnknown(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 07:00", berlin(t))
	got := s.resolveScene(mon, "brunch")
	if got.Scene.ID != "morning" || got.Scene.Forced || got.Scene.Preview {
		t.Errorf("unknown override should fall back to time-based, got %+v", got.Scene)
	}
}

func TestHandleScene(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	rec := httptest.NewRecorder()
	s.handleScene(rec, httptest.NewRequest(http.MethodGet, "/api/scene", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	var payload struct {
		Scene struct {
			ID      string `json:"id"`
			Preview bool   `json:"preview"`
		} `json:"scene"`
		Layout struct {
			Revision int `json:"revision"`
			Widgets  []struct {
				ID string `json:"id"`
			} `json:"widgets"`
		} `json:"layout"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scene.ID == "" || payload.Scene.Preview {
		t.Errorf("scene = %+v", payload.Scene)
	}
	if payload.Layout.Revision != 12 {
		t.Errorf("revision = %d, want 12", payload.Layout.Revision)
	}
}

type stubCalendar struct {
	snap calendar.Snapshot
}

func (s stubCalendar) Snapshot() calendar.Snapshot { return s.snap }
func (s stubCalendar) Refresh(context.Context)     {}
func (s stubCalendar) Run(context.Context, time.Duration) {
}

func TestDashboardSceneAndLayout(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	s.calendar = stubCalendar{}
	s.briefing = &briefing.Service{
		Sched: s.sceneProvider, Loc: berlin(t),
		Lead: 20 * time.Minute, MinGap: 20 * time.Minute,
		Gather: func(time.Time) briefing.Data { return briefing.Data{} },
	}
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Scene struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"scene"`
		Layout struct {
			Revision int   `json:"revision"`
			Widgets  []any `json:"widgets"`
		} `json:"layout"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scene.ID == "" || payload.Scene.Name == "" || payload.Scene.Kind == "" {
		t.Errorf("scene = %+v", payload.Scene)
	}
	if payload.Layout.Revision != 12 || payload.Layout.Widgets == nil {
		t.Errorf("layout = %+v", payload.Layout)
	}
}

func TestDashboardPreviewBriefing(t *testing.T) {
	now := time.Now()
	until := now.Add(90 * time.Second).UnixMilli()
	body := fmt.Sprintf(`{"version":1,"revision":3,"grid":{"cols":12,"rows":24},"scenes":[
		{"id":"morning","name":"Morgen","kind":"layout","start":{"schoolDay":"06:45","weekend":"07:30"},
		 "widgets":[{"id":"w1","type":"clock","x":0,"y":0,"w":12,"h":4}]},
		{"id":"night","name":"Nacht","kind":"night","start":{"schoolDay":"21:30","weekend":"21:30"}}],
		"preview":{"sceneId":"morning","until":%d,
		 "widgets":[{"id":"p1","type":"briefing","x":0,"y":0,"w":6,"h":4,"props":{"kind":"tomorrow"}}]}}`, until)
	s := sceneTestServer(t, []byte(body))
	s.calendar = stubCalendar{}
	s.briefing = &briefing.Service{
		Sched: s.sceneProvider, Loc: berlin(t),
		Lead: time.Minute, MinGap: 20 * time.Minute,
		Gather: func(time.Time) briefing.Data { return briefing.Data{} },
	}
	if card := s.briefing.For(briefing.Evening, now); card == nil {
		t.Fatal("setup: no evening card")
	}
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Briefing *struct {
			Kind string `json:"kind"`
		} `json:"briefing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Briefing == nil || payload.Briefing.Kind != "evening" {
		t.Errorf("preview briefing = %+v, want the evening card", payload.Briefing)
	}
}

func TestDashboardPreviewWithoutBriefingFallsBack(t *testing.T) {
	now := time.Now()
	until := now.Add(90 * time.Second).UnixMilli()
	body := fmt.Sprintf(`{"version":1,"revision":3,"grid":{"cols":12,"rows":24},"scenes":[
		{"id":"morning","name":"Morgen","kind":"layout","start":{"schoolDay":"06:45","weekend":"07:30"},
		 "widgets":[{"id":"w1","type":"clock","x":0,"y":0,"w":12,"h":4}]},
		{"id":"night","name":"Nacht","kind":"night","start":{"schoolDay":"21:30","weekend":"21:30"}}],
		"preview":{"sceneId":"morning","until":%d,
		 "widgets":[{"id":"p1","type":"news","x":0,"y":0,"w":12,"h":4}]}}`, until)
	s := sceneTestServer(t, []byte(body))
	s.calendar = stubCalendar{}
	s.briefing = &briefing.Service{
		Sched: s.sceneProvider, Loc: berlin(t),
		Lead: 20 * time.Minute, MinGap: 20 * time.Minute,
		Gather: func(time.Time) briefing.Data { return briefing.Data{} },
	}
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Briefing *struct {
			Kind string `json:"kind"`
		} `json:"briefing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	want := s.briefing.Snapshot()
	if (payload.Briefing == nil) != (want == nil) {
		t.Fatalf("briefing = %+v, want scheduled %+v", payload.Briefing, want)
	}
	if payload.Briefing != nil && payload.Briefing.Kind != string(want.Kind) {
		t.Errorf("briefing = %q, want scheduled %q", payload.Briefing.Kind, want.Kind)
	}
}

func TestDashboardSceneQuery(t *testing.T) {
	s := sceneTestServer(t, loadScenesFixture(t))
	s.calendar = stubCalendar{}
	s.briefing = &briefing.Service{
		Sched: s.sceneProvider, Loc: berlin(t),
		Lead: 20 * time.Minute, MinGap: 20 * time.Minute,
		Gather: func(time.Time) briefing.Data { return briefing.Data{} },
	}
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard?scene=evening", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Briefing *struct {
			Kind string `json:"kind"`
		} `json:"briefing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Briefing == nil || payload.Briefing.Kind != "evening" {
		t.Errorf("briefing = %+v", payload.Briefing)
	}

	unknown := httptest.NewRecorder()
	s.handleDashboard(unknown, httptest.NewRequest(http.MethodGet, "/api/dashboard?scene=brunch", nil))
	if unknown.Code != http.StatusOK {
		t.Errorf("unknown scene status = %d, want 200", unknown.Code)
	}
}
