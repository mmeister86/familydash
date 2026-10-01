// Package besteschule reads timetable, substitutions, homework and upcoming
// exams from beste.schule (https://beste.schule) with a personal access token
// (beste.schule → Benutzerkonto → API).
package besteschule

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- output

type School struct {
	Students  []Student `json:"students"`
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

type Student struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Day      Day     `json:"day"`
	Homework []Entry `json:"homework"`
	Exams    []Entry `json:"exams"`
}

// Day is the timetable shown on the wall: today while school is running,
// afterwards the next school day.
type Day struct {
	Date     string   `json:"date"` // YYYY-MM-DD
	Lessons  []Lesson `json:"lessons"`
	Notices  []string `json:"notices,omitempty"`
	NoSchool bool     `json:"noSchool,omitempty"` // nothing within the next week
}

type Lesson struct {
	Nr      int    `json:"nr"`
	Start   string `json:"start,omitempty"` // HH:MM
	End     string `json:"end,omitempty"`
	Subject string `json:"subject"`
	Room    string `json:"room,omitempty"`
	Teacher string `json:"teacher,omitempty"`
	Status  string `json:"status,omitempty"` // "", "cancelled", "substitution"
	Info    string `json:"info,omitempty"`
	Tag     string `json:"tag,omitempty"` // small label, e.g. "GTA" for an afternoon club
}

type Entry struct {
	Date    string `json:"date"`
	Subject string `json:"subject,omitempty"`
	Kind    string `json:"kind,omitempty"` // e.g. "Klassenarbeit", "Hausaufgabe"
	Text    string `json:"text,omitempty"`
	Nr      int    `json:"nr,omitempty"`
}

// ---------------------------------------------------------------- raw input

// Raw holds the unparsed API responses. It is what -besteschule-dump prints.
type Raw struct {
	Students      any            `json:"students"`
	Groups        map[string]any `json:"groups,omitempty"` // student id → groups
	Timetable     any            `json:"timetable"`
	Substitutions any            `json:"substitutions"`
	Journal       map[string]any `json:"journal"` // student id → journal/lessons
}

const (
	homeworkDays = 14
	examDays     = 21
	lookahead    = 8 // days to search for the next school day
)

// Build turns raw responses into what the dashboard shows at time now.
// only optionally restricts students by id or first name; fixes correct
// mislabelled A/B weeks.
func Build(raw *Raw, now time.Time, loc *time.Location, only []string, fixes ...WeekFix) School {
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	students := studentList(raw.Students)
	multi := len(students) > 1
	if len(students) == 0 {
		students = []obj{{}}
	}

	tt := timetableOf(raw.Timetable)
	for i, l := range tt.lessons {
		tt.lessons[i] = applyWeekFixes(l, fixes)
	}
	var out []Student
	for _, st := range students {
		s := Student{ID: idOf(st), Name: studentName(st), Homework: []Entry{}, Exams: []Entry{}}
		if !selected(s, only) {
			continue
		}
		var ctx *studentCtx
		if multi {
			ctx = newStudentCtx(st, raw.Groups[s.ID])
		}
		s.Day = pickDay(tt, raw.Substitutions, ctx, now, today)
		s.Homework, s.Exams = journalEntries(raw.Journal[s.ID], today)
		out = append(out, s)
	}
	return School{Students: out}
}

func selected(s Student, only []string) bool {
	if len(only) == 0 {
		return true
	}
	for _, o := range only {
		if strings.EqualFold(o, s.ID) || strings.EqualFold(o, s.Name) {
			return true
		}
	}
	return false
}

func studentList(v any) []obj {
	var out []obj
	switch x := payload(v).(type) {
	case []any:
		for _, it := range x {
			if m, ok := it.(obj); ok && m["id"] != nil {
				out = append(out, m)
			}
		}
	case obj:
		if x["id"] != nil {
			out = append(out, x)
		}
	}
	return out
}

func idOf(m obj) string {
	s, _ := scalar(m["id"])
	return s
}

func studentName(m obj) string {
	if t := text(firstOf(m, "forename", "first_name", "firstName", "firstname")); t != "" {
		return t
	}
	return text(m)
}

