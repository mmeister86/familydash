// Package scene decides which time-of-day layout ("scene") the wall display
// shows: morning, day, afternoon, evening or night. The frontend switches
// its layout and focus zone on the scene name.
package scene

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Scene names, in the order they happen during a day.
const (
	Morning   = "morning"
	Day       = "day"
	Afternoon = "afternoon"
	Evening   = "evening"
	Night     = "night"
)

// Names lists all scenes in day order.
var Names = []string{Morning, Day, Afternoon, Evening, Night}

// Start is the minute of the day (0…1439) a scene begins.
type Start struct {
	Name   string
	Minute int
}

// Schedule holds the start times for school days (Mon–Fri) and for weekends.
// A scene missing from a list is skipped that day (the previous one runs on).
type Schedule struct {
	SchoolDay []Start
	Weekend   []Start
	Force     string // SCENE_FORCE: always this scene (for testing), "" = off
}

// Defaults: SCENE_<NAME> for school days, SCENE_<NAME>_WEEKEND for Sat/Sun.
var (
	DefaultSchoolDay = map[string]string{Morning: "06:00", Day: "08:00", Afternoon: "15:00", Evening: "18:00", Night: "21:30"}
	DefaultWeekend   = map[string]string{Morning: "07:30", Day: "10:00"} // others fall back to the school-day time
)

// Current is what /api/dashboard reports.
type Current struct {
	Name      string    `json:"name"`
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

// Build turns name→"HH:MM" maps into a sorted list. get returns the raw
// value for a scene name (already merged with defaults by the caller).
func Build(get func(name string) string) ([]Start, error) {
	var out []Start
	for _, n := range Names {
		v := get(n)
		if v == "" {
			continue
		}
		m, err := ParseClock(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		if m >= 0 {
			out = append(out, Start{Name: n, Minute: m})
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
		cur.Name, cur.Since = today[idx].Name, at(now, today[idx].Minute, loc)
	} else {
		// … or, before the first start, whatever ran last on an earlier day
		// (normally last night's "night")
		for back := 1; back <= 7 && cur.Name == ""; back++ {
			prev := now.AddDate(0, 0, -back)
			if list := s.startsFor(prev); len(list) > 0 {
				last := list[len(list)-1]
				cur.Name, cur.Since = last.Name, at(prev, last.Minute, loc)
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

	if cur.Name == "" {
		cur.Name = Day
	}
	if s.Force != "" {
		cur.Name, cur.Forced = s.Force, true
	}
	return cur
}

// Valid reports whether name is a known scene.
func Valid(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}
