package besteschule

// The beste.schule API returns deeply nested JSON whose exact shape varies
// with the "include" parameter and has changed over time. Like the Home
// Assistant integration (github.com/RF1705/beste-schule) we therefore read it
// through tolerant helpers that look for several possible key names instead of
// fixed structs.

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type obj = map[string]any

var (
	dateKeys    = []string{"date", "day_date", "lesson_date", "starts_on", "start_date", "given_at"}
	weekdayKeys = []string{"weekday", "weekDay", "week_day", "dayOfWeek", "day_of_week", "day", "weekday_id"}
	startKeys   = []string{"start", "starts_at", "start_at", "startTime", "start_time", "time_start", "from", "begin", "begins_at"}
	endKeys     = []string{"end", "ends_at", "end_at", "endTime", "end_time", "time_end", "to", "until"}
	nrKeys      = []string{"nr", "number", "lessonNr", "lesson_nr", "lessonNumber"}
	subjectKeys = []string{"subject", "subjects", "subjectName", "subject_name", "course"}
	roomKeys    = []string{"room", "rooms", "roomName", "room_name"}
	teacherKeys = []string{"teacher", "teachers", "teacherName", "teacher_name"}
)

var weekdayNames = map[string]time.Weekday{
	"monday": time.Monday, "montag": time.Monday, "mo": time.Monday,
	"tuesday": time.Tuesday, "dienstag": time.Tuesday, "di": time.Tuesday,
	"wednesday": time.Wednesday, "mittwoch": time.Wednesday, "mi": time.Wednesday,
	"thursday": time.Thursday, "donnerstag": time.Thursday, "do": time.Thursday,
	"friday": time.Friday, "freitag": time.Friday, "fr": time.Friday,
	"saturday": time.Saturday, "samstag": time.Saturday, "sa": time.Saturday,
	"sunday": time.Sunday, "sonntag": time.Sunday, "so": time.Sunday,
}

// payload unwraps the Laravel-style {"data": …} envelope.
func payload(v any) any {
	if m, ok := v.(obj); ok {
		if d, ok := m["data"]; ok {
			return d
		}
	}
	return v
}

func asList(v any) []any {
	l, _ := payload(v).([]any)
	return l
}

func scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(x), true
	}
	return "", false
}

func toInt(v any) (int, bool) {
	s, ok := scalar(v)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}

// direct returns the first scalar found under keys at this level.
func direct(m obj, keys []string) (string, bool) {
	for _, k := range keys {
		if s, ok := scalar(m[k]); ok {
			return s, true
		}
	}
	return "", false
}

// find returns the first scalar under keys at this level or in nested objects.
func find(m obj, keys []string) (string, bool) {
	if s, ok := direct(m, keys); ok {
		return s, true
	}
	for _, v := range m {
		if n, ok := v.(obj); ok {
			if s, ok := find(n, keys); ok {
				return s, true
			}
		}
	}
	return "", false
}

// text extracts a human-readable label from a string, relation object or list.
func text(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []any:
		var parts []string
		for _, it := range x {
			if t := text(it); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ", ")
	case obj:
		for _, k := range []string{"name", "display_name", "displayName", "full_name", "fullName", "title", "label", "description", "text", "shortName", "short_name", "abbreviation", "local_id"} {
			if t := text(x[k]); t != "" {
				return t
			}
		}
		first := text(firstOf(x, "forename", "firstName", "first_name", "firstname"))
		last := text(firstOf(x, "lastName", "last_name", "lastname"))
		return strings.TrimSpace(first + " " + last)
	}
	return ""
}

