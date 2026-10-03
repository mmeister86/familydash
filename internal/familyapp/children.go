package familyapp

import (
	"sort"
	"strings"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/briefing"
	"familydash/internal/calendar"
	"familydash/internal/timetable"
	"familydash/internal/vielfalt"
)

// Days is how many days of timetable, appointments and lunch go to the app.
const Days = 7

// ChildSnapshot is the body of POST /ingest/child. The app replaces the
// child's document with it. Field names and optional fields follow the
// Convex validators in docs/FAMILY_APP.md – optional ones are omitted, never null.
type ChildSnapshot struct {
	ChildSlug       string     `json:"childSlug"`
	Days            []ChildDay `json:"days"`
	Homework        []Homework `json:"homework"`
	Exams           []Exam     `json:"exams"`
	SourceUpdatedAt int64      `json:"sourceUpdatedAt"` // ms; oldest of the sources behind it
	// Incomplete is local-only (never transmitted): a required central
	// calendar source has no valid data yet, so this projection must not
	// be pushed over the last good one in the app. SourceUpdatedAt is 0
	// while incomplete — explicitly unknown, never "now".
	Incomplete bool `json:"-"`
}

type ChildDay struct {
	Date      string   `json:"date"`
	Notices   []string `json:"notices,omitempty"`
	Timetable []Lesson `json:"timetable"`
	Events    []Event  `json:"events"`
	Meal      *Meal    `json:"meal,omitempty"`
}

type Lesson struct {
	Period  int     `json:"period"` // 0 = outside the numbered lessons (e.g. afternoon club)
	Start   string  `json:"start,omitempty"`
	End     string  `json:"end,omitempty"`
	Subject string  `json:"subject"`
	Room    string  `json:"room,omitempty"`
	Teacher string  `json:"teacher,omitempty"`
	Tag     string  `json:"tag,omitempty"` // e.g. "GTA"
	Change  *Change `json:"change,omitempty"`
}

type Change struct {
	Type string `json:"type"` // cancelled | substitution | roomChange | other
	Note string `json:"note,omitempty"`
}

type Event struct {
	Title    string `json:"title"`
	Start    string `json:"start"`         // RFC 3339, or YYYY-MM-DD when allDay
	End      string `json:"end,omitempty"` // RFC 3339, or exclusive YYYY-MM-DD when allDay
	AllDay   bool   `json:"allDay"`
	Calendar string `json:"calendar,omitempty"`
	Location string `json:"location,omitempty"`
}

