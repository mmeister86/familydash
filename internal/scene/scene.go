// Package scene decides which wall scene the display shows and when. Scene
// identities come from the Convex feed (or the SCENE_* fallback); the
// frontend switches its layout on the scene id.
package scene

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Built-in scene ids, kept stable so ?scene= and SCENE_FORCE keep working.
const (
	Morning   = "morning"
	Day       = "day"
	Afternoon = "afternoon"
	Evening   = "evening"
	Night     = "night"
)

// FallbackScene describes one built-in scene for the SCENE_* fallback.
type FallbackScene struct {
	ID, Name, Kind string
}

// FallbackScenes lists the built-in scenes in day order.
var FallbackScenes = []FallbackScene{
	{Morning, "Morgen", "layout"},
	{Day, "Tag", "layout"},
	{Afternoon, "Nachmittag", "layout"},
	{Evening, "Abend", "layout"},
	{Night, "Nacht", "night"},
}

// Start is one scene start: the minute of the day (0…1439) it begins,
// with the scene identity.
type Start struct {
	ID, Name, Kind string
	Minute         int
}

// Schedule holds the start times for school days (Mon–Fri) and for weekends.
// A scene missing from a list is skipped that day (the previous one runs on).
type Schedule struct {
	SchoolDay []Start
	Weekend   []Start
	Force     string // SCENE_FORCE: always this scene id (for testing), "" = off
}

// Defaults: SCENE_<NAME> for school days, SCENE_<NAME>_WEEKEND for Sat/Sun.
var (
	DefaultSchoolDay = map[string]string{Morning: "06:45", Day: "09:00", Afternoon: "14:00", Evening: "19:00", Night: "21:30"}
	DefaultWeekend   = map[string]string{Morning: "07:30", Day: "10:00"} // others fall back to the school-day time
)

// Current is what /api/dashboard reports.
type Current struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Since     time.Time `json:"since"`
	Until     time.Time `json:"until"`
	SchoolDay bool      `json:"schoolDay"` // Mon–Fri; public holidays/vacations: TODO
	Forced    bool      `json:"forced,omitempty"`
}

// ParseClock parses "HH:MM" (also "H:MM", "HH.MM") into minutes of the day.
// "off"/"-" returns -1: the scene is skipped.
func ParseClock(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "off" || s == "-" || s == "aus" {
		return -1, nil
	}
	s = strings.ReplaceAll(s, ".", ":")
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		m = "0"
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("invalid time %q (want HH:MM or off)", s)
	}
	return hh*60 + mm, nil
}

// Build turns id→"HH:MM" maps into a sorted list. get returns the raw
// value for a scene id (already merged with defaults by the caller).
func Build(get func(id string) string) ([]Start, error) {
	var out []Start
	for _, scene := range FallbackScenes {
		v := get(scene.ID)
		if v == "" {
			continue
		}
		m, err := ParseClock(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", scene.ID, err)
		}
		if m >= 0 {
			out = append(out, Start{ID: scene.ID, Name: scene.Name, Kind: scene.Kind, Minute: m})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Minute < out[j].Minute })
	return out, nil
}

// IsSchoolDay: Monday to Friday. Holidays and vacations come later
// (beste.schule "noSchool" or the Feiertage calendar).
func IsSchoolDay(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func (s *Schedule) startsFor(day time.Time) []Start {
	if IsSchoolDay(day) {
		return s.SchoolDay
	}
	return s.Weekend
}

func (s *Schedule) known(id string) (Start, bool) {
	for _, list := range [][]Start{s.SchoolDay, s.Weekend} {
		for _, st := range list {
			if st.ID == id {
				return st, true
			}
		}
	}
	return Start{}, false
}

// at returns the wall-clock time `minute` on the calendar day of `day`.
// time.Date normalises DST gaps, so 02:30 on the spring-forward day is fine.
func at(day time.Time, minute int, loc *time.Location) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, minute/60, minute%60, 0, 0, loc)
}

// At returns the scene active at now. If a day has no scenes configured at
// all, it is one endless "day".
func (s *Schedule) At(now time.Time, loc *time.Location) Current {
	now = now.In(loc)
	cur := Current{SchoolDay: IsSchoolDay(now)}

	today := s.startsFor(now)
	// the last start today that has already happened …
	idx := -1
	for i, st := range today {
		if !at(now, st.Minute, loc).After(now) {
			idx = i
		}
	}
	if idx >= 0 {
		st := today[idx]
		cur.ID, cur.Name, cur.Kind, cur.Since = st.ID, st.Name, st.Kind, at(now, st.Minute, loc)
	} else {
		// … or, before the first start, whatever ran last on an earlier day
		// (normally last night's "night")
		for back := 1; back <= 7 && cur.ID == ""; back++ {
			prev := now.AddDate(0, 0, -back)
			if list := s.startsFor(prev); len(list) > 0 {
				last := list[len(list)-1]
				cur.ID, cur.Name, cur.Kind, cur.Since = last.ID, last.Name, last.Kind, at(prev, last.Minute, loc)
			}
		}
	}

	// next start: later today, or the first one on a following day
	if idx+1 < len(today) {
		cur.Until = at(now, today[idx+1].Minute, loc)
	} else {
		for fwd := 1; fwd <= 7; fwd++ {
			next := now.AddDate(0, 0, fwd)
			if list := s.startsFor(next); len(list) > 0 {
				cur.Until = at(next, list[0].Minute, loc)
				break
			}
		}
	}

	if cur.ID == "" {
		cur.ID = Day
		for _, fb := range FallbackScenes {
			if fb.ID == Day {
				cur.Name, cur.Kind = fb.Name, fb.Kind
			}
		}
	}
	if s.Force != "" {
		if st, ok := s.known(s.Force); ok {
			cur.ID, cur.Name, cur.Kind, cur.Forced = st.ID, st.Name, st.Kind, true
		}
		// Unknown force ids stay silent here (warned once per schedule
		// swap); the time-based scene runs instead of failing the wall.
	}
	return cur
}

// Provider serves the current scene from a hot-swappable schedule: the wall
// follows feed updates without a restart.
type Provider struct {
	mu    sync.RWMutex
	sched Schedule
}

// NewProvider serves sched; an unknown initial force warns once.
func NewProvider(sched Schedule) *Provider {
	p := &Provider{}
	p.SetSchedule(Schedule{SchoolDay: sched.SchoolDay, Weekend: sched.Weekend})
	p.SetForce(sched.Force)
	return p
}

// SetSchedule swaps the schedule, keeping the force. An unknown force warns
// (once per swap) and stays set: a later feed may still provide that scene.
func (p *Provider) SetSchedule(sched Schedule) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sched.Force = p.sched.Force
	p.sched = sched
	p.warnUnknownForceLocked()
}

// SetForce pins one scene id; unknown ids warn and are ignored by At until
// a schedule provides them.
func (p *Provider) SetForce(force string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sched.Force = force
	p.warnUnknownForceLocked()
}

func (p *Provider) warnUnknownForceLocked() {
	if p.sched.Force == "" {
		return
	}
	if _, ok := p.sched.known(p.sched.Force); !ok {
		slog.Warn("scene force matches no known scene, ignoring", "force", p.sched.Force)
	}
}

// At returns the scene active at now.
func (p *Provider) At(now time.Time, loc *time.Location) Current {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sched.At(now, loc)
}
