package calendar

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// rrule covers the subset of RFC 5545 recurrence rules that real-world family
// calendars (Google, iCloud, Outlook) actually produce.
type rrule struct {
	freq       string
	interval   int
	count      int
	until      time.Time
	hasUntil   bool
	byDay      []weekdayNum
	byMonthDay []int
	byMonth    []time.Month
	bySetPos   []int
	wkst       time.Weekday
}

type weekdayNum struct {
	n  int // 0 = every such weekday, 1 = first, -1 = last, …
	wd time.Weekday
}

var weekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// maxPeriods bounds the expansion loop (e.g. a daily rule since 1990).
const maxPeriods = 50000

func parseRRule(s string, loc *time.Location) (*rrule, error) {
	r := &rrule{interval: 1, wkst: time.Monday}
	for _, part := range strings.Split(strings.TrimPrefix(s, "RRULE:"), ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.ToUpper(k) {
		case "FREQ":
			r.freq = strings.ToUpper(v)
		case "INTERVAL":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				r.interval = n
			}
		case "COUNT":
			r.count, _ = strconv.Atoi(v)
		case "UNTIL":
			t, allDay, err := parseTimeValue(v, map[string]string{}, loc)
			if err != nil {
				return nil, fmt.Errorf("UNTIL: %w", err)
			}
			if allDay {
				t = t.AddDate(0, 0, 1).Add(-time.Nanosecond) // inclusive whole day
			}
			r.until, r.hasUntil = t, true
		case "BYDAY":
			for _, d := range strings.Split(v, ",") {
				d = strings.TrimSpace(strings.ToUpper(d))
				if len(d) < 2 {
					continue
				}
				wd, ok := weekdays[d[len(d)-2:]]
				if !ok {
					return nil, fmt.Errorf("BYDAY %q", d)
				}
				n := 0
				if len(d) > 2 {
					n, _ = strconv.Atoi(d[:len(d)-2])
				}
				r.byDay = append(r.byDay, weekdayNum{n, wd})
			}
		case "BYMONTHDAY":
			r.byMonthDay = atoiList(v)
		case "BYMONTH":
			for _, m := range atoiList(v) {
				r.byMonth = append(r.byMonth, time.Month(m))
			}
		case "BYSETPOS":
			r.bySetPos = atoiList(v)
		case "WKST":
			if wd, ok := weekdays[strings.ToUpper(v)]; ok {
				r.wkst = wd
			}
		}
	}
	switch r.freq {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
	default:
		return nil, fmt.Errorf("unsupported FREQ %q", r.freq)
	}
	return r, nil
}

