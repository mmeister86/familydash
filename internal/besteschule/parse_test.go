package besteschule

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

// Shapes follow what the Home Assistant integration (RF1705/beste-schule)
// parses: {"data": …} envelopes, lessons with nr/weekday/time relation,
// substitution days with dated lessons and journal lessons with typed notes.
const fixture = `{
 "students": {"data": [{"id": 11, "forename": "Lukas", "name": "Meister", "meta_groups": [{"id": 1, "name": "7b"}]}]},
 "timetable": {"data": {
   "valid_from": "2026-08-17", "valid_to": "2027-07-10",
   "no_school_dates": ["2026-10-05"],
   "weeks": [{"nr": 40, "year": "2026", "types": ["A"]}, {"nr": 41, "year": "2026", "types": ["B"]}],
   "lessons": [
     {"weekday": 3, "nr": 1, "subject": {"name": "Mathematik", "local_id": "MA"}, "rooms": [{"local_id": "R12"}], "teachers": [{"forename": "Anna", "name": "Schulz"}], "time": {"from": "07:30", "to": "08:15"}},
     {"weekday": 3, "nr": 2, "subject": {"name": "Deutsch"}, "rooms": [{"local_id": "R12"}], "time": {"from": "08:25", "to": "09:10"}},
     {"weekday": 3, "nr": 3, "weeks": ["A"], "subject": {"name": "Englisch"}, "time": {"from": "09:30", "to": "10:15"}},
     {"weekday": 3, "nr": 3, "weeks": ["B"], "subject": {"name": "Musik"}, "time": {"from": "09:30", "to": "10:15"}},
     {"weekday": 3, "nr": 4, "subject": {"name": "Sport"}, "rooms": [{"local_id": "TH"}], "time": {"from": "10:25", "to": "11:10"}},
     {"weekday": 4, "nr": 1, "subject": {"name": "Biologie"}, "time": {"from": "07:30", "to": "08:15"}},
     {"weekday": 4, "nr": 2, "subject": {"name": "Geschichte"}, "time": {"from": "08:25", "to": "09:10"}},
     {"weekday": 5, "nr": 1, "subject": {"name": "Kunst"}, "time": {"from": "07:30", "to": "08:15"}},
     {"weekday": 1, "nr": 1, "subject": {"name": "Physik"}, "time": {"from": "07:30", "to": "08:15"}},
     {"weekday": 2, "nr": 1, "subject": {"name": "Chemie"}, "time": {"from": "07:30", "to": "08:15"}}
   ]}},
 "substitutions": {"data": [
   {"date": "2026-09-30", "notes": [{"text": "Heute Kurzstunden wegen Hitze"}], "lessons": [
     {"nr": 2, "status": "cancelled", "subject": {"name": "Deutsch"}},
     {"nr": 4, "status": "planned", "subject": {"name": "Mathematik"}, "rooms": [{"local_id": "TH"}, {"local_id": "R7"}]},
     {"nr": 1, "status": "initial", "subject": {"name": "Mathematik"}}
   ]},
   {"date": "2026-10-01", "lessons": [{"nr": 3, "status": "planned", "subject": {"name": "Informatik"}, "notes": "Zusatzstunde"}]}
 ]},
 "journal": {"11": {"data": [
   {"date": "2026-10-01", "nr": 2, "subject": {"name": "Deutsch"}, "notes": [{"id": 1, "type": {"name": "Hausaufgabe"}, "description": "Arbeitsblatt S. 3 fertig"}]},
   {"date": "2026-10-08", "nr": 3, "subject": {"name": "Mathematik"}, "notes": [{"id": 2, "type": {"name": "Klassenarbeit"}, "description": "Bruchrechnung"}]},
   {"date": "2026-10-02", "nr": 1, "subject": {"name": "Englisch"}, "notes": [{"id": 3, "type": {"name": "Sonstiges"}, "description": "Vokabeltest Unit 2"}]},
   {"date": "2026-09-20", "nr": 1, "subject": {"name": "Kunst"}, "notes": [{"id": 4, "type": {"name": "Hausaufgabe"}, "description": "alt"}]},
   {"date": "2026-10-01", "nr": 2, "subject": {"name": "Deutsch"}, "notes": [{"id": 1, "type": {"name": "Hausaufgabe"}, "description": "Arbeitsblatt S. 3 fertig"}]}
 ]}}
}`

