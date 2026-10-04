package briefing

import (
	"fmt"
	"strings"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/calendar"
	"familydash/internal/timetable"
	"familydash/internal/todo"
	"familydash/internal/vielfalt"
	"familydash/internal/waste"
	"familydash/internal/weather"
)

// Kind of briefing: the morning one is about today, the evening one about tomorrow.
type Kind string

const (
	Morning Kind = "morning"
	Evening Kind = "evening"
)

// Data is one snapshot of every source the briefing looks at. nil/empty = not configured.
type Data struct {
	Calendar   calendar.Snapshot
	Weather    *weather.Weather
	School     *besteschule.School
	Timetables []timetable.Card
	Meals      *vielfalt.Meals
	Waste      []waste.Pickup
	Todos      *todo.List
}

// Facts is everything the model gets – already computed and worded by code,
// so it only has to pick and phrase, never to calculate times or dates.
// German keys: the model writes German, and the prompt shows these as-is.
type Facts struct {
	Art          string   `json:"art"`
	Jetzt        string   `json:"jetzt"`
	Zieltag      string   `json:"zieltag"`
	Wochenende   bool     `json:"wochenende,omitempty"`
	Kinder       []Kid    `json:"kinder,omitempty"`
	HeuteAbend   []Appt   `json:"termine_heute_abend,omitempty"` // evening only: still coming up tonight
	Termine      []Appt   `json:"termine,omitempty"`
	Gleichzeitig []string `json:"zeitgleich,omitempty"`
	Muell        []string `json:"muell,omitempty"`
	Wetter       *Wx      `json:"wetter,omitempty"`
	Todos        []Todo   `json:"todos,omitempty"`
	// family app only
	Bestaetigungen int      `json:"bestaetigungen_offen,omitempty"`
	Punkte         []string `json:"punkte,omitempty"`
}

type Kid struct {
	Name         string     `json:"name"`
	Schule       *SchoolDay `json:"schule,omitempty"`
	Arbeiten     []string   `json:"arbeiten,omitempty"`
	Hausaufgaben []string   `json:"hausaufgaben_faellig,omitempty"`
	Essen        string     `json:"mittagessen,omitempty"`
}

type SchoolDay struct {
	Frei       bool     `json:"frei,omitempty"`
	Beginn     string   `json:"beginn,omitempty"`
	Ende       string   `json:"ende,omitempty"`
	Faecher    []string `json:"faecher,omitempty"`
	Ausfall    []string `json:"ausfall,omitempty"`
	Vertretung []string `json:"vertretung,omitempty"`
	Nachmittag []string `json:"nachmittag,omitempty"`
	Hinweise   []string `json:"hinweise,omitempty"`
	Sport      bool     `json:"sport,omitempty"`
}

type Appt struct {
	Kalender string `json:"kalender"`
	Titel    string `json:"titel"`
	Zeit     string `json:"zeit"` // "16:00–17:00" or "ganztägig"
	Ort      string `json:"ort,omitempty"`
}

type Wx struct {
	Wetterlage string `json:"wetterlage,omitempty"`
	Min        int    `json:"min_grad"`
	Max        int    `json:"max_grad"`
	RegenMax   int    `json:"regen_max_prozent"`
	RegenAb    string `json:"regen_ab,omitempty"` // first hour with ≥ 50 %
	Schulweg   string `json:"schulweg,omitempty"` // 07–08 o'clock in words
	Schnee     bool   `json:"schnee,omitempty"`
	Gewitter   bool   `json:"gewitter,omitempty"`
	Frost      bool   `json:"frost,omitempty"`
}

type Todo struct {
	Titel   string `json:"titel"`
	Wer     string `json:"wer,omitempty"`
	Faellig string `json:"faellig,omitempty"`
}

const ymdLayout = "2006-01-02"

