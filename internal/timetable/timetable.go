// Package timetable shows a fixed weekly school timetable for a child whose
// school has no beste.schule account. The plan lives in a small JSON file:
// the embedded stundenplan.json by default, or TIMETABLE_FILE to override it.
// Each child renders as the same card as a beste.schule student.
package timetable

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"familydash/internal/besteschule"
)

//go:embed stundenplan.json
var embedded []byte

type File struct {
	Children []Child `json:"children"`
}

type Child struct {
	Name string `json:"name"`
	// Calendar is the name of a CALENDAR_n_PANEL=school calendar; its entries
	// are shown at the bottom of this child's card instead of a card of its own.
	Calendar  string            `json:"calendar,omitempty"`
	ValidFrom string            `json:"validFrom,omitempty"` // YYYY-MM-DD
	ValidTo   string            `json:"validTo,omitempty"`
	Periods   [][2]string       `json:"periods"` // lesson n (1-based) → [start, end]
	Days      map[string][]Slot `json:"days"`    // "Mo" … "Fr" → subject per lesson, "" = free
	// WeekA is any date in an A week. Lessons written as {"A": …, "B": …}
	// alternate weekly from there (calendar weeks, holidays included).
	WeekA    string  `json:"weekA,omitempty"`
	Extra    []Extra `json:"extra,omitempty"`
	NoSchool []Range `json:"noSchool,omitempty"`
}

// Slot is one lesson: a plain subject ("Mathe"), or {"A": "Werken", "B": "Kunst"}
// for a subject that alternates between A and B weeks ("" = free that week).
type Slot struct {
	A, B string
}

func (s *Slot) UnmarshalJSON(b []byte) error {
	var plain string
	if err := json.Unmarshal(b, &plain); err == nil {
		s.A, s.B = plain, plain
		return nil
	}
	var ab struct{ A, B string }
	if err := json.Unmarshal(b, &ab); err != nil {
		return fmt.Errorf("Stunde muss \"Fach\" oder {\"A\": …, \"B\": …} sein: %s", b)
	}
	s.A, s.B = ab.A, ab.B
	return nil
}

func (s Slot) alternates() bool { return s.A != s.B }

