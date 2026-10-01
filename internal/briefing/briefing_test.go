package briefing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/calendar"
	"familydash/internal/scene"
	"familydash/internal/things"
	"familydash/internal/vielfalt"
	"familydash/internal/waste"
	"familydash/internal/weather"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, berlin)
	if err != nil {
		panic(err)
	}
	return t
}

// Thursday evening, looking at Friday 2026-10-02
func sample() Data {
	return Data{
		Calendar: calendar.Snapshot{
			Calendars: []calendar.CalendarInfo{
				{ID: 0, Name: "Familie", Color: "#4F8EF7", Panel: "column"},
				{ID: 1, Name: "Hannah", Color: "#F2B53A", Panel: "school"},
			},
			Events: []calendar.Event{
				{Cal: 0, Calendar: "Familie", Title: "Elternabend", Start: at("2026-10-01 19:30"), End: at("2026-10-01 21:00")},
				{Cal: 0, Calendar: "Familie", Title: "Zahnarzt Lukas", Start: at("2026-10-02 16:00"), End: at("2026-10-02 17:00")},
				{Cal: 1, Calendar: "Hannah", Title: "Schwimmen", Start: at("2026-10-02 16:30"), End: at("2026-10-02 17:30")},
				{Cal: 0, Calendar: "Familie", Title: "Oma Geburtstag", AllDay: true, StartDate: "2026-10-02", EndDate: "2026-10-03"},
				{Cal: 0, Calendar: "Familie", Title: "Nächste Woche", Start: at("2026-10-05 10:00"), End: at("2026-10-05 11:00")},
			},
		},
		School: &besteschule.School{Students: []besteschule.Student{{
			Name: "Lukas",
			Day: besteschule.Day{Date: "2026-10-02", Lessons: []besteschule.Lesson{
				{Nr: 1, Start: "07:30", End: "08:15", Subject: "Mathe", Status: "cancelled"},
				{Nr: 2, Start: "08:25", End: "09:10", Subject: "Deutsch"},
				{Nr: 3, Start: "09:30", End: "10:15", Subject: "Sport", Status: "substitution", Info: "statt Englisch"},
			}},
			Exams:    []besteschule.Entry{{Date: "2026-10-02", Subject: "Bio", Kind: "Kurzkontrolle"}, {Date: "2026-10-20", Subject: "Mathe"}},
			Homework: []besteschule.Entry{{Date: "2026-10-02", Subject: "Deutsch", Text: "S. 34 Nr. 2"}},
		}}},
		Meals: &vielfalt.Meals{Children: []vielfalt.Child{
			{Name: "Lukas", Color: "#46C28E", Days: []vielfalt.Day{{Date: "2026-10-02", Ordered: []vielfalt.Dish{}}}},
		}},
		Waste: []waste.Pickup{{Name: "Gelbe Tonne", Dates: []string{"2026-10-02", "2026-10-16"}}},
		Weather: &weather.Weather{
			Daily: []weather.Day{{Date: "2026-10-02", Min: 6.4, Max: 14.6, PrecipProb: 70, Label: "Regen", Icon: "rain"}},
			Hourly: []weather.Hour{
				{Time: at("2026-10-02 07:00"), Temp: 7, PrecipProb: 10, Icon: "cloud"},
				{Time: at("2026-10-02 13:00"), Temp: 13, PrecipProb: 64, Icon: "rain"},
			},
		},
		Todos: &things.List{Tasks: []things.Task{
			{ID: "1", Title: "Turnbeutel waschen", Evening: true},
			{ID: "2", Title: "Steuer", Deadline: "2026-10-20"},
			{ID: "3", Title: "Brief einwerfen", Deadline: "2026-10-02"},
			{ID: "4", Title: "Erledigt", Evening: true, Done: true},
		}},
	}
}