func firstOf(m obj, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// roomText prefers the school's local room number ("R12") over a long name.
func roomText(v any) string {
	switch x := v.(type) {
	case []any:
		var parts []string
		for _, it := range x {
			if t := roomText(it); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ", ")
	case obj:
		for _, k := range []string{"local_id", "name", "title", "label"} {
			if t := text(x[k]); t != "" {
				return t
			}
		}
	}
	return text(v)
}

// teacherText returns "Forename Name" when both are present.
func teacherText(v any) string {
	switch x := v.(type) {
	case []any:
		var parts []string
		for _, it := range x {
			if t := teacherText(it); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ", ")
	case obj:
		fn, n := text(x["forename"]), text(x["name"])
		if fn != "" || n != "" {
			return strings.TrimSpace(fn + " " + n)
		}
	}
	return text(v)
}

// directText formats the first non-empty relation under keys at this level.
func directText(m obj, keys []string, format func(any) string) string {
	for _, k := range keys {
		if t := format(m[k]); t != "" {
			return t
		}
	}
	return ""
}

// nestedText is directText, searching nested objects and lists too.
func nestedText(m obj, keys []string, format func(any) string) string {
	if t := directText(m, keys, format); t != "" {
		return t
	}
	for _, v := range m {
		switch x := v.(type) {
		case obj:
			if t := nestedText(x, keys, format); t != "" {
				return t
			}
		case []any:
			for _, it := range x {
				if n, ok := it.(obj); ok {
					if t := nestedText(n, keys, format); t != "" {
						return t
					}
				}
			}
		}
	}
	return ""
}

// replacement picks the substitute from a relation list: beste.schule lists
// the original and the replacement; the replacement is the one that differs
// from the original (or the last one).
func replacement(v any, format func(any) string, original string) string {
	l, ok := v.([]any)
	if !ok || len(l) < 2 {
		return format(v)
	}
	for _, it := range l {
		if t := format(it); t != "" && !strings.EqualFold(t, original) {
			return t
		}
	}
	return format(l[len(l)-1])
}

// allText flattens any value into searchable lower-case text.
func allText(v any) string {
	switch x := v.(type) {
	case string:
		return strings.ToLower(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case obj:
		var b strings.Builder
		for _, it := range x {
			b.WriteString(allText(it))
			b.WriteByte(' ')
		}
		return b.String()
	case []any:
		var b strings.Builder
		for _, it := range x {
			b.WriteString(allText(it))
			b.WriteByte(' ')
		}
		return b.String()
	}
	return ""
}

var (
	reDate = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	reTime = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
)

// parseDate returns YYYY-MM-DD or "".
func parseDate(s string) string {
	if m := reDate.FindString(s); m != "" {
		return m
	}
	return ""
}

// parseClock returns HH:MM or "".
func parseClock(s string) string {
	m := reTime.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	if h > 23 || mi > 59 {
		return ""
	}
	return strconv.Itoa(100 + h)[1:] + ":" + m[2]
}

func parseWeekday(s string) (time.Weekday, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if wd, ok := weekdayNames[s]; ok {
		return wd, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	switch {
	case n >= 1 && n <= 7: // ISO: 1 = Monday … 7 = Sunday
		return time.Weekday(n % 7), true
	case n == 0:
		return time.Monday, true
	}
	return 0, false
}

// walk visits every object below v. Each visited object is merged with the
// date/weekday fields of its ancestors, so a lesson inside a day inherits the
// day's date.
func walk(v any, fn func(obj)) { walkCtx(v, obj{}, fn) }

func walkCtx(v any, ctx obj, fn func(obj)) {
	switch x := v.(type) {
	case obj:
		merged := make(obj, len(ctx)+len(x))
		for k, val := range ctx {
			merged[k] = val
		}
		for k, val := range x {
			merged[k] = val
		}
		fn(merged)
		next := ctx
		for _, k := range append(append([]string{}, dateKeys...), weekdayKeys...) {
			if val, ok := x[k]; ok {
				if _, isScalar := scalar(val); isScalar {
					if _, have := next[k]; !have {
						if len(next) == len(ctx) {
							next = cloneObj(ctx)
						}
						next[k] = val
					}
				}
			}
		}
		for _, val := range x {
			walkCtx(val, next, fn)
		}
	case []any:
		for _, it := range x {
			walkCtx(it, ctx, fn)
		}
	}
}

func cloneObj(m obj) obj {
	out := make(obj, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// dateOf finds the date an item (lesson, note, day) belongs to.
func dateOf(m obj) string {
	if s, ok := find(m, dateKeys); ok {
		if d := parseDate(s); d != "" {
			return d
		}
	}
	for _, k := range []string{"day", "lesson", "notable"} {
		if n, ok := m[k].(obj); ok {
			if d := dateOf(n); d != "" {
				return d
			}
		}
	}
	return ""
}