// ---------------------------------------------------------------- timetable

type timetable struct {
	lessons     []obj
	periods     map[int][2]string
	validFrom   string
	validTo     string
	noSchool    map[string]bool
	weekTypes   map[string][]string // "2026-W35" → ["A"]
	hasWeekInfo bool
}

func timetableOf(v any) *timetable {
	t := &timetable{periods: map[int][2]string{}, noSchool: map[string]bool{}, weekTypes: map[string][]string{}}
	m, _ := payload(v).(obj)
	if m == nil {
		return t
	}
	for _, it := range asList(m["lessons"]) {
		l, ok := it.(obj)
		if !ok {
			continue
		}
		t.lessons = append(t.lessons, l)
		nr, okNr := toInt(firstOf(l, nrKeys...))
		start, end := lessonClock(l)
		if okNr && start != "" && end != "" {
			if _, have := t.periods[nr]; !have {
				t.periods[nr] = [2]string{start, end}
			}
		}
	}
	vf, _ := scalar(m["valid_from"])
	vt, _ := scalar(m["valid_to"])
	t.validFrom, t.validTo = parseDate(vf), parseDate(vt)
	for _, d := range asList(m["no_school_dates"]) {
		if s, ok := scalar(d); ok {
			t.noSchool[parseDate(s)] = true
		}
	}
	for _, w := range asList(m["weeks"]) {
		wm, ok := w.(obj)
		if !ok {
			continue
		}
		nr, ok1 := toInt(wm["nr"])
		yr, ok2 := toInt(wm["year"])
		if !ok1 || !ok2 {
			continue
		}
		var types []string
		for _, ty := range asList(wm["types"]) {
			if s, ok := scalar(ty); ok && strings.TrimSpace(s) != "" {
				types = append(types, strings.TrimSpace(s))
			}
		}
		t.weekTypes[weekKey(yr, nr)] = types
		t.hasWeekInfo = true
	}
	return t
}

func weekKey(year, week int) string { return strconv.Itoa(year) + "-W" + strconv.Itoa(week) }

func lessonClock(l obj) (string, string) {
	var start, end string
	if s, ok := find(l, startKeys); ok {
		start = parseClock(s)
	}
	if s, ok := find(l, endKeys); ok {
		end = parseClock(s)
	}
	return start, end
}

func (t *timetable) isSchoolDay(date string) bool {
	if t.validFrom != "" && date < t.validFrom {
		return false
	}
	if t.validTo != "" && date > t.validTo {
		return false
	}
	return !t.noSchool[date]
}

func (t *timetable) weekAllows(l obj, day time.Time) bool {
	lw := asList(l["weeks"])
	if len(lw) == 0 || !t.hasWeekInfo {
		return true
	}
	y, w := day.ISOWeek()
	types, ok := t.weekTypes[weekKey(y, w)]
	if !ok {
		return true
	}
	for _, x := range lw {
		s, _ := scalar(x)
		for _, ty := range types {
			if strings.TrimSpace(s) == ty {
				return true
			}
		}
	}
	return false
}

func pickDay(tt *timetable, subs any, ctx *studentCtx, now, today time.Time) Day {
	for i := 0; i < lookahead; i++ {
		d := today.AddDate(0, 0, i)
		day := buildDay(tt, subs, ctx, d)
		if len(day.Lessons) == 0 {
			continue
		}
		if i == 0 && !now.Before(dayEnd(day, d)) {
			continue // school's out – show the next school day
		}
		return day
	}
	day := buildDay(tt, subs, ctx, today)
	day.NoSchool = true
	return day
}

// dayEnd is when the last lesson ends (fallback 16:00).
func dayEnd(day Day, d time.Time) time.Time {
	last := ""
	for _, l := range day.Lessons {
		if l.End > last {
			last = l.End
		}
	}
	h, m := 16, 0
	if last != "" {
		h, _ = strconv.Atoi(last[:2])
		m, _ = strconv.Atoi(last[3:])
	}
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, d.Location())
}

