// Package calendar fetches iCalendar (.ics) feeds – e.g. the "secret address in
// iCal format" that Google Calendar offers per calendar – and expands them into
// concrete events for a time window. Standard library only, on purpose.
package calendar

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Event is one concrete occurrence, ready for the frontend.
type Event struct {
	Cal       int       `json:"cal"` // index into Snapshot.Calendars
	Calendar  string    `json:"calendar"`
	Color     string    `json:"color"`
	Title     string    `json:"title"`
	Location  string    `json:"location,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	AllDay    bool      `json:"allDay"`
	StartDate string    `json:"startDate,omitempty"` // YYYY-MM-DD, all-day only
	EndDate   string    `json:"endDate,omitempty"`   // exclusive, all-day only

	// Central (Convex) mode additions: stable backend identity of the
	// occurrence and its calendar. Empty in local mode.
	CalendarID      string `json:"calendarId"`
	Key             string `json:"key"`
	UID             string `json:"uid"`
	IdentityQuality string `json:"identityQuality"`
}

// ---------------------------------------------------------------- raw parsing

type prop struct {
	name   string
	params map[string]string
	value  string
}

type component struct {
	name     string
	props    []prop
	children []*component
}

func (c *component) get(name string) (prop, bool) {
	for _, p := range c.props {
		if p.name == name {
			return p, true
		}
	}
	return prop{}, false
}

func (c *component) all(name string) []prop {
	var out []prop
	for _, p := range c.props {
		if p.name == name {
			out = append(out, p)
		}
	}
	return out
}

// unfold joins RFC 5545 folded lines (continuation lines start with space/tab).
func unfold(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var lines []string
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func parseProp(line string) (prop, bool) {
	inQuote := false
	colon := -1
	for i, r := range line {
		if r == '"' {
			inQuote = !inQuote
		} else if r == ':' && !inQuote {
			colon = i
			break
		}
	}
	if colon < 0 {
		return prop{}, false
	}
	head, value := line[:colon], line[colon+1:]
	parts := splitOutsideQuotes(head, ';')
	p := prop{name: strings.ToUpper(parts[0]), params: map[string]string{}, value: value}
	for _, kv := range parts[1:] {
		if k, v, ok := strings.Cut(kv, "="); ok {
			p.params[strings.ToUpper(k)] = strings.Trim(v, `"`)
		}
	}
	return p, true
}

func splitOutsideQuotes(s string, sep rune) []string {
	var out []string
	inQuote, start := false, 0
	for i, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == sep && !inQuote:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func parseComponents(data []byte) (*component, error) {
	root := &component{name: "ROOT"}
	stack := []*component{root}
	for _, line := range unfold(data) {
		p, ok := parseProp(line)
		if !ok {
			continue // tolerate junk lines
		}
		switch p.name {
		case "BEGIN":
			c := &component{name: strings.ToUpper(p.value)}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, c)
			stack = append(stack, c)
		case "END":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		default:
			cur := stack[len(stack)-1]
			cur.props = append(cur.props, p)
		}
	}
	if len(root.children) == 0 {
		return nil, fmt.Errorf("no iCalendar data found")
	}
	return root, nil
}

func unescapeText(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}

// ---------------------------------------------------------------- date/time

var (
	locCache   = map[string]*time.Location{}
	locCacheMu sync.Mutex
)

// windowsZones maps the few Outlook/Windows TZIDs that show up in shared
// calendars to IANA names. Google itself always uses IANA names.
var windowsZones = map[string]string{
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Romance Standard Time":          "Europe/Paris",
	"GMT Standard Time":              "Europe/London",
	"UTC":                            "UTC",
	"Coordinated Universal Time":     "UTC",
	"Eastern Standard Time":          "America/New_York",
	"Pacific Standard Time":          "America/Los_Angeles",
	"Central European Standard Time": "Europe/Warsaw",
}