var (
	weekdaysDE = []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	monthsDE   = []string{"", "Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}
)

func dayName(t time.Time) string {
	return fmt.Sprintf("%s, %d. %s", weekdaysDE[t.Weekday()], t.Day(), monthsDE[t.Month()])
}

// relDay names a date relative to today: "heute", "morgen", "übermorgen", "Montag (5.10.)".
func relDay(date string, today time.Time, loc *time.Location) string {
	d, err := time.ParseInLocation(ymdLayout, date, loc)
	if err != nil {
		return date
	}
	switch int(d.Sub(today).Round(24*time.Hour) / (24 * time.Hour)) {
	case 0:
		return "heute"
	case 1:
		return "morgen"
	case 2:
		return "übermorgen"
	}
	return fmt.Sprintf("%s (%d.%d.)", weekdaysDE[d.Weekday()], d.Day(), int(d.Month()))
}

func midnight(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Target is the day a briefing is about: today in the morning, tomorrow in the evening.
func Target(kind Kind, now time.Time, loc *time.Location) time.Time {
	d := midnight(now, loc)
	if kind == Evening {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// BuildFacts turns the raw source data into the fact sheet for one briefing
// about the day target (see Target).
func BuildFacts(kind Kind, now, target time.Time, loc *time.Location, d Data) Facts {
	now = now.In(loc)
	today := midnight(now, loc)
	target = midnight(target, loc)
	key := target.Format(ymdLayout)

	f := Facts{
		Art:        "Morgen-Briefing für heute",
		Jetzt:      fmt.Sprintf("%s, %s Uhr", dayName(now), now.Format("15:04")),
		Zieltag:    dayName(target),
		Wochenende: target.Weekday() == time.Saturday || target.Weekday() == time.Sunday,
	}
	if kind == Evening {
		f.Art = "Abend-Vorausschau auf morgen"
	}

	// children: beste.schule + fixed timetables, lunch and homework/exams by name
	var students []besteschule.Student
	if d.School != nil {
		students = append(students, d.School.Students...)
	}
	for _, c := range d.Timetables {
		students = append(students, c.Student)
	}
	var meals []vielfalt.Child
	if d.Meals != nil {
		meals = d.Meals.Children
	}
	usedMeal := map[int]bool{}
	for _, st := range students {
		k := Kid{Name: st.Name}
		if !f.Wochenende {
			k.Schule = schoolDay(st.Day, key)
		}
		for _, e := range st.Exams {
			if e.Date >= key && e.Date <= target.AddDate(0, 0, 3).Format(ymdLayout) {
				k.Arbeiten = append(k.Arbeiten, strings.TrimSpace(fmt.Sprintf("%s %s – %s", e.Subject, e.Kind, relDay(e.Date, today, loc))))
			}
		}
		for _, e := range st.Homework {
			if e.Date == key {
				k.Hausaufgaben = append(k.Hausaufgaben, joinNonEmpty(": ", e.Subject, e.Text))
			}
		}
		for i, m := range meals {
			if usedMeal[i] || !SameKid(m.Name, st.Name) {
				continue
			}
			usedMeal[i] = true
			k.Essen = mealOn(m, key, k.Schule)
		}
		f.Kinder = append(f.Kinder, k)
	}

	// appointments on the target day (+ tonight's in the evening)
	ends := target.AddDate(0, 0, 1)
	type timed struct {
		key        string // stable calendar identity; the display name is text only
		cal        string
		start, end time.Time
		title      string
	}
	calKey := func(e calendar.Event) string {
		if e.CalendarID != "" {
			return "\x00" + e.CalendarID
		}
		return e.Calendar
	}
	var spans []timed
	for _, e := range d.Calendar.Events {
		if e.AllDay {
			if e.StartDate <= key && key < e.EndDate {
				f.Termine = append(f.Termine, Appt{Kalender: e.Calendar, Titel: e.Title, Zeit: "ganztägig", Ort: e.Location})
			}
			continue
		}
		s, en := e.Start.In(loc), e.End.In(loc)
		if kind == Evening && s.After(now) && s.Before(target) {
			f.HeuteAbend = append(f.HeuteAbend, Appt{Kalender: e.Calendar, Titel: e.Title, Zeit: span(s, en), Ort: e.Location})
		}
		if s.Before(ends) && (en.After(target) || (s.Equal(en) && !s.Before(target))) {
			if kind == Morning && !en.After(now) && !s.Equal(en) {
				continue // already over this morning
			}
			f.Termine = append(f.Termine, Appt{Kalender: e.Calendar, Titel: e.Title, Zeit: span(s, en), Ort: e.Location})
			if en.After(s) {
				spans = append(spans, timed{calKey(e), e.Calendar, s, en, e.Title})
			}
		}
	}
	// same time, different calendars – maybe a pickup problem; the model decides.
	// Identity is the stable calendar id in central mode (a rename is not a
	// second calendar); the names stay visible in the text only.
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			a, b := spans[i], spans[j]
			if a.key != b.key && a.start.Before(b.end) && b.start.Before(a.end) {
				f.Gleichzeitig = append(f.Gleichzeitig, fmt.Sprintf("%s %s (%s) und %s %s (%s)",
					a.start.Format("15:04"), a.title, a.cal, b.start.Format("15:04"), b.title, b.cal))
			}
		}
	}

	// bins: in the evening the ones collected tomorrow (put out tonight)
	for _, p := range d.Waste {
		for _, date := range p.Dates {
			if date != key {
				continue
			}
			if kind == Evening {
				f.Muell = append(f.Muell, p.Name+": Abholung morgen früh, heute Abend rausstellen")
			} else {
				f.Muell = append(f.Muell, p.Name+": Abholung heute")
			}
		}
	}

	f.Wetter = weatherFacts(d.Weather, key, loc)

	// to-dos from the family app, per person: morning today's, evening what's
	// still open today plus tomorrow's
	if d.Todos != nil {
		appTodos(&f, kind, today, loc, d.Todos)
	}
	return f
}

const maxTodos = 8

// appTodos adds the family app's tasks: overdue first, then the target
// day's open ones; in the evening also what's still open today. Tasks a
// child ticked off and that only wait for a parent count as done here and
// show up as bestaetigungen_offen instead.
func appTodos(f *Facts, kind Kind, today time.Time, loc *time.Location, l *todo.List) {
	f.Bestaetigungen = l.Pending
	for _, p := range l.People {
		if p.Role == "child" {
			f.Punkte = append(f.Punkte, fmt.Sprintf("%s: %d Punkte", p.Name, p.Points))
		}
	}
	todayKey := today.Format(ymdLayout)
	add := func(t todo.Task, due string) bool {
		f.Todos = append(f.Todos, Todo{Titel: t.Title, Wer: t.Who, Faellig: due})
		return len(f.Todos) < maxTodos
	}
	var overdue, open []todo.Task
	for _, t := range l.Tasks {
		if t.Done || t.Pending {
			continue
		}
		if t.Deadline != "" && t.Deadline < todayKey {
			overdue = append(overdue, t)
		} else {
			open = append(open, t)
		}
	}
	for _, t := range overdue {
		if !add(t, "überfällig seit "+relDay(t.Deadline, today, loc)) {
			return
		}
	}
	due := "heute"
	if kind == Evening {
		due = "heute noch offen"
	}
	for _, t := range open {
		if !add(t, due) {
			return
		}
	}
	if kind == Evening {
		for _, t := range l.Tomorrow {
			if t.Pending {
				continue
			}
			if !add(t, "morgen") {
				return
			}
		}
	}
}

func schoolDay(day besteschule.Day, key string) *SchoolDay {
	sd := &SchoolDay{Hinweise: day.Notices}
	if day.NoSchool || day.Date != key || len(day.Lessons) == 0 {
		sd.Frei = true
		return sd
	}
	for _, l := range day.Lessons {
		name := l.Subject
		if strings.Contains(strings.ToLower(name), "sport") || strings.Contains(strings.ToLower(name), "schwimm") {
			sd.Sport = true
		}
		if l.Tag != "" {
			sd.Nachmittag = append(sd.Nachmittag, fmt.Sprintf("%s %s (%s–%s)", l.Tag, name, l.Start, l.End))
			continue
		}
		label := name
		if l.Nr > 0 {
			label = fmt.Sprintf("%d. Stunde %s", l.Nr, name)
		}
		switch l.Status {
		case "cancelled":
			sd.Ausfall = append(sd.Ausfall, label)
			continue
		case "substitution":
			sd.Vertretung = append(sd.Vertretung, joinNonEmpty(" – ", label, l.Info))
		}
		if sd.Beginn == "" {
			sd.Beginn = l.Start
		}
		if l.End != "" {
			sd.Ende = l.End
		}
		if !contains(sd.Faecher, name) {
			sd.Faecher = append(sd.Faecher, name)
		}
	}
	if sd.Beginn == "" && len(sd.Nachmittag) == 0 {
		sd.Frei = true // everything cancelled
	}
	return sd
}

func mealOn(m vielfalt.Child, key string, sd *SchoolDay) string {
	for _, day := range m.Days {
		if day.Date != key {
			continue
		}
		if len(day.Ordered) == 0 {
			return "nichts bestellt – Brotbox nötig"
		}
		names := make([]string, len(day.Ordered))
		for i, o := range day.Ordered {
			names[i] = o.Name
		}
		return strings.Join(names, ", ")
	}
	if sd != nil && !sd.Frei && m.Error == "" && len(m.Days) > 0 {
		return "nichts bestellt – Brotbox nötig"
	}
	return ""
}

func weatherFacts(w *weather.Weather, key string, loc *time.Location) *Wx {
	if w == nil || len(w.Daily) == 0 {
		return nil
	}
	var x *Wx
	for _, day := range w.Daily {
		if day.Date == key {
			x = &Wx{Wetterlage: day.Label, Min: round(day.Min), Max: round(day.Max), RegenMax: bucket(day.PrecipProb)}
			x.Schnee = day.Icon == "snow" || day.Icon == "sleet"
			x.Gewitter = day.Icon == "thunder"
		}
	}
	if x == nil {
		return nil
	}
	// hourly: daytime 07–19 overrides the daily rain, plus the way to school 07–08
	first := true
	var way []string
	for _, h := range w.Hourly {
		t := h.Time.In(loc)
		if t.Format(ymdLayout) != key || t.Hour() < 7 || t.Hour() >= 19 {
			continue
		}
		if first {
			x.RegenMax, first = 0, false
		}
		if p := bucket(h.PrecipProb); p > x.RegenMax {
			x.RegenMax = p
		}
		if h.PrecipProb >= 50 && x.RegenAb == "" {
			x.RegenAb = t.Format("15:04")
		}
		switch h.Icon {
		case "snow", "sleet":
			x.Schnee = true
		case "thunder":
			x.Gewitter = true
		}
		if t.Hour() == 7 {
			way = append(way, fmt.Sprintf("%d °C", round(h.Temp)))
			if h.PrecipProb >= 30 {
				way = append(way, fmt.Sprintf("%d %% Regen", bucket(h.PrecipProb)))
			}
		}
	}
	x.Schulweg = strings.Join(way, ", ")
	x.Frost = x.Min <= 0
	return x
}

func span(s, e time.Time) string {
	if !e.After(s) {
		return s.Format("15:04")
	}
	if e.Format(ymdLayout) != s.Format(ymdLayout) {
		return s.Format("15:04") + " (mehrtägig)"
	}
	return s.Format("15:04") + "–" + e.Format("15:04")
}

// bucket rounds a percentage to tens, so the forecast's small wiggles don't
// count as "new data" and trigger another model call.
func bucket(p int) int { return (p + 5) / 10 * 10 }

func round(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// SameKid mirrors sameKid in app.js: "Lukas" matches "lukas" and "Meister Lukas";
// two multi-word names must be equal.
func SameKid(a, b string) bool {
	wa, wb := strings.Fields(strings.ToLower(a)), strings.Fields(strings.ToLower(b))
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	if strings.Join(wa, " ") == strings.Join(wb, " ") {
		return true
	}
	short, long := wa, wb
	if len(wb) < len(wa) {
		short, long = wb, wa
	}
	return len(short) == 1 && contains(long, short[0])
}