func atoiList(v string) []int {
	var out []int
	for _, s := range strings.Split(v, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// occurrences returns all occurrence starts in [from, to). COUNT is honoured
// from dtstart on, so expansion always begins at dtstart.
func (r *rrule) occurrences(dtstart, from, to time.Time) []time.Time {
	var out []time.Time
	emitted := 0
	for p := 0; p < maxPeriods; p++ {
		periodStart, cands := r.period(dtstart, p)
		if periodStart.After(to) {
			break
		}
		for _, c := range cands {
			if c.Before(dtstart) {
				continue
			}
			if r.hasUntil && c.After(r.until) {
				return out
			}
			emitted++
			if r.count > 0 && emitted > r.count {
				return out
			}
			if !c.Before(to) {
				return out
			}
			if !c.Before(from) {
				out = append(out, c)
			}
		}
	}
	return out
}

// period returns the start of the p-th period and its sorted candidates.
func (r *rrule) period(s time.Time, p int) (time.Time, []time.Time) {
	loc := s.Location()
	h, mi, sec := s.Clock()
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, h, mi, sec, 0, loc) }
	var cands []time.Time
	var start time.Time

	switch r.freq {
	case "DAILY":
		start = s.AddDate(0, 0, p*r.interval)
		if r.monthOK(start.Month()) && r.weekdayOK(start.Weekday()) {
			cands = []time.Time{start}
		}
	case "WEEKLY":
		offset := (int(s.Weekday()) - int(r.wkst) + 7) % 7
		weekStart := at(s.Year(), s.Month(), s.Day()-offset).AddDate(0, 0, 7*p*r.interval)
		start = weekStart
		days := []time.Weekday{s.Weekday()}
		if len(r.byDay) > 0 {
			days = days[:0]
			for _, bd := range r.byDay {
				days = append(days, bd.wd)
			}
		}
		for _, wd := range days {
			c := weekStart.AddDate(0, 0, (int(wd)-int(r.wkst)+7)%7)
			if r.monthOK(c.Month()) {
				cands = append(cands, c)
			}
		}
	case "MONTHLY":
		start = at(s.Year(), s.Month()+time.Month(p*r.interval), 1)
		if r.monthOK(start.Month()) {
			cands = r.monthCandidates(start.Year(), start.Month(), s, at)
		}
	case "YEARLY":
		y := s.Year() + p*r.interval
		start = at(y, 1, 1)
		months := r.byMonth
		if len(months) == 0 {
			months = []time.Month{s.Month()}
		}
		for _, m := range months {
			cands = append(cands, r.monthCandidates(y, m, s, at)...)
		}
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].Before(cands[j]) })
	cands = uniqueTimes(cands)
	if len(r.bySetPos) > 0 && len(cands) > 0 {
		var picked []time.Time
		for _, pos := range r.bySetPos {
			i := pos - 1
			if pos < 0 {
				i = len(cands) + pos
			}
			if i >= 0 && i < len(cands) {
				picked = append(picked, cands[i])
			}
		}
		sort.Slice(picked, func(i, j int) bool { return picked[i].Before(picked[j]) })
		cands = uniqueTimes(picked)
	}
	return start, cands
}

func (r *rrule) monthCandidates(y int, m time.Month, s time.Time, at func(int, time.Month, int) time.Time) []time.Time {
	dim := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var out []time.Time
	switch {
	case len(r.byMonthDay) > 0:
		for _, d := range r.byMonthDay {
			if d < 0 {
				d = dim + d + 1
			}
			if d >= 1 && d <= dim {
				c := at(y, m, d)
				if r.weekdayOK(c.Weekday()) {
					out = append(out, c)
				}
			}
		}
	case len(r.byDay) > 0:
		first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).Weekday()
		for _, bd := range r.byDay {
			firstDay := 1 + (int(bd.wd)-int(first)+7)%7 // first such weekday in month
			switch {
			case bd.n == 0:
				for d := firstDay; d <= dim; d += 7 {
					out = append(out, at(y, m, d))
				}
			case bd.n > 0:
				if d := firstDay + (bd.n-1)*7; d <= dim {
					out = append(out, at(y, m, d))
				}
			default:
				last := firstDay
				for last+7 <= dim {
					last += 7
				}
				if d := last + (bd.n+1)*7; d >= 1 {
					out = append(out, at(y, m, d))
				}
			}
		}
	default:
		if s.Day() <= dim { // e.g. "every 31st" skips short months, per RFC
			out = append(out, at(y, m, s.Day()))
		}
	}
	return out
}

func (r *rrule) monthOK(m time.Month) bool {
	if len(r.byMonth) == 0 {
		return true
	}
	for _, bm := range r.byMonth {
		if bm == m {
			return true
		}
	}
	return false
}

// weekdayOK applies BYDAY as a plain weekday filter (ordinals ignored).
func (r *rrule) weekdayOK(wd time.Weekday) bool {
	if len(r.byDay) == 0 {
		return true
	}
	for _, bd := range r.byDay {
		if bd.wd == wd {
			return true
		}
	}
	return false
}

func uniqueTimes(ts []time.Time) []time.Time {
	if len(ts) < 2 {
		return ts
	}
	out := ts[:1]
	for _, t := range ts[1:] {
		if !t.Equal(out[len(out)-1]) {
			out = append(out, t)
		}
	}
	return out
}
