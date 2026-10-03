package familyapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/calendar-feed-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// feedServer serves data (or calls h) and records the Authorization header.
func feedServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *string) {
	t.Helper()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/dashboard/calendars" {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &gotAuth
}

func convexService(t *testing.T, srv *httptest.Server, token string) *CalendarService {
	t.Helper()
	c := NewClient(srv.URL, "", "")
	return NewCalendarService(c, token, filepath.Join(t.TempDir(), "cache.json"), berlin)
}

func refresh(t *testing.T, s *CalendarService) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Refresh(ctx)
}

// The Convex feed works without any local CALENDAR_n_URL configuration:
// the service only talks to the companion backend.
func TestConvexCalendarsWithoutLocalURLs(t *testing.T) {
	fixture := loadFixture(t)
	srv, gotAuth := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	})
	s := convexService(t, srv, "cal-token")
	refresh(t, s)

	if *gotAuth != "Bearer cal-token" {
		t.Errorf("auth = %q, want Bearer cal-token", *gotAuth)
	}
	snap := s.Snapshot()
	if !snap.Central {
		t.Error("snapshot should be marked central")
	}
	if len(snap.Calendars) != 2 {
		t.Fatalf("calendars = %d, want 2", len(snap.Calendars))
	}
	fam, sch := snap.Calendars[0], snap.Calendars[1]
	if fam.CalendarID != "cal_familie" || fam.ID != 0 || fam.Into != -1 || fam.Name != "Familie" {
		t.Errorf("family calendar: %+v", fam)
	}
	if sch.CalendarID != "cal_schule_lena" || sch.ID != 1 || sch.Into != 0 {
		t.Errorf("school calendar should merge into display column 0: %+v", sch)
	}
	if len(sch.PersonIDs) != 1 || sch.PersonIDs[0] != "user_lena" {
		t.Errorf("school personIds: %v", sch.PersonIDs)
	}
	if len(snap.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(snap.Events))
	}
	for _, e := range snap.Events {
		if e.CalendarID == "" || e.Key == "" || e.UID == "" || e.IdentityQuality == "" {
			t.Errorf("event misses stable identity: %+v", e)
		}
	}
	if len(snap.People) != 2 || len(snap.Bindings) != 1 {
		t.Errorf("people/bindings: %d/%d, want 2/1", len(snap.People), len(snap.Bindings))
	}
	if snap.ConfigurationRevision != 3 {
		t.Errorf("configurationRevision = %d, want 3", snap.ConfigurationRevision)
	}
	if snap.WindowStart.UnixMilli() != 1790978400000 || snap.WindowEnd.UnixMilli() != 1794610800000 {
		t.Errorf("window = %v..%v", snap.WindowStart, snap.WindowEnd)
	}
}

// Convex selection never starts local ICS polling, even when the feed fails.
func TestConvexNoLocalFallback(t *testing.T) {
	icsHits := 0
	ics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		icsHits++
		w.Write([]byte("BEGIN:VCALENDAR"))
	}))
	t.Cleanup(ics.Close)

	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
	})
	s := convexService(t, srv, "cal-token")
	_ = ics // local URLs are not even configured; the service must not dial ICS
	refresh(t, s)

	snap := s.Snapshot()
	if !snap.Central {
		t.Error("failed convex refresh must keep the central mode, not fall back to local")
	}
	if len(snap.Events) != 0 || len(snap.Calendars) != 0 {
		t.Errorf("no data expected, got %d events %d calendars", len(snap.Events), len(snap.Calendars))
	}
	if len(snap.Errors) == 0 {
		t.Error("unavailable source must be visible via snapshot errors")
	}
	if icsHits != 0 {
		t.Errorf("local ICS endpoint hit %d times in convex mode", icsHits)
	}
}

func TestRejectMissingVersionOrArrays(t *testing.T) {
	fixture := loadFixture(t)
	var base map[string]any
	if err := json.Unmarshal(fixture, &base); err != nil {
		t.Fatal(err)
	}
	clone := func() map[string]any {
		raw, _ := json.Marshal(base)
		var m map[string]any
		json.Unmarshal(raw, &m)
		return m
	}
	cases := map[string]func(map[string]any){
		"empty object": func(m map[string]any) {
			for k := range m {
				delete(m, k)
			}
		},
		"missing version":   func(m map[string]any) { delete(m, "version") },
		"missing people":    func(m map[string]any) { delete(m, "people") },
		"missing bindings":  func(m map[string]any) { delete(m, "bindings") },
		"missing calendars": func(m map[string]any) { delete(m, "calendars") },
		"missing events":    func(m map[string]any) { delete(m, "events") },
		"missing window":    func(m map[string]any) { delete(m, "window") },
		"wrong version":     func(m map[string]any) { m["version"] = 2 },
		"wrong scope":       func(m map[string]any) { m["scope"] = "other" },
		"wrong timezone":    func(m map[string]any) { m["timezone"] = "UTC" },
		"null calendars":    func(m map[string]any) { m["calendars"] = nil },
		"null events":       func(m map[string]any) { m["events"] = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := clone()
			mutate(m)
			raw, _ := json.Marshal(m)
			if _, err := ParseCalendarFeed(raw, berlin); err == nil {
				t.Errorf("ParseCalendarFeed accepted %s", name)
			}
		})
	}
}

