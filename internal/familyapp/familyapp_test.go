package familyapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/briefing"
	"familydash/internal/calendar"
	"familydash/internal/timetable"
	"familydash/internal/vielfalt"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, berlin)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Lukas": "lukas", "Lukas Meister": "lukas", " Hannah ": "hannah", "Jürgen": "juergen", "": ""} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// Friday 2026-10-02: Lukas from beste.schule, Hannah from the fixed plan.
func inputs() Inputs {
	updated := at("2026-10-02 06:50")
	return Inputs{
		Calendar: calendar.Snapshot{
			UpdatedAt: at("2026-10-02 06:55"),
			Calendars: []calendar.CalendarInfo{
				{ID: 0, Name: "Familie", Panel: "column"},
				{ID: 1, Name: "Schule Hannah", Panel: "school"},
				{ID: 2, Name: "Lukas", Panel: "school"},
			},
			Events: []calendar.Event{
				{Cal: 0, Calendar: "Familie", Title: "Zahnarzt Lukas", Start: at("2026-10-02 16:00"), End: at("2026-10-02 17:00")},
				{Cal: 1, Calendar: "Schule Hannah", Title: "Wandertag", AllDay: true, StartDate: "2026-10-05", EndDate: "2026-10-06"},
				{Cal: 2, Calendar: "Lukas", Title: "Elternsprechtag", Start: at("2026-10-06 17:00"), End: at("2026-10-06 18:00")},
				{Cal: 2, Calendar: "Lukas", Title: "zu weit", Start: at("2026-10-20 17:00"), End: at("2026-10-20 18:00")},
			},
		},
		School: &besteschule.School{UpdatedAt: updated, Students: []besteschule.Student{{
			Name:     "Lukas Meister",
			Homework: []besteschule.Entry{{Date: "2026-10-05", Subject: "Deutsch", Text: "S. 34"}},
			Exams:    []besteschule.Entry{{Date: "2026-10-08", Subject: "Mathe", Kind: "Klassenarbeit", Text: "Brüche"}},
		}}},
		SchoolDays: map[string][]besteschule.Day{"Lukas Meister": {
			{Date: "2026-10-02", Notices: []string{"Kurzstunden"}, Lessons: []besteschule.Lesson{
				{Nr: 1, Start: "07:30", End: "08:15", Subject: "Mathe", Status: "cancelled"},
				{Nr: 2, Start: "08:25", End: "09:10", Subject: "Sport", Status: "substitution", Info: "statt Englisch"},
			}},
		}},
		Timetables: []timetable.Card{{Student: besteschule.Student{Name: "Hannah"}, Calendar: "Schule Hannah"}},
		PlanDays: map[string][]besteschule.Day{"Hannah": {
			{Date: "2026-10-02", Lessons: []besteschule.Lesson{{Nr: 1, Subject: "Deutsch"}, {Subject: "Kreativ", Tag: "GTA", Start: "14:00", End: "15:00"}}},
		}},
		Meals: &vielfalt.Meals{Children: []vielfalt.Child{
			{Name: "Lukas", UpdatedAt: at("2026-10-02 06:30"), Days: []vielfalt.Day{
				{Date: "2026-10-02", Ordered: []vielfalt.Dish{{Name: "Nudeln", Side: "Salat"}}},
				{Date: "2026-10-05", Ordered: []vielfalt.Dish{}},
			}},
		}},
	}
}

