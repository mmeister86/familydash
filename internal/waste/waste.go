// Package waste computes bin collection dates from simple weekly rules like
// "Restabfall: mittwochs gerade Kalenderwoche" – for districts that only
// publish a printable plan instead of an iCal feed.
package waste

import (
	"fmt"
	"strings"
	"time"
)

type Parity int

const (
	Every Parity = iota
	Even
	Odd
)

type Bin struct {
	Name    string
	Color   string
	Weekday time.Weekday
	Weeks   Parity // ISO calendar week number parity
}

// Pickup is what /api/dashboard returns per bin.
type Pickup struct {
	Name    string   `json:"name"`
	Color   string   `json:"color"`
	Dates   []string `json:"dates"`             // next collections, YYYY-MM-DD
	Shifted []string `json:"shifted,omitempty"` // those moved by a public holiday
}

type Service struct {
	Bins  []Bin
	Shift bool // move collections by one day per public holiday earlier in the same week
}

const count = 2 // collections listed per bin

// Snapshot lists the next collections from today (today included all day).
func (s *Service) Snapshot(now time.Time, loc *time.Location) []Pickup {
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	out := make([]Pickup, 0, len(s.Bins))
	for _, b := range s.Bins {
		p := Pickup{Name: b.Name, Color: b.Color, Dates: []string{}}
		// start a week back: a holiday may push last week's date to today
		for d := today.AddDate(0, 0, -7); len(p.Dates) < count && d.Before(today.AddDate(0, 0, 60)); d = d.AddDate(0, 0, 1) {
			if d.Weekday() != b.Weekday || !b.inWeek(d) {
				continue
			}
			got := d
			if s.Shift {
				got = shifted(d)
			}
			if got.Before(today) {
				continue
			}
			p.Dates = append(p.Dates, got.Format("2006-01-02"))
			if !got.Equal(d) {
				p.Shifted = append(p.Shifted, got.Format("2006-01-02"))
			}
		}
		out = append(out, p)
	}
	return out
}

func (b Bin) inWeek(d time.Time) bool {
	_, w := d.ISOWeek()
	switch b.Weeks {
	case Even:
		return w%2 == 0
	case Odd:
		return w%2 == 1
	}
	return true
}

// shifted applies the usual German rule: every weekday holiday on or before
// the collection day in the same week moves it back by one day.
func shifted(d time.Time) time.Time {
	monday := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	n := 0
	for h := monday; !h.After(d); h = h.AddDate(0, 0, 1) {
		if h.Weekday() != time.Saturday && h.Weekday() != time.Sunday && IsHoliday(h) {
			n++
		}
	}
	return d.AddDate(0, 0, n)
}

// IsHoliday reports public holidays in Saxony.
func IsHoliday(d time.Time) bool {
	y, m, day := d.Date()
	switch {
	case m == time.January && day == 1,
		m == time.May && day == 1,
		m == time.October && (day == 3 || day == 31),
		m == time.December && (day == 25 || day == 26):
		return true
	}
	date := time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	e := easter(y)
	for _, off := range []int{-2, 1, 39, 50} { // Karfreitag, Ostermontag, Himmelfahrt, Pfingstmontag
		if date.Equal(e.AddDate(0, 0, off)) {
			return true
		}
	}
	// Buß- und Bettag: the Wednesday before 23 November
	bb := time.Date(y, time.November, 22, 12, 0, 0, 0, time.UTC)
	for bb.Weekday() != time.Wednesday {
		bb = bb.AddDate(0, 0, -1)
	}
	return date.Equal(bb)
}

// easter returns Easter Sunday (anonymous Gregorian algorithm), noon UTC.
func easter(y int) time.Time {
	a := y % 19
	b, c := y/100, y%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(y, time.Month(month), day, 12, 0, 0, 0, time.UTC)
}

var weekdays = map[string]time.Weekday{
	"mo": time.Monday, "di": time.Tuesday, "mi": time.Wednesday, "do": time.Thursday,
	"fr": time.Friday, "sa": time.Saturday, "so": time.Sunday,
	"tu": time.Tuesday, "we": time.Wednesday, "th": time.Thursday, "su": time.Sunday,
}

// ParseWeekday accepts "Mi", "mittwoch", "mittwochs", "Wed", "wednesday".
func ParseWeekday(s string) (time.Weekday, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) >= 2 {
		if wd, ok := weekdays[s[:2]]; ok {
			return wd, nil
		}
	}
	return 0, fmt.Errorf("unbekannter Wochentag %q", s)
}

// ParseParity accepts "gerade"/"even", "ungerade"/"odd", "" / "jede" / "all".
func ParseParity(s string) (Parity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "jede", "alle", "all", "every", "weekly", "wöchentlich":
		return Every, nil
	case "gerade", "even":
		return Even, nil
	case "ungerade", "odd":
		return Odd, nil
	}
	return 0, fmt.Errorf("unbekannte Kalenderwoche %q (gerade/ungerade/jede)", s)
}