// Meal is set on delivery days only. Ordered false = nothing ordered.
type Meal struct {
	Ordered     bool   `json:"ordered"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type Homework struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
	DueDate string `json:"dueDate"`
}

type Exam struct {
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Text    string `json:"text,omitempty"`
}

// Inputs is one snapshot of the sources a child's week is built from.
type Inputs struct {
	Calendar   calendar.Snapshot
	School     *besteschule.School
	SchoolDays map[string][]besteschule.Day // by student name, Days days from today
	Timetables []timetable.Card
	// PlanDaysByID is PlanDays keyed by stable timetable child id (see
	// timetable.File.DaysByID). Central mode prefers it so a rename cannot
	// move a week to the wrong child; producers that only fill PlanDays
	// keep working through the name fallback.
	PlanDaysByID map[string][]besteschule.Day // by stable timetable child id
	PlanDays     map[string][]besteschule.Day // by child name (compat fallback)
	Meals        *vielfalt.Meals
}

// BuildChildren makes one snapshot per child, matched by first name the same
// way the wall's cards do: beste.schule students and fixed timetables, their
// school calendar (CALENDAR_n_PANEL=school, or the timetable's "calendar"),
// extra calendars from FAMILY_APP_<SLUG>_CALENDARS (indexes into the
// calendar list) and their VielfaltMenü account.
//
// In central mode every person and calendar resolves through explicit
// stable bindings (calendar.PersonForSource): renames and reorders cannot
// move data, numeric extra calendars and name matching do not override
// bindings, and same-name source children never merge. Unmapped children
// keep their school data without guessed calendars; see CentralWarnings.
func BuildChildren(in Inputs, now time.Time, loc *time.Location, extraCals map[string][]int) []ChildSnapshot {
	if in.Calendar.Central {
		return buildChildrenCentral(in, now, loc)
	}
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	type kid struct {
		st       besteschule.Student
		days     []besteschule.Day
		calName  string
		fromPlan bool
	}
	var kids []kid
	if in.School != nil {
		for _, st := range in.School.Students {
			kids = append(kids, kid{st: st, days: in.SchoolDays[st.Name]})
		}
	}
	for _, c := range in.Timetables {
		kids = append(kids, kid{st: c.Student, days: in.PlanDays[c.Name], calName: c.Calendar, fromPlan: true})
	}

	var meals []vielfalt.Child
	if in.Meals != nil {
		meals = in.Meals.Children
	}
	usedCal, usedMeal, seen := map[int]bool{}, map[int]bool{}, map[string]bool{}
	var out []ChildSnapshot
	for _, k := range kids {
		slug := Slug(k.st.Name)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true

		cals := map[int]bool{}
		for _, c := range in.Calendar.Calendars {
			if c.Panel != "school" || usedCal[c.ID] {
				continue
			}
			if (k.calName != "" && strings.EqualFold(c.Name, k.calName)) || (k.calName == "" && briefing.SameKid(c.Name, k.st.Name)) {
				usedCal[c.ID], cals[c.ID] = true, true
				break
			}
		}
		for _, id := range extraCals[slug] {
			cals[id] = true
		}

		var meal *vielfalt.Child
		for i := range meals {
			if !usedMeal[i] && briefing.SameKid(meals[i].Name, k.st.Name) {
				usedMeal[i], meal = true, &meals[i]
				break
			}
		}

		// staleness: the oldest source behind this child (fixed timetables never go stale)
		var oldest time.Time
		older := func(t time.Time) {
			if !t.IsZero() && (oldest.IsZero() || t.Before(oldest)) {
				oldest = t
			}
		}
		if !k.fromPlan && in.School != nil {
			older(in.School.UpdatedAt)
		}
		if len(cals) > 0 {
			older(in.Calendar.UpdatedAt)
		}
		if meal != nil {
			older(meal.UpdatedAt)
		}
		if oldest.IsZero() {
			oldest = now
		}

		snap := ChildSnapshot{
			ChildSlug:       slug,
			Days:            make([]ChildDay, 0, Days),
			Homework:        homework(k.st.Homework),
			Exams:           exams(k.st.Exams),
			SourceUpdatedAt: oldest.UnixMilli(),
		}
		byDate := map[string]besteschule.Day{}
		for _, d := range k.days {
			byDate[d.Date] = d
		}
		for i := 0; i < Days; i++ {
			d := today.AddDate(0, 0, i)
			key := d.Format("2006-01-02")
			sd := byDate[key]
			cd := ChildDay{Date: key, Notices: sd.Notices, Timetable: lessons(sd.Lessons),
				Events: events(in.Calendar, cals, d, loc), Meal: mealOn(meal, key)}
			snap.Days = append(snap.Days, cd)
		}
		out = append(out, snap)
	}
	return out
}

// buildChildrenCentral makes one snapshot per bound person plus one per
// unmapped source child. Identity comes from explicit bindings only:
// besteschule students resolve through ("besteschule", student id),
// timetable cards through ("timetable", stable card id). The slug of a
// bound child is the bound user's slug; numeric extra calendars, calendar
// names and first-name matching never apply. A bound child sees events
// from every school calendar assigned to its person. Unmapped children
// keep their school data with no calendars and a configuration warning.
//
// A child's calendar contribution to SourceUpdatedAt is the oldest real
// success time of its assigned school calendars. When a required assigned
// calendar never loaded, the projection is incomplete with a zero
// timestamp — explicitly unknown, never "now" — and the pusher holds it
// back until first valid data arrives.
func buildChildrenCentral(in Inputs, now time.Time, loc *time.Location) []ChildSnapshot {
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	type kid struct {
		st       besteschule.Student
		days     []besteschule.Day
		personID string // "" when unmapped
		slug     string
		fromPlan bool
	}
	var kids []kid
	if in.School != nil {
		for _, st := range in.School.Students {
			k := kid{st: st, days: in.SchoolDays[st.Name]}
			if b, ok := calendar.PersonForSource(in.Calendar, "besteschule", st.ID); ok {
				k.personID = b.PersonID
				if p, found := calendar.PersonByID(in.Calendar, b.PersonID); found {
					k.slug = p.Slug
				}
			}
			if k.slug == "" {
				k.slug = Slug(st.Name)
			}
			kids = append(kids, k)
		}
	}
	for _, c := range in.Timetables {
		days := in.PlanDaysByID[c.Student.ID]
		if days == nil {
			days = in.PlanDays[c.Name] // compat: producers still keying by name
		}
		k := kid{st: c.Student, days: days, fromPlan: true}
		if b, ok := calendar.PersonForSource(in.Calendar, "timetable", c.Student.ID); ok {
			k.personID = b.PersonID
			if p, found := calendar.PersonByID(in.Calendar, b.PersonID); found {
				k.slug = p.Slug
			}
		}
		if k.slug == "" {
			k.slug = Slug(c.Student.Name)
		}
		kids = append(kids, k)
	}

	// numeric id of every school calendar assigned to the person; never by name
	calsFor := func(personID string) map[int]bool {
		cals := map[int]bool{}
		if personID == "" {
			return cals
		}
		want := map[string]bool{}
		for _, c := range calendar.CalendarsForPerson(in.Calendar, personID) {
			if c.Panel == "school" {
				want[c.CalendarID] = true
			}
		}
		for _, c := range in.Calendar.Calendars {
			if c.Panel == "school" && want[c.CalendarID] {
				cals[c.ID] = true
			}
		}
		return cals
	}

	var meals []vielfalt.Child
	if in.Meals != nil {
		meals = in.Meals.Children
	}
	usedMeal := map[int]bool{}
	seen, seenSlug := map[string]bool{}, map[string]bool{}
	var out []ChildSnapshot
	for _, k := range kids {
		dkey := "p:" + k.personID
		if k.personID == "" {
			kind, ext := "besteschule", k.st.ID
			if k.fromPlan {
				kind = "timetable"
			}
			dkey = "u:" + kind + ":" + ext
		}
		if k.slug == "" || seen[dkey] {
			continue // never merge same-name source children; same source twice is one child
		}
		seen[dkey] = true
		if seenSlug[k.slug] {
			continue // slugs collide: keep the first projection, warn (see CentralWarnings)
		}
		seenSlug[k.slug] = true

		cals := calsFor(k.personID)

		var meal *vielfalt.Child
		for i := range meals {
			if !usedMeal[i] && briefing.SameKid(meals[i].Name, k.st.Name) {
				usedMeal[i], meal = true, &meals[i]
				break
			}
		}

		var oldest time.Time
		older := func(t time.Time) {
			if !t.IsZero() && (oldest.IsZero() || t.Before(oldest)) {
				oldest = t
			}
		}
		incomplete := false
		if !k.fromPlan && in.School != nil {
			older(in.School.UpdatedAt)
		}
		if k.personID != "" {
			if calOldest, missing := calendar.AssignedCalendarState(in.Calendar, k.personID, "school"); len(missing) > 0 {
				incomplete = true
			} else {
				older(calOldest)
			}
		}
		if meal != nil {
			older(meal.UpdatedAt)
		}

		snap := ChildSnapshot{
			ChildSlug:       k.slug,
			Days:            make([]ChildDay, 0, Days),
			Homework:        homework(k.st.Homework),
			Exams:           exams(k.st.Exams),
			SourceUpdatedAt: 0,
			Incomplete:      incomplete,
		}
		if !incomplete {
			if oldest.IsZero() {
				oldest = now // fixed timetables never go stale
			}
			snap.SourceUpdatedAt = oldest.UnixMilli()
		}
		byDate := map[string]besteschule.Day{}
		for _, d := range k.days {
			byDate[d.Date] = d
		}
		for i := 0; i < Days; i++ {
			d := today.AddDate(0, 0, i)
			key := d.Format("2006-01-02")
			sd := byDate[key]
			cd := ChildDay{Date: key, Notices: sd.Notices, Timetable: lessons(sd.Lessons),
				Events: events(in.Calendar, cals, d, loc), Meal: mealOn(meal, key)}
			snap.Days = append(snap.Days, cd)
		}
		out = append(out, snap)
	}
	return out
}

// CentralWarnings lists German configuration warnings for central mode:
// source children without an explicit person binding (they keep their
// school data but get no calendars), and slug collisions where only the
// first projection is kept. Empty outside central mode.
func CentralWarnings(cal calendar.Snapshot, school *besteschule.School, plan []timetable.Card) []string {
	if !cal.Central {
		return nil
	}
	var warns []string
	seen := map[string]bool{}
	warn := func(s string) {
		if !seen[s] {
			seen[s] = true
			warns = append(warns, s)
		}
	}
	slugs := map[string]bool{}
	noteSlug := func(slug string) {
		if slug == "" {
			return
		}
		if slugs[slug] {
			warn("Doppelte Kennung „" + slug + "“ – nur die erste Zuordnung wird übertragen")
			return
		}
		slugs[slug] = true
	}
	if school != nil {
		for _, st := range school.Students {
			slug := Slug(st.Name)
			if b, ok := calendar.PersonForSource(cal, "besteschule", st.ID); ok {
				if p, found := calendar.PersonByID(cal, b.PersonID); found {
					slug = p.Slug
				}
				noteSlug(slug)
				continue
			}
			warn("Schule: „" + st.Name + "“ ist keiner Person zugeordnet – keine Termine")
			noteSlug(slug)
		}
	}
	for _, c := range plan {
		slug := Slug(c.Student.Name)
		if b, ok := calendar.PersonForSource(cal, "timetable", c.Student.ID); ok {
			if p, found := calendar.PersonByID(cal, b.PersonID); found {
				slug = p.Slug
			}
			noteSlug(slug)
			continue
		}
		warn("Stundenplan: „" + c.Student.Name + "“ ist keiner Person zugeordnet – keine Termine")
		noteSlug(slug)
	}
	return warns
}

// Slug turns a name into the app's user slug: first word, lower case,
// umlauts spelled out ("Lukas Meister" → "lukas", "Jürgen" → "juergen").
func Slug(name string) string {
	f := strings.Fields(strings.ToLower(name))
	if len(f) == 0 {
		return ""
	}
	r := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	var b strings.Builder
	for _, c := range r.Replace(f[0]) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func lessons(ls []besteschule.Lesson) []Lesson {
	out := make([]Lesson, 0, len(ls))
	for _, l := range ls {
		le := Lesson{Period: l.Nr, Start: l.Start, End: l.End, Subject: l.Subject, Room: l.Room, Teacher: l.Teacher, Tag: l.Tag}
		switch l.Status {
		case "cancelled", "substitution":
			le.Change = &Change{Type: l.Status, Note: l.Info}
		}
		out = append(out, le)
	}
	return out
}

func events(snap calendar.Snapshot, cals map[int]bool, day time.Time, loc *time.Location) []Event {
	out := []Event{}
	if len(cals) == 0 {
		return out
	}
	key := day.Format("2006-01-02")
	next := day.AddDate(0, 0, 1)
	for _, e := range snap.Events {
		if !cals[e.Cal] {
			continue
		}
		if e.AllDay {
			if e.StartDate <= key && key < e.EndDate {
				out = append(out, Event{Title: e.Title, Start: e.StartDate, End: e.EndDate, AllDay: true, Calendar: e.Calendar, Location: e.Location})
			}
			continue
		}
		s, en := e.Start.In(loc), e.End.In(loc)
		if s.Before(next) && (en.After(day) || (!en.After(s) && !s.Before(day))) {
			ev := Event{Title: e.Title, Start: s.Format(time.RFC3339), Calendar: e.Calendar, Location: e.Location}
			if en.After(s) {
				ev.End = en.Format(time.RFC3339)
			}
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		return out[i].Start < out[j].Start
	})
	return out
}

func mealOn(m *vielfalt.Child, key string) *Meal {
	if m == nil {
		return nil
	}
	for _, d := range m.Days {
		if d.Date != key {
			continue
		}
		if len(d.Ordered) == 0 {
			return &Meal{Ordered: false}
		}
		var names, sides []string
		for _, o := range d.Ordered {
			names = append(names, o.Name)
			if o.Side != "" {
				sides = append(sides, o.Side)
			}
		}
		return &Meal{Ordered: true, Title: strings.Join(names, " / "), Description: strings.Join(sides, " / ")}
	}
	return nil
}

func homework(es []besteschule.Entry) []Homework {
	out := make([]Homework, 0, len(es))
	for _, e := range es {
		if e.Date == "" {
			continue
		}
		out = append(out, Homework{Subject: e.Subject, Text: e.Text, DueDate: e.Date})
	}
	return out
}

func exams(es []besteschule.Entry) []Exam {
	out := make([]Exam, 0, len(es))
	for _, e := range es {
		if e.Date == "" {
			continue
		}
		text := strings.TrimSpace(strings.TrimSpace(e.Kind) + " " + strings.TrimSpace(e.Text))
		out = append(out, Exam{Subject: e.Subject, Date: e.Date, Text: text})
	}
	return out
}