func TestBuildChildren(t *testing.T) {
	kids := BuildChildren(inputs(), at("2026-10-02 07:00"), berlin, map[string][]int{"lukas": {0}})
	if len(kids) != 2 || kids[0].ChildSlug != "lukas" || kids[1].ChildSlug != "hannah" {
		t.Fatalf("kids = %+v", kids)
	}
	lukas, hannah := kids[0], kids[1]
	if len(lukas.Days) != Days || lukas.Days[0].Date != "2026-10-02" || lukas.Days[6].Date != "2026-10-08" {
		t.Fatalf("days: %+v", lukas.Days)
	}
	d0 := lukas.Days[0]
	if len(d0.Timetable) != 2 || d0.Timetable[0].Change == nil || d0.Timetable[0].Change.Type != "cancelled" ||
		d0.Timetable[1].Change.Type != "substitution" || d0.Timetable[1].Change.Note != "statt Englisch" {
		t.Errorf("timetable: %+v", d0.Timetable)
	}
	if len(d0.Notices) != 1 {
		t.Errorf("notices: %v", d0.Notices)
	}
	// Familie via FAMILY_APP_LUKAS_CALENDARS, own school calendar matched by name
	if len(d0.Events) != 1 || d0.Events[0].Title != "Zahnarzt Lukas" || d0.Events[0].Start != "2026-10-02T16:00:00+02:00" {
		t.Errorf("events today: %+v", d0.Events)
	}
	if ev := lukas.Days[4].Events; len(ev) != 1 || ev[0].Title != "Elternsprechtag" {
		t.Errorf("events tuesday: %+v", ev)
	}
	if m := d0.Meal; m == nil || !m.Ordered || m.Title != "Nudeln" || m.Description != "Salat" {
		t.Errorf("meal today: %+v", m)
	}
	if m := lukas.Days[3].Meal; m == nil || m.Ordered {
		t.Errorf("meal monday (nothing ordered): %+v", m)
	}
	if lukas.Days[1].Meal != nil {
		t.Errorf("saturday is no delivery day: %+v", lukas.Days[1].Meal)
	}
	if lukas.Days[1].Timetable == nil || lukas.Days[1].Events == nil {
		t.Error("empty lists must be [] not null")
	}
	if len(lukas.Homework) != 1 || lukas.Homework[0].DueDate != "2026-10-05" ||
		len(lukas.Exams) != 1 || lukas.Exams[0].Text != "Klassenarbeit Brüche" {
		t.Errorf("homework/exams: %+v %+v", lukas.Homework, lukas.Exams)
	}
	if lukas.SourceUpdatedAt != at("2026-10-02 06:30").UnixMilli() {
		t.Errorf("sourceUpdatedAt = %d, want the oldest source (lunch)", lukas.SourceUpdatedAt)
	}

	// Hannah: school calendar via the timetable's "calendar", GTA keeps its tag
	if tt := hannah.Days[0].Timetable; len(tt) != 2 || tt[1].Tag != "GTA" || tt[1].Period != 0 {
		t.Errorf("hannah timetable: %+v", tt)
	}
	if ev := hannah.Days[3].Events; len(ev) != 1 || !ev[0].AllDay || ev[0].Start != "2026-10-05" || ev[0].End != "2026-10-06" {
		t.Errorf("hannah events: %+v", ev)
	}
	if len(hannah.Days[0].Events) != 0 {
		t.Errorf("Lukas' dentist must not show up for Hannah: %+v", hannah.Days[0].Events)
	}

	// optional fields are omitted, never null (Convex v.optional)
	js, _ := json.Marshal(hannah)
	if strings.Contains(string(js), "null") {
		t.Errorf("null in payload: %s", js)
	}
}