func buildDay(tt *timetable, subs any, ctx *studentCtx, d time.Time) Day {
	date := d.Format("2006-01-02")
	day := Day{Date: date, Lessons: []Lesson{}}
	subDay := substitutionDay(subs, date)
	if subDay != nil {
		day.Notices = dayNotices(subDay)
	}
	if !tt.isSchoolDay(date) {
		return day
	}

	for _, l := range tt.lessons {
		if ld := parseDate(firstScalar(l, dateKeys)); ld != "" {
			if ld != date {
				continue
			}
		} else {
			wdStr, ok := direct(l, weekdayKeys)
			if !ok {
				continue
			}
			wd, ok := parseWeekday(wdStr)
			if !ok || wd != d.Weekday() || !tt.weekAllows(l, d) {
				continue
			}
		}
		if !ctx.matches(l) {
			continue
		}
		les := lessonFrom(l, tt)
		if les.Subject != "" {
			day.Lessons = append(day.Lessons, les)
		}
	}

	if subDay != nil {
		applySubstitutions(&day, subDay, tt, ctx)
	}

	sort.SliceStable(day.Lessons, func(i, j int) bool {
		if day.Lessons[i].Nr != day.Lessons[j].Nr {
			return day.Lessons[i].Nr < day.Lessons[j].Nr
		}
		return day.Lessons[i].Start < day.Lessons[j].Start
	})
	day.Lessons = dedupeLessons(day.Lessons)
	return day
}

func firstScalar(m obj, keys []string) string {
	s, _ := direct(m, keys)
	return s
}

func lessonFrom(l obj, tt *timetable) Lesson {
	nr, _ := toInt(firstOf(l, nrKeys...))
	start, end := lessonClock(l)
	if (start == "" || end == "") && nr > 0 {
		if p, ok := tt.periods[nr]; ok {
			start, end = p[0], p[1]
		}
	}
	return Lesson{
		Nr:      nr,
		Start:   start,
		End:     end,
		Subject: nestedText(l, subjectKeys, text),
		Room:    nestedText(l, roomKeys, roomText),
		Teacher: nestedText(l, teacherKeys, teacherText),
	}
}

func substitutionDay(subs any, date string) obj {
	for _, it := range asList(subs) {
		if m, ok := it.(obj); ok {
			if s, ok := scalar(m["date"]); ok && parseDate(s) == date {
				return m
			}
		}
	}
	return nil
}