func loadLoc(tzid string, def *time.Location) *time.Location {
	if tzid == "" {
		return def
	}
	tzid = strings.TrimPrefix(tzid, "/") // some exporters prefix with "/"
	locCacheMu.Lock()
	defer locCacheMu.Unlock()
	if l, ok := locCache[tzid]; ok {
		return l
	}
	name := tzid
	if m, ok := windowsZones[tzid]; ok {
		name = m
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		l = def
	}
	locCache[tzid] = l
	return l
}

// parseTimeValue parses a DATE or DATE-TIME value. allDay reports a DATE value.
func parseTimeValue(val string, params map[string]string, def *time.Location) (t time.Time, allDay bool, err error) {
	val = strings.TrimSpace(val)
	if params["VALUE"] == "DATE" || len(val) == 8 {
		t, err = time.ParseInLocation("20060102", val, def)
		return t, true, err
	}
	if strings.HasSuffix(val, "Z") {
		t, err = time.Parse("20060102T150405Z", val)
		return t, false, err
	}
	t, err = time.ParseInLocation("20060102T150405", val, loadLoc(params["TZID"], def))
	return t, false, err
}

// parseDuration handles RFC 5545 durations like PT1H30M, P1D, P2W, -PT15M.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	sign := time.Duration(1)
	if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	}
	s = strings.TrimPrefix(s, "+")
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	s = s[1:]
	var d time.Duration
	inTime := false
	num := 0
	for _, r := range s {
		switch {
		case r == 'T':
			inTime = true
		case r >= '0' && r <= '9':
			num = num*10 + int(r-'0')
		default:
			unit := map[rune]time.Duration{'W': 7 * 24 * time.Hour, 'D': 24 * time.Hour, 'H': time.Hour, 'S': time.Second}[r]
			if r == 'M' {
				unit = time.Minute
				if !inTime {
					return 0, fmt.Errorf("months not allowed in duration")
				}
			}
			if unit == 0 {
				return 0, fmt.Errorf("bad duration unit %q", r)
			}
			d += time.Duration(num) * unit
			num = 0
		}
	}
	return sign * d, nil
}

// ---------------------------------------------------------------- events

type rawEvent struct {
	uid       string
	summary   string
	location  string
	start     time.Time
	end       time.Time
	allDay    bool
	rrule     string
	exdates   []time.Time
	recurID   time.Time
	hasRecID  bool
	cancelled bool
}

func (e rawEvent) duration() time.Duration { return e.end.Sub(e.start) }

func toRawEvent(c *component, loc *time.Location) (rawEvent, error) {
	var ev rawEvent
	if p, ok := c.get("UID"); ok {
		ev.uid = p.value
	}
	if p, ok := c.get("SUMMARY"); ok {
		ev.summary = unescapeText(p.value)
	}
	if p, ok := c.get("LOCATION"); ok {
		ev.location = unescapeText(p.value)
	}
	if p, ok := c.get("STATUS"); ok && strings.EqualFold(p.value, "CANCELLED") {
		ev.cancelled = true
	}
	p, ok := c.get("DTSTART")
	if !ok {
		return ev, fmt.Errorf("event %q without DTSTART", ev.uid)
	}
	var err error
	if ev.start, ev.allDay, err = parseTimeValue(p.value, p.params, loc); err != nil {
		return ev, err
	}
	switch {
	case hasProp(c, "DTEND"):
		p, _ := c.get("DTEND")
		if ev.end, _, err = parseTimeValue(p.value, p.params, loc); err != nil {
			return ev, err
		}
	case hasProp(c, "DURATION"):
		p, _ := c.get("DURATION")
		d, err := parseDuration(p.value)
		if err != nil {
			return ev, err
		}
		ev.end = ev.start.Add(d)
	case ev.allDay:
		ev.end = ev.start.AddDate(0, 0, 1)
	default:
		ev.end = ev.start
	}
	if !ev.end.After(ev.start) && ev.allDay {
		ev.end = ev.start.AddDate(0, 0, 1)
	}
	if p, ok := c.get("RRULE"); ok {
		ev.rrule = p.value
	}
	for _, p := range c.all("EXDATE") {
		for _, v := range strings.Split(p.value, ",") {
			if t, _, err := parseTimeValue(v, p.params, ev.start.Location()); err == nil {
				ev.exdates = append(ev.exdates, t)
			}
		}
	}
	if p, ok := c.get("RECURRENCE-ID"); ok {
		if ev.recurID, _, err = parseTimeValue(p.value, p.params, ev.start.Location()); err == nil {
			ev.hasRecID = true
		}
	}
	return ev, nil
}