func TestConvertTodos(t *testing.T) {
	r := TodosResponse{
		Date: "2026-10-02",
		People: []WirePerson{
			{Slug: "matthias", Name: "Matthias", Role: "parent"},
			{Slug: "lukas", Name: "Lukas", Role: "child", Points: 120},
		},
		Tasks: []WireTask{
			{ID: "a", Title: "Zimmer aufräumen", Assignee: "lukas", Date: "2026-10-02", Status: "done", Points: 10, Recurring: true},
			{ID: "b", Title: "Müll", Date: "2026-10-02", Status: "open", Recurring: true},
			{ID: "c", Title: "Bibliotheksbuch", Assignee: "lukas", Date: "2026-09-30", Status: "open", Points: 5},
			{ID: "d", Title: "Hund füttern", Assignee: "lukas", Date: "2026-10-02", Status: "pending", Points: 3, Recurring: true},
			{ID: "e", Title: "Gestern verpasst", Date: "2026-10-01", Status: "missed", Recurring: true},
			{ID: "f", Title: "Steuer", Assignee: "matthias", Date: "2026-10-03", Status: "open"},
			{ID: "g", Title: "Irgendwann", Assignee: "lukas", Status: "open", Points: 50},
			{ID: "h", Title: "Alt erledigt", Date: "2026-09-29", Status: "done"},
			{ID: "i", Title: "Irgendwann erledigt", Status: "done"},
		},
	}
	l := Convert(r, at("2026-10-02 07:00"), berlin)
	var got []string
	for _, t := range l.Tasks {
		got = append(got, t.ID)
	}
	if strings.Join(got, ",") != "c,b,g,d,a" {
		t.Fatalf("today order = %v (overdue, open incl. undated, waiting, done)", got)
	}
	if g := l.Tasks[2]; g.Who != "Lukas" || g.Deadline != "" || g.Points != 50 || g.Done {
		t.Errorf("undated one-off: %+v", g)
	}
	if c := l.Tasks[0]; c.Who != "Lukas" || c.Deadline != "2026-09-30" || c.Points != 5 {
		t.Errorf("overdue one-off: %+v", c)
	}
	if b := l.Tasks[1]; b.Who != "" || b.Deadline != "" {
		t.Errorf("recurring family task: %+v", b)
	}
	if !l.Tasks[3].Pending || l.Tasks[3].Done || !l.Tasks[4].Done {
		t.Errorf("states: %+v", l.Tasks[3:])
	}
	if len(l.Tomorrow) != 1 || l.Tomorrow[0].Who != "Matthias" {
		t.Errorf("tomorrow: %+v", l.Tomorrow)
	}
	if l.Pending != 1 || len(l.People) != 2 || l.People[1].Points != 120 || l.Source != "familyapp" {
		t.Errorf("list: %+v", l)
	}
}

func TestTodoServiceKeepsLastGoodList(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/todos" || r.URL.Query().Get("days") != "2" || r.Header.Get("Authorization") != "Bearer read" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"date":"2026-10-02","people":[],"tasks":[{"id":"1","title":"Müll","date":"2026-10-02","status":"open"}]}`)
	}))
	defer srv.Close()
	s := NewTodoService(NewClient(srv.URL+"/", "ingest", "read"), berlin)
	s.Refresh(context.Background())
	if l := s.Snapshot(); l == nil || len(l.Tasks) != 1 || l.Error != "" {
		t.Fatalf("first: %+v", l)
	}
	fail = true
	s.Refresh(context.Background())
	if l := s.Snapshot(); len(l.Tasks) != 1 || !strings.Contains(l.Error, "500") {
		t.Fatalf("after failure: %+v", l)
	}
}

type recorder struct {
	mu     sync.Mutex
	posts  []string // path:childSlug|kind
	status int
}

func (rc *recorder) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer ingest" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad request %s %s %v", r.Method, r.URL.Path, r.Header)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		rc.mu.Lock()
		defer rc.mu.Unlock()
		id, _ := body["childSlug"].(string)
		if id == "" {
			id, _ = body["kind"].(string)
		}
		rc.posts = append(rc.posts, r.URL.Path+":"+id)
		if rc.status != 0 {
			w.WriteHeader(rc.status)
		}
	})
}

func (rc *recorder) take() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	p := rc.posts
	rc.posts = nil
	return p
}