// Extra is something outside the numbered lessons, e.g. an afternoon club.
type Extra struct {
	Day     string `json:"day"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Subject string `json:"subject"`
	Tag     string `json:"tag,omitempty"`
	Room    string `json:"room,omitempty"`
}

type Range struct {
	From string `json:"from"`
	To   string `json:"to"`
	Name string `json:"name,omitempty"`
}

// Card is what /api/dashboard returns per child.
type Card struct {
	besteschule.Student
	Calendar string `json:"calendar,omitempty"`
}

const lookahead = 31 // days to search for the next school day (covers any holiday)

var weekdays = map[string]time.Weekday{
	"so": time.Sunday, "mo": time.Monday, "di": time.Tuesday, "mi": time.Wednesday,
	"do": time.Thursday, "fr": time.Friday, "sa": time.Saturday,
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func weekdayOf(s string) (time.Weekday, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 3 {
		s = s[:2] // "Montag" → "mo", "Dienstag" → "di"
		if wd, ok := weekdays[s]; ok {
			return wd, true
		}
	}
	wd, ok := weekdays[s]
	return wd, ok
}

// Load reads path, or the embedded plan when path is empty.
func Load(path string) (*File, error) {
	data := embedded
	if path != "" {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return nil, err
		}
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("stundenplan: %w", err)
	}
	for _, c := range f.Children {
		for k, slots := range c.Days {
			if _, ok := weekdayOf(k); !ok {
				return nil, fmt.Errorf("stundenplan %s: unbekannter Wochentag %q", c.Name, k)
			}
			for _, sl := range slots {
				if sl.alternates() && c.WeekA == "" {
					return nil, fmt.Errorf("stundenplan %s: A/B-Stunden brauchen \"weekA\"", c.Name)
				}
			}
		}
		if c.WeekA != "" {
			if _, err := time.Parse("2006-01-02", c.WeekA); err != nil {
				return nil, fmt.Errorf("stundenplan %s: weekA: %w", c.Name, err)
			}
		}
		for _, e := range c.Extra {
			if _, ok := weekdayOf(e.Day); !ok {
				return nil, fmt.Errorf("stundenplan %s: unbekannter Wochentag %q", c.Name, e.Day)
			}
		}
	}
	return &f, nil
}

// Build returns one card per child for time now.
func (f *File) Build(now time.Time, loc *time.Location) []Card {
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	out := make([]Card, 0, len(f.Children))
	for _, c := range f.Children {
		out = append(out, Card{
			Student: besteschule.Student{
				ID:       "plan-" + strings.ToLower(c.Name),
				Name:     c.Name,
				Day:      c.pickDay(now, today),
				Homework: []besteschule.Entry{},
				Exams:    []besteschule.Entry{},
			},
			Calendar: c.Calendar,
		})
	}
	return out
}

// pickDay mirrors beste.schule: today while school runs, then the next school day.
func (c *Child) pickDay(now, today time.Time) besteschule.Day {
	for i := 0; i < lookahead; i++ {
		d := today.AddDate(0, 0, i)
		lessons := c.lessonsOn(d)
		if len(lessons) == 0 {
			continue
		}
		if i == 0 && !now.Before(clock(d, lastEnd(lessons))) {
			continue // school's out – show the next school day
		}
		day := besteschule.Day{Date: d.Format("2006-01-02"), Lessons: lessons}
		if r := c.holiday(today); r != nil && i > 0 {
			day.Notices = []string{holidayNotice(r)}
		}
		return day
	}
	day := besteschule.Day{Date: today.Format("2006-01-02"), Lessons: []besteschule.Lesson{}, NoSchool: true}
	if r := c.holiday(today); r != nil {
		day.Notices = []string{holidayNotice(r)}
	}
	return day
}

func (c *Child) lessonsOn(d time.Time) []besteschule.Lesson {
	date := d.Format("2006-01-02")
	if (c.ValidFrom != "" && date < c.ValidFrom) || (c.ValidTo != "" && date > c.ValidTo) || c.holiday(d) != nil {
		return nil
	}
	var out []besteschule.Lesson
	bWeek := c.isBWeek(d)
	for key, subjects := range c.Days {
		if wd, _ := weekdayOf(key); wd != d.Weekday() {
			continue
		}
		for i, sl := range subjects {
			s := sl.A
			if bWeek {
				s = sl.B
			}
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			l := besteschule.Lesson{Nr: i + 1, Subject: s}
			if i < len(c.Periods) {
				l.Start, l.End = c.Periods[i][0], c.Periods[i][1]
			}
			out = append(out, l)
		}
	}
	for _, e := range c.Extra {
		if wd, _ := weekdayOf(e.Day); wd == d.Weekday() {
			out = append(out, besteschule.Lesson{Start: e.Start, End: e.End, Subject: e.Subject, Room: e.Room, Tag: e.Tag})
		}
	}
	sortLessons(out)
	return out
}

// isBWeek counts whole weeks between the Monday of WeekA and d's Monday.
func (c *Child) isBWeek(d time.Time) bool {
	if c.WeekA == "" {
		return false
	}
	a, err := time.ParseInLocation("2006-01-02", c.WeekA, d.Location())
	if err != nil {
		return false
	}
	monday := func(t time.Time) time.Time {
		t = time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, t.Location()) // noon: DST-safe
		return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
	}
	weeks := int(monday(d).Sub(monday(a)).Hours()/24+0.5) / 7
	return weeks%2 != 0
}

func sortLessons(ls []besteschule.Lesson) {
	for i := 1; i < len(ls); i++ { // tiny lists – insertion sort by start time
		for j := i; j > 0 && ls[j].Start < ls[j-1].Start; j-- {
			ls[j], ls[j-1] = ls[j-1], ls[j]
		}
	}
}

func (c *Child) holiday(d time.Time) *Range {
	date := d.Format("2006-01-02")
	for i := range c.NoSchool {
		r := &c.NoSchool[i]
		to := r.To
		if to == "" {
			to = r.From
		}
		if r.From <= date && date <= to {
			return r
		}
	}
	return nil
}

func holidayNotice(r *Range) string {
	name := r.Name
	if name == "" {
		name = "Schulfrei"
	}
	to := r.To
	if to == "" {
		to = r.From
	}
	if t, err := time.Parse("2006-01-02", to); err == nil && to != r.From {
		return fmt.Sprintf("%s bis %s", name, t.Format("02.01."))
	}
	return name
}

func lastEnd(ls []besteschule.Lesson) string {
	last := ""
	for _, l := range ls {
		if l.End > last {
			last = l.End
		}
	}
	return last
}

func clock(d time.Time, hm string) time.Time {
	h, m := 16, 0
	if len(hm) == 5 {
		fmt.Sscanf(hm, "%d:%d", &h, &m)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, d.Location())
}

// Days returns each child's timetable for n days from the day of from on,
// one entry per day (empty lessons on weekends and holidays, with the
// holiday as notice). Keyed by child name. Used for the family app, which
// shows a whole week instead of "today or the next school day".
func (f *File) Days(from time.Time, n int, loc *time.Location) map[string][]besteschule.Day {
	from = from.In(loc)
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	out := make(map[string][]besteschule.Day, len(f.Children))
	for i := range f.Children {
		c := &f.Children[i]
		days := make([]besteschule.Day, 0, n)
		for j := 0; j < n; j++ {
			d := start.AddDate(0, 0, j)
			day := besteschule.Day{Date: d.Format("2006-01-02"), Lessons: c.lessonsOn(d)}
			if day.Lessons == nil {
				day.Lessons = []besteschule.Lesson{}
			}
			if r := c.holiday(d); r != nil {
				day.Notices = []string{holidayNotice(r)}
			}
			days = append(days, day)
		}
		out[c.Name] = days
	}
	return out
}