func TestBuildFactsEvening(t *testing.T) {
	now := at("2026-10-01 19:05")
	f := BuildFacts(Evening, now, Target(Evening, now, berlin), berlin, sample())

	if f.Zieltag != "Freitag, 2. Oktober" || f.Art != "Abend-Vorausschau auf morgen" {
		t.Fatalf("header: %q %q", f.Art, f.Zieltag)
	}
	if len(f.Kinder) != 1 {
		t.Fatalf("kids: %+v", f.Kinder)
	}
	k := f.Kinder[0]
	sd := k.Schule
	if sd == nil || sd.Beginn != "08:25" || sd.Ende != "10:15" || !sd.Sport {
		t.Fatalf("school day: %+v", sd)
	}
	if len(sd.Ausfall) != 1 || sd.Ausfall[0] != "1. Stunde Mathe" {
		t.Errorf("ausfall: %v", sd.Ausfall)
	}
	if len(sd.Vertretung) != 1 || !strings.Contains(sd.Vertretung[0], "statt Englisch") {
		t.Errorf("vertretung: %v", sd.Vertretung)
	}
	if len(k.Arbeiten) != 1 || k.Arbeiten[0] != "Bio Kurzkontrolle – morgen" {
		t.Errorf("exams: %v", k.Arbeiten)
	}
	if len(k.Hausaufgaben) != 1 || k.Hausaufgaben[0] != "Deutsch: S. 34 Nr. 2" {
		t.Errorf("homework: %v", k.Hausaufgaben)
	}
	if !strings.HasPrefix(k.Essen, "nichts bestellt") {
		t.Errorf("lunch: %q", k.Essen)
	}
	if len(f.HeuteAbend) != 1 || f.HeuteAbend[0].Titel != "Elternabend" {
		t.Errorf("tonight: %+v", f.HeuteAbend)
	}
	if len(f.Termine) != 3 {
		t.Errorf("appointments: %+v", f.Termine)
	}
	if len(f.Gleichzeitig) != 1 {
		t.Errorf("overlaps: %v", f.Gleichzeitig)
	}
	if len(f.Muell) != 1 || !strings.Contains(f.Muell[0], "heute Abend rausstellen") {
		t.Errorf("bins: %v", f.Muell)
	}
	w := f.Wetter
	if w == nil || w.Min != 6 || w.Max != 15 || w.RegenMax != 60 || w.RegenAb != "13:00" || w.Schulweg != "7 °C" {
		t.Errorf("weather: %+v", w)
	}
	if len(f.Todos) != 2 || f.Todos[0].Titel != "Turnbeutel waschen" || f.Todos[1].Faellig != "morgen" {
		t.Errorf("todos: %+v", f.Todos)
	}
}

func TestBuildFactsWeekendHasNoSchool(t *testing.T) {
	now := at("2026-10-02 19:00") // Friday evening → Saturday
	f := BuildFacts(Evening, now, Target(Evening, now, berlin), berlin, sample())
	if !f.Wochenende || f.Kinder[0].Schule != nil {
		t.Fatalf("weekend: %v %+v", f.Wochenende, f.Kinder[0].Schule)
	}
}

func TestBuildFactsMorningSkipsPastEvents(t *testing.T) {
	d := sample()
	d.Calendar.Events = append(d.Calendar.Events,
		calendar.Event{Cal: 0, Calendar: "Familie", Title: "Frühsport", Start: at("2026-10-02 06:00"), End: at("2026-10-02 06:30")})
	now := at("2026-10-02 06:40")
	f := BuildFacts(Morning, now, Target(Morning, now, berlin), berlin, d)
	for _, a := range f.Termine {
		if a.Titel == "Frühsport" {
			t.Fatal("past event listed")
		}
	}
	if len(f.HeuteAbend) != 0 {
		t.Errorf("tonight in the morning: %+v", f.HeuteAbend)
	}
	if f.Muell[0] != "Gelbe Tonne: Abholung heute" {
		t.Errorf("bins: %v", f.Muell)
	}
}