func TestPusherOnlyPostsChangesAndHeartbeat(t *testing.T) {
	rc := &recorder{}
	srv := httptest.NewServer(rc.handler(t))
	defer srv.Close()

	in := inputs()
	card := &briefing.Briefing{Kind: briefing.Morning, Date: "2026-10-02", Headline: "Regen ab Mittag",
		Items: []briefing.Item{{Section: "day", Icon: "regen", Who: "Familie", Text: "Regenjacke mitnehmen"}}, AI: true, CreatedAt: at("2026-10-02 06:30")}
	p := &Pusher{Client: NewClient(srv.URL, "ingest", "read"), Loc: berlin, Heartbeat: 15 * time.Minute,
		Gather:    func(time.Time) Inputs { return in },
		Briefings: func() []*briefing.Briefing { return []*briefing.Briefing{card} }}
	ctx := context.Background()

	p.Push(ctx, at("2026-10-02 07:00"))
	if got := strings.Join(rc.take(), " "); got != "/ingest/child:lukas /ingest/child:hannah /ingest/briefing:morning" {
		t.Fatalf("first push: %s", got)
	}
	p.Push(ctx, at("2026-10-02 07:05"))
	if got := rc.take(); len(got) != 0 {
		t.Fatalf("nothing changed, still posted: %v", got)
	}
	in.Meals.Children[0].Days[0].Ordered = nil // lunch cancelled → Lukas changes
	p.Push(ctx, at("2026-10-02 07:06"))
	if got := strings.Join(rc.take(), " "); got != "/ingest/child:lukas" {
		t.Fatalf("after change: %s", got)
	}
	p.Push(ctx, at("2026-10-02 07:20"))
	if got := strings.Join(rc.take(), " "); got != "/ingest/child:hannah" {
		t.Fatalf("heartbeat (children only, briefing unchanged): %s", got)
	}
}

func TestPusherBacksOffAfterFailure(t *testing.T) {
	rc := &recorder{status: http.StatusBadRequest}
	srv := httptest.NewServer(rc.handler(t))
	defer srv.Close()
	in := inputs()
	in.Timetables = nil
	p := &Pusher{Client: NewClient(srv.URL, "ingest", "read"), Loc: berlin, Heartbeat: 15 * time.Minute,
		Gather: func(time.Time) Inputs { return in }}
	ctx := context.Background()
	p.Push(ctx, at("2026-10-02 07:00"))
	p.Push(ctx, at("2026-10-02 07:01"))
	if got := rc.take(); len(got) != 1 {
		t.Fatalf("retried too soon: %v", got)
	}
	rc.status = 0
	p.Push(ctx, at("2026-10-02 07:06"))
	if got := rc.take(); len(got) != 1 {
		t.Fatalf("no retry after backoff: %v", got)
	}
}

func TestBriefingPayload(t *testing.T) {
	b := &briefing.Briefing{Kind: briefing.Evening, Date: "2026-10-03", Headline: "Ruhiger Samstag", AI: true, CreatedAt: at("2026-10-02 18:40"),
		Items: []briefing.Item{
			{Section: "tonight", Icon: "muell", Who: "Familie", Text: "Gelbe Tonne rausstellen"},
			{Section: "tonight", Icon: "sport", Who: "Lukas", Color: "#46C28E", Text: "Sportbeutel packen"},
			{Section: "day", Icon: "termin", Text: "10:00 Fußball"},
		}}
	p := NewBriefingPayload(b)
	want := "**Ruhiger Samstag**\n\nHeute Abend:\n- **Familie:** Gelbe Tonne rausstellen\n- **Lukas:** Sportbeutel packen\n\nMorgen:\n- 10:00 Fußball"
	if p.Text != want {
		t.Errorf("text:\n%s\nwant:\n%s", p.Text, want)
	}
	if p.Kind != "evening" || p.Date != "2026-10-03" || len(p.Items) != 3 || p.Items[1].Color != "#46C28E" || p.GeneratedAt != b.CreatedAt.UnixMilli() {
		t.Errorf("payload: %+v", p)
	}
}

// ---- central (Convex) mode: explicit person/calendar bindings ----

