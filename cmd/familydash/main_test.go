package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/familyapp"
	"familydash/internal/server"
	"familydash/internal/timetable"
)

func testConfig(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestNewCalendarSourceDefaultsLocal(t *testing.T) {
	cfg := testConfig(t, nil)
	src := newCalendarSource(cfg)
	if _, ok := src.(*calendar.Service); !ok {
		t.Errorf("default source is %T, want *calendar.Service", src)
	}
}

// All consumers (server, briefing, pusher, previews) share newCalendarSource:
// in convex mode it returns the central service, talks only to the fake
// backend, and never falls back to local polling when the feed fails.
func TestNewCalendarSourceConvex(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/calendar-feed-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var body = append([]byte(nil), fixture...)
	var status = http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cal-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "not configured", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cfg := testConfig(t, map[string]string{
		"CALENDAR_SOURCE":           "convex",
		"FAMILY_APP_SITE_URL":       srv.URL,
		"FAMILY_APP_CALENDAR_TOKEN": "cal-token",
		"CALENDAR_CACHE_FILE":       filepath.Join(t.TempDir(), "cache.json"),
	})
	src := newCalendarSource(cfg)
	cs, ok := src.(*familyapp.CalendarService)
	if !ok {
		t.Fatalf("convex source is %T, want *familyapp.CalendarService", src)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	src.Refresh(ctx)
	if snap := cs.Snapshot(); len(snap.Events) != 3 || !snap.Central {
		t.Fatalf("convex snapshot: %d events central=%v", len(snap.Events), snap.Central)
	}

	// Backend outage: the same factory product keeps last-good data and the
	// central mode instead of starting local polling.
	status = http.StatusServiceUnavailable
	src.Refresh(ctx)
	if _, ok := newCalendarSource(cfg).(*familyapp.CalendarService); !ok {
		t.Error("factory must keep selecting the central source after failures")
	}
	if snap := cs.Snapshot(); len(snap.Events) != 3 {
		t.Errorf("outage replaced last-good: %d events", len(snap.Events))
	}
}

// The pusher feeds the family app from every source snapshot: timetable days
// go in keyed by child name (compat) and by stable child id (central mode
// resolves bindings through DaysByID, so a rename cannot move a week).
func TestNewPusherFillsPlanDaysAndPlanDaysByID(t *testing.T) {
	cfg := testConfig(t, nil)
	plan := &timetable.File{Children: []timetable.Child{{ID: "plan-42", Name: "Lukas"}}}
	p := newPusher(cfg, calendar.NewService(cfg), server.Sources{Plan: plan})
	in := p.Gather(time.Now())
	if got := in.PlanDays["Lukas"]; len(got) != familyapp.Days {
		t.Errorf("PlanDays[Lukas] has %d days, want %d", len(got), familyapp.Days)
	}
	if got := in.PlanDaysByID["plan-42"]; len(got) != familyapp.Days {
		t.Errorf("PlanDaysByID[plan-42] has %d days, want %d", len(got), familyapp.Days)
	}
}

// An invalid chosen convex config is a visible unavailable source, never a
// silent local fallback.
func TestNewCalendarSourceConvexInvalidConfig(t *testing.T) {
	cfg := testConfig(t, map[string]string{
		"CALENDAR_SOURCE":     "convex",
		"FAMILY_APP_SITE_URL": "https://familybackend-http.matthias.lol",
	})
	src := newCalendarSource(cfg)
	if _, ok := src.(*familyapp.CalendarService); !ok {
		t.Fatalf("convex source is %T, want *familyapp.CalendarService", src)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	src.Refresh(ctx)
	snap := src.Snapshot()
	if !snap.Central || len(snap.Events) != 0 || len(snap.Errors) == 0 {
		t.Errorf("want visible unavailable central state, got %+v", snap)
	}
}