func hasProp(c *component, name string) bool { _, ok := c.get(name); return ok }

// occurrenceKey identifies an occurrence for EXDATE / RECURRENCE-ID matching.
func occurrenceKey(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("20060102")
	}
	return fmt.Sprint(t.Unix())
}

// Expand parses an .ics document and returns all event occurrences that
// overlap [from, to), converted to loc and sorted by start.
func Expand(data []byte, calName, color string, from, to time.Time, loc *time.Location) ([]Event, error) {
	root, err := parseComponents(data)
	if err != nil {
		return nil, err
	}
	var vevents []*component
	var walk func(*component)
	walk = func(c *component) {
		for _, ch := range c.children {
			if ch.name == "VEVENT" {
				vevents = append(vevents, ch)
			}
			walk(ch)
		}
	}
	walk(root)

	var masters []rawEvent
	overrides := map[string]map[string]bool{} // uid -> recurrence keys handled by an override
	var singles []rawEvent
	for _, c := range vevents {
		ev, err := toRawEvent(c, loc)
		if err != nil {
			continue // skip broken events instead of failing the whole feed
		}
		switch {
		case ev.hasRecID:
			if overrides[ev.uid] == nil {
				overrides[ev.uid] = map[string]bool{}
			}
			overrides[ev.uid][occurrenceKey(ev.recurID, ev.allDay)] = true
			if !ev.cancelled {
				singles = append(singles, ev)
			}
		case ev.cancelled:
			// ignore
		case ev.rrule != "":
			masters = append(masters, ev)
		default:
			singles = append(singles, ev)
		}
	}

	var out []Event
	emit := func(ev rawEvent, start time.Time) {
		end := start.Add(ev.duration())
		if ev.allDay {
			end = start.AddDate(0, 0, int(ev.end.Sub(ev.start).Hours()/24+0.5))
		}
		if !(start.Before(to) && end.After(from)) && !(start.Equal(end) && !start.Before(from) && start.Before(to)) {
			return
		}
		e := Event{Calendar: calName, Color: color, Title: ev.summary, Location: ev.location, AllDay: ev.allDay}
		if ev.allDay {
			e.StartDate = start.Format("2006-01-02")
			e.EndDate = end.Format("2006-01-02")
			e.Start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
			e.End = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
		} else {
			e.Start, e.End = start.In(loc), end.In(loc)
		}
		if e.Title == "" {
			e.Title = "(ohne Titel)"
		}
		out = append(out, e)
	}

	for _, ev := range singles {
		emit(ev, ev.start)
	}
	for _, ev := range masters {
		rule, err := parseRRule(ev.rrule, ev.start.Location())
		if err != nil {
			emit(ev, ev.start) // unknown rule: show at least the first occurrence
			continue
		}
		skip := map[string]bool{}
		for _, x := range ev.exdates {
			skip[occurrenceKey(x, ev.allDay)] = true
		}
		for k := range overrides[ev.uid] {
			skip[k] = true
		}
		// Widen the lookback by the event duration so long events that started
		// before the window are still found.
		for _, start := range rule.occurrences(ev.start, from.Add(-ev.duration()), to) {
			if !skip[occurrenceKey(start, ev.allDay)] {
				emit(ev, start)
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}