func centralSnapshot() calendar.Snapshot {
	succ := at("2026-10-02 06:55")
	return calendar.Snapshot{
		Central:               true,
		ConfigurationRevision: 7,
		UpdatedAt:             succ,
		People: []calendar.Person{
			{ID: "u-lukas", Slug: "lukas", Name: "Lukas Meister", Role: "child"},
			{ID: "u-hannah", Slug: "hannah", Name: "Hannah Meister", Role: "child"},
		},
		Bindings: []calendar.PersonBinding{
			{PersonID: "u-lukas", Kind: "besteschule", ExternalID: "bs-lukas"},
			{PersonID: "u-hannah", Kind: "timetable", ExternalID: "plan-01"},
		},
		Calendars: []calendar.CalendarInfo{
			{ID: 0, CalendarID: "cal-lukas", Name: "Schule Lukas", Color: "#F2B53A", Panel: "school", Into: -1,
				PersonIDs: []string{"u-lukas"},
				Status:    calendar.SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"}},
			{ID: 1, CalendarID: "cal-hannah", Name: "Schule Hannah", Color: "#46C28E", Panel: "school", Into: -1,
				PersonIDs: []string{"u-hannah"},
				Status:    calendar.SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"}},
		},
		Events: []calendar.Event{
			{Cal: 0, Calendar: "Schule Lukas", CalendarID: "cal-lukas", Title: "Elternsprechtag", Start: at("2026-10-06 17:00"), End: at("2026-10-06 18:00")},
			{Cal: 1, Calendar: "Schule Hannah", CalendarID: "cal-hannah", Title: "Wandertag", AllDay: true, StartDate: "2026-10-05", EndDate: "2026-10-06"},
		},
	}
}

func centralInputs() Inputs {
	return Inputs{
		Calendar: centralSnapshot(),
		School: &besteschule.School{UpdatedAt: at("2026-10-02 06:50"), Students: []besteschule.Student{
			{ID: "bs-lukas", Name: "Lukas Meister"},
		}},
		SchoolDays: map[string][]besteschule.Day{"Lukas Meister": {
			{Date: "2026-10-02", Lessons: []besteschule.Lesson{{Nr: 1, Subject: "Mathe"}}},
		}},
		Timetables: []timetable.Card{{Student: besteschule.Student{ID: "plan-01", Name: "Hannah"}, Calendar: "Schule Hannah"}},
		PlanDays:   map[string][]besteschule.Day{},
		PlanDaysByID: map[string][]besteschule.Day{"plan-01": {
			{Date: "2026-10-02", Lessons: []besteschule.Lesson{{Nr: 1, Subject: "Deutsch"}}},
		}},
	}
}

func centralKid(kids []ChildSnapshot, slug string) ChildSnapshot {
	for _, k := range kids {
		if k.ChildSlug == slug {
			return k
		}
	}
	return ChildSnapshot{}
}

func TestCalendarReorderKeepsChildMapping(t *testing.T) {
	now := at("2026-10-02 07:00")
	kids := BuildChildren(centralInputs(), now, berlin, nil)
	lukas, hannah := centralKid(kids, "lukas"), centralKid(kids, "hannah")
	if lukas.ChildSlug == "" || hannah.ChildSlug == "" {
		t.Fatalf("slugs come from bound users: %+v", kids)
	}
	if ev := lukas.Days[4].Events; len(ev) != 1 || ev[0].Title != "Elternsprechtag" {
		t.Fatalf("lukas events: %+v", lukas.Days[4].Events)
	}
	if ev := hannah.Days[3].Events; len(ev) != 1 || ev[0].Title != "Wandertag" {
		t.Fatalf("hannah events: %+v", hannah.Days[3].Events)
	}

	// same backend state, calendars renamed and reordered: the mapping must
	// follow the stable ids, not names or numeric indexes
	in := centralInputs()
	in.Calendar.Calendars[0], in.Calendar.Calendars[1] = in.Calendar.Calendars[1], in.Calendar.Calendars[0]
	for i := range in.Calendar.Calendars {
		in.Calendar.Calendars[i].ID = i
	}
	in.Calendar.Calendars[0].Name = "Hannah (neu)"
	in.Calendar.Calendars[1].Name = "Lukas (neu)"
	for i := range in.Calendar.Events {
		if in.Calendar.Events[i].CalendarID == "cal-lukas" {
			in.Calendar.Events[i].Cal = 1
			in.Calendar.Events[i].Calendar = "Lukas (neu)"
		} else {
			in.Calendar.Events[i].Cal = 0
			in.Calendar.Events[i].Calendar = "Hannah (neu)"
		}
	}
	again := BuildChildren(in, now, berlin, nil)
	if ev := centralKid(again, "lukas").Days[4].Events; len(ev) != 1 || ev[0].Title != "Elternsprechtag" {
		t.Errorf("lukas after reorder: %+v", ev)
	}
	if ev := centralKid(again, "hannah").Days[3].Events; len(ev) != 1 || ev[0].Title != "Wandertag" {
		t.Errorf("hannah after reorder: %+v", ev)
	}
}

func TestSameNamesDoNotMergeCentralBindings(t *testing.T) {
	succ := at("2026-10-02 06:55")
	fresh := calendar.SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"}
	in := Inputs{
		Calendar: calendar.Snapshot{
			Central: true,
			People: []calendar.Person{
				{ID: "u-1", Slug: "lukas1", Name: "Lukas Meister", Role: "child"},
				{ID: "u-2", Slug: "lukas2", Name: "Lukas Müller", Role: "child"},
			},
			Bindings: []calendar.PersonBinding{
				{PersonID: "u-1", Kind: "besteschule", ExternalID: "bs-1"},
				{PersonID: "u-2", Kind: "besteschule", ExternalID: "bs-2"},
			},
			Calendars: []calendar.CalendarInfo{
				{ID: 0, CalendarID: "c-1", Name: "Schule", Panel: "school", PersonIDs: []string{"u-1"}, Status: fresh},
				{ID: 1, CalendarID: "c-2", Name: "Schule", Panel: "school", PersonIDs: []string{"u-2"}, Status: fresh},
			},
			Events: []calendar.Event{
				{Cal: 0, Calendar: "Schule", CalendarID: "c-1", Title: "Test A", Start: at("2026-10-06 17:00"), End: at("2026-10-06 18:00")},
				{Cal: 1, Calendar: "Schule", CalendarID: "c-2", Title: "Test B", Start: at("2026-10-06 17:00"), End: at("2026-10-06 18:00")},
			},
		},
		School: &besteschule.School{UpdatedAt: succ, Students: []besteschule.Student{
			{ID: "bs-1", Name: "Lukas Meister"},
			{ID: "bs-2", Name: "Lukas Müller"},
		}},
		SchoolDays: map[string][]besteschule.Day{
			"Lukas Meister": {{Date: "2026-10-02"}},
			"Lukas Müller":  {{Date: "2026-10-02"}},
		},
	}
	kids := BuildChildren(in, at("2026-10-02 07:00"), berlin, nil)
	if len(kids) != 2 {
		t.Fatalf("same-name source children must not merge: %+v", kids)
	}
	a, b := centralKid(kids, "lukas1"), centralKid(kids, "lukas2")
	if a.ChildSlug == "" || b.ChildSlug == "" {
		t.Fatalf("person slugs: %+v", kids)
	}
	if ev := a.Days[4].Events; len(ev) != 1 || ev[0].Title != "Test A" {
		t.Errorf("lukas1 events: %+v", ev)
	}
	if ev := b.Days[4].Events; len(ev) != 1 || ev[0].Title != "Test B" {
		t.Errorf("lukas2 events: %+v", ev)
	}
}

func TestAllAssignedCalendarsIncluded(t *testing.T) {
	in := centralInputs()
	succ := at("2026-10-02 06:55")
	in.Calendar.Calendars = append(in.Calendar.Calendars, calendar.CalendarInfo{
		ID: 2, CalendarID: "cal-lukas-2", Name: "AG Lukas", Panel: "school", Into: -1,
		PersonIDs: []string{"u-lukas"},
		Status:    calendar.SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"},
	})
	in.Calendar.Events = append(in.Calendar.Events, calendar.Event{
		Cal: 2, Calendar: "AG Lukas", CalendarID: "cal-lukas-2", Title: "AG-Termin",
		Start: at("2026-10-06 15:00"), End: at("2026-10-06 16:00"),
	})
	kids := BuildChildren(in, at("2026-10-02 07:00"), berlin, nil)
	ev := centralKid(kids, "lukas").Days[4].Events
	if len(ev) != 2 {
		t.Fatalf("all assigned calendars included: %+v", ev)
	}
	if ev[0].Title != "AG-Termin" || ev[1].Title != "Elternsprechtag" {
		t.Errorf("order: %+v", ev)
	}
}

func TestMissingSourceNeverBecomesFresh(t *testing.T) {
	in := centralInputs()
	in.Calendar.Calendars[0].Status = calendar.SourceStatus{Freshness: "neverLoaded", LastAttemptStatus: "error", Error: "500"}
	in.Calendar.Events = in.Calendar.Events[1:]
	kids := BuildChildren(in, at("2026-10-02 07:00"), berlin, nil)
	lukas := centralKid(kids, "lukas")
	if lukas.ChildSlug == "" {
		t.Fatal("child projection exists")
	}
	if !lukas.Incomplete {
		t.Error("required never-loaded source must mark the projection incomplete")
	}
	if lukas.SourceUpdatedAt != 0 {
		t.Errorf("sourceUpdatedAt = %d, want 0 (unknown, not now)", lukas.SourceUpdatedAt)
	}
	if len(lukas.Days[4].Events) != 0 {
		t.Errorf("no guessed events: %+v", lukas.Days[4].Events)
	}
	// the healthy child is unaffected
	if hannah := centralKid(kids, "hannah"); hannah.Incomplete || hannah.SourceUpdatedAt == 0 {
		t.Errorf("healthy child: %+v", hannah)
	}
}

func TestCentralIgnoresNumericExtrasAndNames(t *testing.T) {
	in := centralInputs()
	// numeric ENV indexes and timetable calendar names must not override bindings
	kids := BuildChildren(in, at("2026-10-02 07:00"), berlin, map[string][]int{"lukas": {1}})
	if ev := centralKid(kids, "lukas").Days[3].Events; len(ev) != 0 {
		t.Errorf("numeric extra must not leak hannah's calendar into lukas: %+v", ev)
	}
}

func TestUnmappedChildKeepsSchoolDataWithWarning(t *testing.T) {
	in := centralInputs()
	in.Calendar.Bindings = in.Calendar.Bindings[1:] // lukas' besteschule binding gone
	kids := BuildChildren(in, at("2026-10-02 07:00"), berlin, nil)
	lukas := centralKid(kids, "lukas")
	if lukas.ChildSlug == "" {
		t.Fatal("unmapped child keeps its projection")
	}
	if len(lukas.Days[0].Timetable) != 1 {
		t.Errorf("school data kept: %+v", lukas.Days[0].Timetable)
	}
	if len(lukas.Days[4].Events) != 0 {
		t.Errorf("no guessed calendars: %+v", lukas.Days[4].Events)
	}
	if lukas.Incomplete {
		t.Error("a child without assigned calendars is complete (nothing required)")
	}
	warns := CentralWarnings(in.Calendar, in.School, nil)
	if len(warns) != 1 {
		t.Fatalf("configuration warning: %v", warns)
	}
}

func TestPusherHoldsIncompleteCentralChild(t *testing.T) {
	rc := &recorder{}
	srv := httptest.NewServer(rc.handler(t))
	defer srv.Close()
	in := centralInputs()
	in.Calendar.Calendars[0].Status = calendar.SourceStatus{Freshness: "neverLoaded", LastAttemptStatus: "error", Error: "500"}
	in.Calendar.Events = in.Calendar.Events[1:]
	p := &Pusher{Client: NewClient(srv.URL, "ingest", "read"), Loc: berlin,
		Gather: func(time.Time) Inputs { return in }}
	p.Push(context.Background(), at("2026-10-02 07:00"))
	if got := strings.Join(rc.take(), " "); got != "/ingest/child:hannah" {
		t.Fatalf("incomplete child held back, healthy child sent: %s", got)
	}
}