func dayNotices(day obj) []string {
	var out []string
	for _, n := range asList(day["notes"]) {
		if t := noteText(n); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func noteText(n any) string {
	if m, ok := n.(obj); ok {
		for _, k := range []string{"description", "text", "content", "body", "note", "name", "title"} {
			if t := text(m[k]); t != "" {
				return t
			}
		}
		return ""
	}
	return text(n)
}

var (
	cancelledStatus = map[string]bool{"cancelled": true, "canceled": true, "ausfall": true, "free": true}
	substStatus     = map[string]bool{"planned": true, "substitution": true, "vertretung": true}
	regularStatus   = map[string]bool{"initial": true, "hold": true, "regular": true, "": true}
)

func applySubstitutions(day *Day, subDay obj, tt *timetable, ctx *studentCtx) {
	for _, it := range asList(subDay["lessons"]) {
		sl, ok := it.(obj)
		if !ok || !ctx.matches(sl) {
			continue
		}
		nr, ok := toInt(firstOf(sl, nrKeys...))
		if !ok {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(firstScalar(sl, []string{"status"})))
		notes := ""
		if n := sl["notes"]; n != nil {
			var parts []string
			for _, x := range asList(n) {
				if t := noteText(x); t != "" {
					parts = append(parts, t)
				}
			}
			if s, ok := n.(string); ok {
				parts = append(parts, strings.TrimSpace(s))
			}
			notes = strings.Join(parts, " · ")
		}
		lowNotes := strings.ToLower(notes)
		subject := directText(sl, subjectKeys, text)

		idx := -1
		for i, l := range day.Lessons {
			if l.Nr == nr {
				idx = i
				break
			}
		}

		switch {
		case cancelledStatus[status] || containsAny(lowNotes, "ausfall", "entfällt", "entfaellt", "cancel"):
			if idx >= 0 {
				day.Lessons[idx].Status = "cancelled"
				day.Lessons[idx].Info = notes
			} else if subject != "" {
				l := newSlot(nr, subject, tt)
				l.Status, l.Info = "cancelled", notes
				day.Lessons = append(day.Lessons, l)
			}

		case substStatus[status] || containsAny(lowNotes, "vertret", "ersatz", "verlegt", "raumänderung") || !regularStatus[status]:
			var orig Lesson
			if idx >= 0 {
				orig = day.Lessons[idx]
			} else {
				orig = newSlot(nr, "", tt)
			}
			l := orig
			if subject != "" {
				l.Subject = subject
			}
			if r := replacement(firstOf(sl, roomKeys...), roomText, orig.Room); r != "" {
				l.Room = r
			}
			if t := replacement(firstOf(sl, teacherKeys...), teacherText, orig.Teacher); t != "" {
				l.Teacher = t
			}
			var info []string
			if orig.Subject != "" && !strings.EqualFold(orig.Subject, l.Subject) {
				info = append(info, "statt "+orig.Subject)
			} else if orig.Room != "" && l.Room != orig.Room {
				info = append(info, "Raum "+l.Room)
			} else if orig.Teacher != "" && l.Teacher != orig.Teacher {
				info = append(info, "bei "+l.Teacher)
			}
			if notes != "" {
				info = append(info, notes)
			}
			l.Status, l.Info = "substitution", strings.Join(info, " · ")
			if l.Subject == "" {
				continue
			}
			if idx >= 0 {
				day.Lessons[idx] = l
			} else {
				day.Lessons = append(day.Lessons, l)
			}

		default: // regular entry in the plan – add it if the weekly plan lacks it
			if idx < 0 && subject != "" {
				l := newSlot(nr, subject, tt)
				l.Room = directText(sl, roomKeys, roomText)
				day.Lessons = append(day.Lessons, l)
			}
		}
	}
}

func newSlot(nr int, subject string, tt *timetable) Lesson {
	l := Lesson{Nr: nr, Subject: subject}
	if p, ok := tt.periods[nr]; ok {
		l.Start, l.End = p[0], p[1]
	}
	return l
}

// dedupeLessons drops exact duplicates (e.g. a lesson listed per group).
func dedupeLessons(ls []Lesson) []Lesson {
	seen := map[string]bool{}
	out := ls[:0]
	for _, l := range ls {
		k := strconv.Itoa(l.Nr) + "|" + l.Subject + "|" + l.Room + "|" + l.Status
		if !seen[k] {
			seen[k] = true
			out = append(out, l)
		}
	}
	return out
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- students & groups

type studentCtx struct {
	id     string
	groups map[string]bool
	class  [][2]string // ("7", "b") from "7b"
}

var (
	reClass  = regexp.MustCompile(`^(\d+)\s*([a-z])$`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]`)
)

func newStudentCtx(st obj, groupsResp any) *studentCtx {
	c := &studentCtx{id: idOf(st), groups: map[string]bool{}}
	add := func(v any) {
		for _, g := range asList(v) {
			if gm, ok := g.(obj); ok {
				for _, val := range groupValues(gm) {
					c.groups[val] = true
				}
			}
		}
	}
	add(st["meta_groups"])
	add(st["groups"])
	add(groupsResp)
	for g := range c.groups {
		if m := reClass.FindStringSubmatch(g); m != nil {
			c.class = append(c.class, [2]string{m[1], m[2]})
		}
	}
	return c
}

func groupValues(g obj) []string {
	var out []string
	for _, k := range []string{"id", "local_id", "name"} {
		if s, ok := scalar(g[k]); ok {
			if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// matches reports whether a lesson belongs to the student. Lessons without
// student/group information are kept.
func (c *studentCtx) matches(item obj) bool {
	if c == nil {
		return true
	}
	if sid := itemStudentID(item); sid != "" {
		return sid == c.id
	}
	ig := itemGroups(item)
	if len(ig) == 0 || len(c.groups) == 0 {
		return true
	}
	for g := range ig {
		if c.groups[g] {
			return true
		}
	}
	for g := range ig {
		compact := nonAlnum.ReplaceAllString(g, "")
		for _, cl := range c.class {
			if strings.HasPrefix(compact, cl[0]) && strings.HasSuffix(compact, cl[1]) {
				return true
			}
		}
	}
	return false
}

func itemStudentID(v any) string {
	switch x := v.(type) {
	case obj:
		if st, ok := x["student"].(obj); ok {
			if s, ok := scalar(st["id"]); ok {
				return s
			}
		}
		if s, ok := scalar(x["student_id"]); ok {
			return s
		}
		for k, val := range x {
			if k == "students" { // a group's member list is not the lesson's owner
				continue
			}
			if s := itemStudentID(val); s != "" {
				return s
			}
		}
	case []any:
		for _, it := range x {
			if s := itemStudentID(it); s != "" {
				return s
			}
		}
	}
	return ""
}

func itemGroups(v any) map[string]bool {
	out := map[string]bool{}
	var rec func(any)
	rec = func(v any) {
		switch x := v.(type) {
		case obj:
			if g, ok := x["group"].(obj); ok {
				for _, s := range groupValues(g) {
					out[s] = true
				}
			}
			for _, g := range asList(x["groups"]) {
				if gm, ok := g.(obj); ok {
					for _, s := range groupValues(gm) {
						out[s] = true
					}
				}
			}
			for _, val := range x {
				switch val.(type) {
				case obj, []any:
					rec(val)
				}
			}
		case []any:
			for _, it := range x {
				rec(it)
			}
		}
	}
	rec(v)
	return out
}

// ---------------------------------------------------------------- journal: homework & exams

var (
	examTypeMarkers = []string{"klassenarbeit", "leistungskontrolle", "kurzkontrolle", "klausur", "test", "arbeit", "exam", "lk"}
	examTextMarkers = []string{"klassenarbeit", "leistungskontrolle", "kurzkontrolle", "klausur", "vokabeltest", "kurztest"}
)

func journalEntries(journal any, today time.Time) (homework, exams []Entry) {
	homework, exams = []Entry{}, []Entry{}
	from := today.Format("2006-01-02")
	hwTo := today.AddDate(0, 0, homeworkDays).Format("2006-01-02")
	exTo := today.AddDate(0, 0, examDays).Format("2006-01-02")
	seen := map[string]bool{}

	walk(journal, func(item obj) {
		notes := asList(item["notes"])
		if len(notes) == 0 {
			return
		}
		date := dateOf(item)
		if date == "" || date < from || date > exTo {
			return
		}
		subject := subjectOf(item)
		nr, _ := toInt(firstOf(item, nrKeys...))
		for _, n := range notes {
			note, ok := n.(obj)
			if !ok {
				continue
			}
			kind := text(note["type"])
			desc := strings.TrimSpace(text(firstOf(note, "description", "text", "content", "body")))
			lk, ld := strings.ToLower(kind), strings.ToLower(desc)

			e := Entry{Date: date, Subject: subject, Kind: kind, Text: desc, Nr: nr}
			key := date + "|" + subject + "|" + lk + "|" + ld
			if seen[key] {
				continue
			}
			switch {
			case containsAny(lk, "hausauf", "homework") || (lk == "" && strings.Contains(ld, "hausaufgabe")):
				if date <= hwTo {
					seen[key] = true
					homework = append(homework, e)
				}
			case lk == "lk" || containsAny(lk, examTypeMarkers...) || containsAny(ld, examTextMarkers...):
				seen[key] = true
				exams = append(exams, e)
			}
		}
	})
	byDate := func(es []Entry) {
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Date != es[j].Date {
				return es[i].Date < es[j].Date
			}
			return es[i].Nr < es[j].Nr
		})
	}
	byDate(homework)
	byDate(exams)
	return homework, exams
}

// subjectOf looks at the item and its lesson/course relation only – not into
// child lists, so a day's note doesn't pick up the subject of its first lesson.
func subjectOf(m obj) string {
	if t := directText(m, subjectKeys, text); t != "" {
		return t
	}
	for _, k := range []string{"lesson", "course", "group"} {
		if n, ok := m[k].(obj); ok {
			if t := directText(n, subjectKeys, text); t != "" {
				return t
			}
		}
	}
	return ""
}