func TestFallbackEvening(t *testing.T) {
	now := at("2026-10-01 19:05")
	d := sample()
	f := BuildFacts(Evening, now, Target(Evening, now, berlin), berlin, d)
	b := Fallback(Evening, Target(Evening, now, berlin), f, Colors(d))
	if b.AI || b.Date != "2026-10-02" || len(b.Items) == 0 || len(b.Items) > maxItems {
		t.Fatalf("card: %+v", b)
	}
	want := map[string]bool{"muell": false, "test": false, "brotbox": false, "sport": false}
	for _, it := range b.Items {
		if _, ok := want[it.Icon]; ok {
			want[it.Icon] = true
			if it.Section != "tonight" {
				t.Errorf("%s should be tonight: %+v", it.Icon, it)
			}
		}
		if it.Who == "Lukas" && it.Color != "#46C28E" {
			t.Errorf("Lukas' color: %q", it.Color)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("missing %s in %+v", k, b.Items)
		}
	}
	if b.Items[0].Section != "tonight" {
		t.Errorf("tonight first: %+v", b.Items)
	}
}

func TestSameKid(t *testing.T) {
	for _, c := range []struct {
		a, b string
		ok   bool
	}{{"Lukas", "lukas", true}, {"Lukas", "Meister Lukas", true}, {"Lukas Meister", "Lukas Müller", false}, {"", "Lukas", false}} {
		if got := SameKid(c.a, c.b); got != c.ok {
			t.Errorf("SameKid(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// fake generateContent: records requests, answers with JSON wrapped like Gemini
func fakeGemini(t *testing.T, status int, answer string, hits *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("missing key header")
		}
		if r.URL.Query().Get("key") != "" {
			t.Errorf("key must not be in the URL")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("request: %v", err)
		}
		gen := req["generationConfig"].(map[string]any)
		if gen["responseMimeType"] != "application/json" || gen["responseSchema"] == nil {
			t.Errorf("generationConfig: %v", gen)
		}
		if tc, _ := gen["thinkingConfig"].(map[string]any); tc["thinkingLevel"] != "LOW" {
			t.Errorf("thinking: %v", gen["thinkingConfig"])
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			w.Write([]byte(answer))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
				map[string]any{"text": "thinking…", "thought": true},
				map[string]any{"text": answer},
			}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]any{"promptTokenCount": 900, "candidatesTokenCount": 120},
		})
	}))
}

const answer = `{"headline":"Freitag mit Bio-Test","items":[
 {"section":"day","icon":"termin","who":"Familie","text":"16:00 Zahnarzt Lukas"},
 {"section":"tonight","icon":"test","who":"lukas","text":"Für die Bio-Kurzkontrolle lernen"},
 {"section":"tonight","icon":"rakete","who":"Hannah","text":"Schwimmsachen packen"},
 {"section":"day","icon":"hinweis","text":"  "}]}`

func TestGeminiAutoSwitchesToVertex(t *testing.T) {
	var gHits, vHits int32
	g := fakeGemini(t, http.StatusBadRequest, `{"error":{"message":"API key not valid. Please pass a valid API key."}}`, &gHits)
	defer g.Close()
	v := fakeGemini(t, http.StatusOK, answer, &vHits)
	defer v.Close()

	m := NewGemini("k", "gemini-3.8-flash", "low", "auto")
	m.GeminiURL, m.VertexURL = g.URL, v.URL
	for i := 0; i < 2; i++ {
		out, usage, err := m.GenerateJSON(context.Background(), "sys", "user", schema())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "Freitag") || strings.Contains(string(out), "thinking") || usage.Prompt != 900 {
			t.Fatalf("answer: %s %+v", out, usage)
		}
	}
	if gHits != 1 || vHits != 2 {
		t.Errorf("hits gemini=%d vertex=%d, want 1/2 (vertex remembered)", gHits, vHits)
	}
}