func TestRejectBrokenReferences(t *testing.T) {
	fixture := loadFixture(t)
	var base map[string]any
	if err := json.Unmarshal(fixture, &base); err != nil {
		t.Fatal(err)
	}
	clone := func() map[string]any {
		raw, _ := json.Marshal(base)
		var m map[string]any
		json.Unmarshal(raw, &m)
		return m
	}
	at := func(m map[string]any, key string, i int) map[string]any {
		return m[key].([]any)[i].(map[string]any)
	}
	cases := map[string]func(map[string]any){
		"event unknown calendar": func(m map[string]any) {
			at(m, "events", 0)["calendarId"] = "cal_nope"
		},
		"binding unknown person": func(m map[string]any) {
			at(m, "bindings", 0)["personId"] = "user_nope"
		},
		"calendar unknown person": func(m map[string]any) {
			at(m, "calendars", 1)["personIds"] = []any{"user_nope"}
		},
		"dangling intoCalendarId": func(m map[string]any) {
			at(m, "calendars", 1)["intoCalendarId"] = "cal_nope"
		},
		"duplicate calendar id": func(m map[string]any) {
			at(m, "calendars", 1)["id"] = "cal_familie"
		},
		"bad panel": func(m map[string]any) {
			at(m, "calendars", 0)["panel"] = "fridge"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := clone()
			mutate(m)
			raw, _ := json.Marshal(m)
			if _, err := ParseCalendarFeed(raw, berlin); err == nil {
				t.Errorf("ParseCalendarFeed accepted %s", name)
			}
		})
	}
}

// A complete valid empty feed replaces previous events (explicit empty
// differs from failed/oversize responses, which must preserve last-good).
func TestCompleteEmptyReplacesEvents(t *testing.T) {
	fixture := loadFixture(t)
	var base map[string]any
	if err := json.Unmarshal(fixture, &base); err != nil {
		t.Fatal(err)
	}
	base["people"] = []any{}
	base["bindings"] = []any{}
	base["calendars"] = []any{}
	base["events"] = []any{}
	empty, _ := json.Marshal(base)
	if _, err := ParseCalendarFeed(empty, berlin); err != nil {
		t.Fatalf("explicit empty feed must be valid: %v", err)
	}

	body := append([]byte(nil), fixture...)
	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	s := convexService(t, srv, "tok")
	refresh(t, s)
	if n := len(s.Snapshot().Events); n != 3 {
		t.Fatalf("events = %d, want 3", n)
	}
	body = append([]byte(nil), empty...)
	refresh(t, s)
	snap := s.Snapshot()
	if len(snap.Events) != 0 || len(snap.Calendars) != 0 {
		t.Errorf("empty feed must replace: %d events %d calendars", len(snap.Events), len(snap.Calendars))
	}
}

// UpdatedAt comes from the providers' success times, never from the GET time.
func TestProviderSuccessNotGETTime(t *testing.T) {
	fixture := loadFixture(t)
	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	})
	s := convexService(t, srv, "tok")
	before := time.Now()
	refresh(t, s)
	snap := s.Snapshot()
	want := time.UnixMilli(1791028680000)
	if !snap.UpdatedAt.Equal(want.In(snap.UpdatedAt.Location())) && snap.UpdatedAt.UnixMilli() != want.UnixMilli() {
		t.Errorf("UpdatedAt = %v, want oldest provider success %v", snap.UpdatedAt, want)
	}
	if snap.UpdatedAt.After(before) || time.Since(snap.UpdatedAt) < time.Hour {
		t.Errorf("UpdatedAt = %v looks like GET time, want 2026 provider time", snap.UpdatedAt)
	}
}

// Oversize responses are an error, never a silent truncation.
func TestOversizeDetected(t *testing.T) {
	fixture := loadFixture(t)
	var base map[string]any
	if err := json.Unmarshal(fixture, &base); err != nil {
		t.Fatal(err)
	}
	base["padding"] = strings.Repeat("x", (4<<20)+100)
	big, _ := json.Marshal(base)
	if len(big) <= 4<<20 {
		t.Fatalf("test setup: big = %d", len(big))
	}
	if _, err := ParseCalendarFeed(big, berlin); err == nil {
		t.Error("ParseCalendarFeed accepted an oversize body")
	}

	body := append([]byte(nil), fixture...)
	srv, _ := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	s := convexService(t, srv, "tok")
	refresh(t, s)
	if n := len(s.Snapshot().Events); n != 3 {
		t.Fatalf("events = %d, want 3", n)
	}
	body = append([]byte(nil), big...)
	refresh(t, s)
	if n := len(s.Snapshot().Events); n != 3 {
		t.Errorf("oversize must preserve last-good: events = %d", n)
	}
}
