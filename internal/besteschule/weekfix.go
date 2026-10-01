package besteschule

import (
	"fmt"
	"strings"
	"time"
)

// WeekFix corrects the A/B week of lessons that are mislabelled in
// beste.schule, e.g. two alternating lessons in the same slot that are both
// marked "B". Set via BESTESCHULE_WEEK_FIX=Fr:PH=A (comma-separated).
type WeekFix struct {
	Weekday time.Weekday
	Subject string // matched case-insensitively against subject name/short name or group
	Week    string // "A", "B", …
}

// ParseWeekFixes reads "Fr:PH=A, Mo:KU=B".
func ParseWeekFixes(s string) ([]WeekFix, error) {
	var out []WeekFix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		day, rest, ok1 := strings.Cut(part, ":")
		subj, week, ok2 := strings.Cut(rest, "=")
		day, subj, week = strings.TrimSpace(day), strings.TrimSpace(subj), strings.TrimSpace(week)
		if !ok1 || !ok2 || subj == "" || week == "" {
			return nil, fmt.Errorf("%q: erwartet Tag:Fach=Woche, z.B. Fr:PH=A", part)
		}
		wd, ok := parseWeekday(day)
		if !ok {
			return nil, fmt.Errorf("%q: unbekannter Wochentag %q", part, day)
		}
		out = append(out, WeekFix{Weekday: wd, Subject: subj, Week: strings.ToUpper(week)})
	}
	return out, nil
}

// apply replaces the lesson's week list when a fix matches it.
func applyWeekFixes(l obj, fixes []WeekFix) obj {
	if len(fixes) == 0 || len(asList(l["weeks"])) == 0 {
		return l
	}
	wdStr, ok := direct(l, weekdayKeys)
	if !ok {
		return l
	}
	wd, ok := parseWeekday(wdStr)
	if !ok {
		return l
	}
	names := lessonNames(l)
	for _, f := range fixes {
		if f.Weekday == wd && names[strings.ToLower(f.Subject)] {
			c := make(obj, len(l))
			for k, v := range l {
				c[k] = v
			}
			c["weeks"] = []any{f.Week}
			return c
		}
	}
	return l
}

// lessonNames collects the subject's name/short name and the lesson's groups.
func lessonNames(l obj) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out[s] = true
		}
	}
	for _, k := range subjectKeys {
		for _, v := range append([]any{l[k]}, asList(l[k])...) {
			switch x := v.(type) {
			case string:
				add(x)
			case obj:
				for _, kk := range []string{"name", "local_id", "short_name", "shortName", "abbreviation"} {
					if s, ok := scalar(x[kk]); ok {
						add(s)
					}
				}
			}
		}
	}
	for g := range itemGroups(l) {
		add(g)
	}
	return out
}