func load(t *testing.T, js string) *Raw {
	t.Helper()
	var r Raw
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func lessons(d Day) string {
	var out []string
	for _, l := range d.Lessons {
		s := l.Start + " " + l.Subject
		if l.Room != "" {
			s += " [" + l.Room + "]"
		}
		if l.Status != "" {
			s += " " + l.Status
		}
		if l.Info != "" {
			s += " (" + l.Info + ")"
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

func TestTodayWithSubstitutions(t *testing.T) {
	raw := load(t, fixture)
	sc := Build(raw, time.Date(2026, 9, 30, 9, 0, 0, 0, berlin), berlin, nil)
	if len(sc.Students) != 1 || sc.Students[0].Name != "Lukas" {
		t.Fatalf("students: %+v", sc.Students)
	}
	d := sc.Students[0].Day
	want := strings.Join([]string{
		"07:30 Mathematik [R12]",
		"08:25 Deutsch [R12] cancelled",
		"09:30 Englisch",
		"10:25 Mathematik [R7] substitution (statt Sport)",
	}, "\n")
	if d.Date != "2026-09-30" || lessons(d) != want {
		t.Fatalf("day %s:\n%s\nwant:\n%s", d.Date, lessons(d), want)
	}
	if len(d.Notices) != 1 || !strings.Contains(d.Notices[0], "Kurzstunden") {
		t.Errorf("notices: %q", d.Notices)
	}
	if d.Lessons[0].Teacher != "Anna Schulz" {
		t.Errorf("teacher: %q", d.Lessons[0].Teacher)
	}
}

func TestAfterSchoolShowsNextDayWithExtraLesson(t *testing.T) {
	raw := load(t, fixture)
	d := Build(raw, time.Date(2026, 9, 30, 13, 0, 0, 0, berlin), berlin, nil).Students[0].Day
	want := "07:30 Biologie\n08:25 Geschichte\n09:30 Informatik substitution (Zusatzstunde)"
	if d.Date != "2026-10-01" || lessons(d) != want {
		t.Fatalf("day %s:\n%s", d.Date, lessons(d))
	}
}

func TestWeekendAndHolidaySkipped(t *testing.T) {
	raw := load(t, fixture)
	// Friday afternoon → Mon 5.10. is a no-school date → Tue 6.10.
	d := Build(raw, time.Date(2026, 10, 2, 15, 0, 0, 0, berlin), berlin, nil).Students[0].Day
	if d.Date != "2026-10-06" || lessons(d) != "07:30 Chemie" {
		t.Fatalf("day %s:\n%s", d.Date, lessons(d))
	}
	// A/B weeks: Wed 7.10. is week 41 = B → Musik instead of Englisch
	d = Build(raw, time.Date(2026, 10, 7, 7, 0, 0, 0, berlin), berlin, nil).Students[0].Day
	if !strings.Contains(lessons(d), "Musik") || strings.Contains(lessons(d), "Englisch") {
		t.Fatalf("week B:\n%s", lessons(d))
	}
}

func TestHomeworkAndExams(t *testing.T) {
	raw := load(t, fixture)
	s := Build(raw, time.Date(2026, 9, 30, 9, 0, 0, 0, berlin), berlin, nil).Students[0]
	if len(s.Homework) != 1 || s.Homework[0].Subject != "Deutsch" || s.Homework[0].Date != "2026-10-01" {
		t.Errorf("homework (deduped, past dropped, 'Arbeitsblatt' is not an exam): %+v", s.Homework)
	}
	if len(s.Exams) != 2 || s.Exams[0].Text != "Vokabeltest Unit 2" || s.Exams[1].Kind != "Klassenarbeit" {
		t.Errorf("exams: %+v", s.Exams)
	}
}

func TestMultipleStudentsFilteredByGroup(t *testing.T) {
	js := `{
	 "students": {"data": [
	   {"id": 1, "forename": "Lukas", "meta_groups": [{"id": 10, "name": "7b"}, {"id": 12, "name": "7b-rel"}]},
	   {"id": 2, "forename": "Mia", "meta_groups": [{"id": 20, "name": "5a"}]}]},
	 "timetable": {"data": {"lessons": [
	   {"weekday": 3, "nr": 1, "subject": {"name": "Religion"}, "group": {"id": 12, "name": "7b-rel"}, "time": {"from": "07:30", "to": "08:15"}},
	   {"weekday": 3, "nr": 1, "subject": {"name": "Ethik"}, "group": {"id": 13, "name": "7b-eth"}, "time": {"from": "07:30", "to": "08:15"}},
	   {"weekday": 3, "nr": 2, "subject": {"name": "Deutsch"}, "group": {"id": 10, "name": "7b"}, "time": {"from": "08:25", "to": "09:10"}},
	   {"weekday": 3, "nr": 2, "subject": {"name": "Sachkunde"}, "group": {"id": 20, "name": "5a"}, "time": {"from": "08:25", "to": "09:10"}},
	   {"weekday": 3, "nr": 3, "subject": {"name": "Mathe"}, "group": {"name": "7 b Mathe-Kurs b"}, "time": {"from": "09:30", "to": "10:15"}}
	 ]}}}`
	sc := Build(load(t, js), time.Date(2026, 9, 30, 7, 0, 0, 0, berlin), berlin, nil)
	if got := lessons(sc.Students[0].Day); got != "07:30 Religion\n08:25 Deutsch\n09:30 Mathe" {
		t.Errorf("Lukas:\n%s", got)
	}
	if got := lessons(sc.Students[1].Day); got != "08:25 Sachkunde" {
		t.Errorf("Mia:\n%s", got)
	}
	only := Build(load(t, js), time.Date(2026, 9, 30, 7, 0, 0, 0, berlin), berlin, []string{"mia"})
	if len(only.Students) != 1 || only.Students[0].Name != "Mia" {
		t.Errorf("BESTESCHULE_STUDENTS filter: %+v", only.Students)
	}
}

func TestNoSchoolAtAll(t *testing.T) {
	raw := load(t, `{"students": {"data": [{"id": 1, "forename": "Lukas"}]}, "timetable": {"data": {"valid_to": "2026-07-01", "lessons": [{"weekday": 3, "nr": 1, "subject": "Mathe", "time": {"from": "07:30", "to": "08:15"}}]}}}`)
	d := Build(raw, time.Date(2026, 9, 30, 7, 0, 0, 0, berlin), berlin, nil).Students[0].Day
	if !d.NoSchool || len(d.Lessons) != 0 {
		t.Errorf("expected noSchool: %+v", d)
	}
}

func TestBuildDaysWeek(t *testing.T) {
	raw := load(t, fixture)
	days := BuildDays(raw, time.Date(2026, 9, 30, 18, 0, 0, 0, berlin), 7, berlin, nil)["Lukas"]
	if len(days) != 7 {
		t.Fatalf("days = %d", len(days))
	}
	want := []struct {
		date    string
		lessons int
	}{
		{"2026-09-30", 4}, // Wed, after school still today (no jump to the next day)
		{"2026-10-01", 3}, // Thu + extra Informatik
		{"2026-10-02", 1}, // Fri
		{"2026-10-03", 0}, // Sat
		{"2026-10-04", 0}, // Sun
		{"2026-10-05", 0}, // no_school_dates
		{"2026-10-06", 1}, // Tue
	}
	for i, w := range want {
		if days[i].Date != w.date || len(days[i].Lessons) != w.lessons {
			t.Errorf("day %d: %s with %d lessons, want %s with %d:\n%s", i, days[i].Date, len(days[i].Lessons), w.date, w.lessons, lessons(days[i]))
		}
	}
	if days[0].Lessons[1].Status != "cancelled" {
		t.Errorf("substitution lost: %+v", days[0].Lessons[1])
	}
}