func TestGeminiFixedBackendDoesNotSwitch(t *testing.T) {
	var gHits, vHits int32
	g := fakeGemini(t, http.StatusForbidden, `{"error":{"message":"denied"}}`, &gHits)
	defer g.Close()
	v := fakeGemini(t, http.StatusOK, answer, &vHits)
	defer v.Close()
	m := NewGemini("k", "m", "low", "gemini")
	m.GeminiURL, m.VertexURL = g.URL, v.URL
	_, _, err := m.GenerateJSON(context.Background(), "sys", "user", schema())
	if err == nil || !strings.Contains(err.Error(), "denied") || vHits != 0 {
		t.Fatalf("err=%v vertex=%d", err, vHits)
	}
}

func service(m *Gemini) *Service {
	sched := scene.Schedule{}
	sched.SchoolDay, _ = scene.Build(func(n string) string { return scene.DefaultSchoolDay[n] })
	sched.Weekend = sched.SchoolDay
	d := sample()
	return &Service{Model: m, Sched: &sched, Loc: berlin, Lead: 20 * time.Minute, MinGap: 20 * time.Minute,
		Gather: func(time.Time) Data { return d }, started: at("2026-10-01 00:00")}
}

func TestServiceTick(t *testing.T) {
	var hits int32
	srv := fakeGemini(t, http.StatusOK, answer, &hits)
	defer srv.Close()
	m := NewGemini("k", "m", "low", "gemini")
	m.GeminiURL = srv.URL
	s := service(m)
	ctx := context.Background()

	s.tick(ctx, at("2026-10-01 15:00")) // afternoon: nothing due
	if s.Snapshot() != nil || hits != 0 {
		t.Fatal("afternoon should not generate")
	}
	s.tick(ctx, at("2026-10-01 18:45")) // evening starts within the lead time
	b := s.Snapshot()
	if b == nil || !b.AI || b.Kind != Evening || b.Date != "2026-10-02" || hits != 1 {
		t.Fatalf("evening card: %+v hits=%d", b, hits)
	}
	if len(b.Items) != 3 || b.Items[0].Section != "tonight" || b.Items[1].Icon != "hinweis" {
		t.Fatalf("normalized items: %+v", b.Items)
	}
	if b.Items[0].Color != "#46C28E" || b.Items[1].Color != "#F2B53A" {
		t.Errorf("colors: %+v", b.Items)
	}
	s.tick(ctx, at("2026-10-01 18:46")) // same facts → no new call
	if hits != 1 {
		t.Errorf("unchanged facts called again (%d)", hits)
	}
	// new facts but within MinGap → wait; afterwards → call
	extra := sample()
	extra.Calendar.Events = append(extra.Calendar.Events, calendar.Event{Cal: 0, Calendar: "Familie", Title: "Neu", Start: at("2026-10-02 18:00"), End: at("2026-10-02 19:00")})
	s.Gather = func(time.Time) Data { return extra }
	s.tick(ctx, at("2026-10-01 18:50"))
	if hits != 1 {
		t.Errorf("called within MinGap")
	}
	s.tick(ctx, at("2026-10-01 19:10"))
	if hits != 2 {
		t.Errorf("not refreshed after MinGap (%d)", hits)
	}
}

func TestServiceFallsBackOnError(t *testing.T) {
	var hits int32
	srv := fakeGemini(t, http.StatusInternalServerError, `{"error":{"message":"boom"}}`, &hits)
	defer srv.Close()
	m := NewGemini("k", "m", "low", "gemini")
	m.GeminiURL = srv.URL
	s := service(m)
	s.tick(context.Background(), at("2026-10-01 19:00"))
	b := s.Snapshot()
	if b == nil || b.AI || !strings.Contains(b.Error, "boom") || len(b.Items) == 0 {
		t.Fatalf("fallback: %+v", b)
	}
	s.tick(context.Background(), at("2026-10-01 19:05")) // backoff
	if hits != 1 {
		t.Errorf("retried during backoff (%d)", hits)
	}
}

func TestServiceWithoutModel(t *testing.T) {
	s := service(nil)
	s.tick(context.Background(), at("2026-10-02 06:30")) // morning starts 06:45
	b := s.Snapshot()
	if b == nil || b.Kind != Morning || b.Date != "2026-10-02" || b.AI {
		t.Fatalf("rule-based morning card: %+v", b)
	}
}
