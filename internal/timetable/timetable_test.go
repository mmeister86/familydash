package timetable

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, berlin)
	if err != nil {
		panic(err)
	}
	return t
}

func plan(t *testing.T) *File {
	t.Helper()
	f, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func subjects(c Card) []string {
	var out []string
	for _, l := range c.Day.Lessons {
		out = append(out, l.Subject)
	}
	return out
}

func TestEmbeddedPlan(t *testing.T) {
	f := plan(t)
	if len(f.Children) != 1 || f.Children[0].Name != "Hannah" {
		t.Fatalf("children = %+v", f.Children)
	}
}

func TestTuesdayWithAfternoonClub(t *testing.T) {
	// Tue 29.09.2026, 10:00 → today's plan incl. GTA at 14:00
	c := plan(t).Build(at("2026-09-29 10:00"), berlin)[0]
	if c.Day.Date != "2026-09-29" {
		t.Fatalf("date = %s", c.Day.Date)
	}
	got := subjects(c)
	if len(got) != 7 || got[0] != "Deutsch" || got[6] != "Kreativwerkstatt" {
		t.Fatalf("lessons = %v", got)
	}
	gta := c.Day.Lessons[6]
	if gta.Tag != "GTA" || gta.Start != "14:00" || gta.End != "15:00" || gta.Nr != 0 {
		t.Fatalf("gta = %+v", gta)
	}
	if l := c.Day.Lessons[0]; l.Nr != 1 || l.Start != "08:15" || l.End != "09:00" {
		t.Fatalf("first = %+v", l)
	}
}

func TestSwitchesAfterLastLesson(t *testing.T) {
	// Tue ends with GTA at 15:00: 14:30 still Tuesday, 15:00 → Wednesday
	if d := plan(t).Build(at("2026-09-29 14:30"), berlin)[0].Day.Date; d != "2026-09-29" {
		t.Fatalf("14:30 → %s", d)
	}
	if d := plan(t).Build(at("2026-09-29 15:00"), berlin)[0].Day.Date; d != "2026-09-30" {
		t.Fatalf("15:00 → %s", d)
	}
	// Friday afternoon → Monday
	c := plan(t).Build(at("2026-10-02 13:00"), berlin)[0]
	if c.Day.Date != "2026-10-05" || subjects(c)[0] != "Sport / Eislaufen" {
		t.Fatalf("friday → %s %v", c.Day.Date, subjects(c))
	}
}

func TestFridayIsShort(t *testing.T) {
	c := plan(t).Build(at("2026-10-02 07:00"), berlin)[0]
	if got := subjects(c); len(got) != 4 || got[3] != "Förder Mathe" {
		t.Fatalf("friday = %v", got)
	}
}

func TestHolidays(t *testing.T) {
	// Friday before autumn break → next school day is Mon 26.10.
	c := plan(t).Build(at("2026-10-09 14:00"), berlin)[0]
	if c.Day.Date != "2026-10-26" {
		t.Fatalf("before break → %s", c.Day.Date)
	}
	// during the break: notice + first day after
	c = plan(t).Build(at("2026-10-14 09:00"), berlin)[0]
	if c.Day.Date != "2026-10-26" || len(c.Day.Notices) != 1 || c.Day.Notices[0] != "Herbstferien bis 24.10." {
		t.Fatalf("in break → %s %v", c.Day.Date, c.Day.Notices)
	}
	// Buß- und Bettag (Wed 18.11.) is skipped
	if d := plan(t).Build(at("2026-11-17 16:00"), berlin)[0].Day.Date; d != "2026-11-19" {
		t.Fatalf("Buß- und Bettag → %s", d)
	}
}

func TestEndOfSchoolYear(t *testing.T) {
	c := plan(t).Build(at("2027-07-20 09:00"), berlin)[0]
	if !c.Day.NoSchool {
		t.Fatalf("summer 2027 → %+v", c.Day)
	}
}

func TestWeekdayNames(t *testing.T) {
	for _, s := range []string{"Mo", "mo", "Montag", "Mon", "monday"} {
		if wd, ok := weekdayOf(s); !ok || wd != time.Monday {
			t.Errorf("%q → %v %v", s, wd, ok)
		}
	}
	if _, ok := weekdayOf("Xy"); ok {
		t.Error("Xy accepted")
	}
}

func TestABWeeks(t *testing.T) {
	f := plan(t)
	// week A (28.09.): Schulgarten Tue, Werken Wed
	if got := subjects(f.Build(at("2026-09-29 07:00"), berlin)[0]); got[3] != "Schulgarten" {
		t.Fatalf("Di A = %v", got)
	}
	if got := subjects(f.Build(at("2026-09-30 07:00"), berlin)[0]); got[4] != "Werken" || got[5] != "Werken" {
		t.Fatalf("Mi A = %v", got)
	}
	// week B (05.10.): Sachunterricht Tue, Kunst Wed
	if got := subjects(f.Build(at("2026-10-06 07:00"), berlin)[0]); got[3] != "Sachunterricht" {
		t.Fatalf("Di B = %v", got)
	}
	if got := subjects(f.Build(at("2026-10-07 07:00"), berlin)[0]); got[4] != "Kunst" || got[5] != "Kunst" {
		t.Fatalf("Mi B = %v", got)
	}
	// after the 2-week autumn break: 26.10. is 4 weeks after 28.09. → A again
	if got := subjects(f.Build(at("2026-10-27 07:00"), berlin)[0]); got[3] != "Schulgarten" {
		t.Fatalf("Di 27.10. = %v", got)
	}
	// across the year boundary (ISO week 53): 04.01.2027 is 14 weeks later → A
	if f.Children[0].isBWeek(at("2027-01-05 07:00")) {
		t.Fatal("05.01.2027 should be an A week")
	}
}

func TestThursdayWithoutEthik(t *testing.T) {
	got := subjects(plan(t).Build(at("2026-10-01 07:00"), berlin)[0])
	if len(got) != 6 || got[4] != "Kunst" || got[5] != "Ballspiele" {
		t.Fatalf("Do = %v", got)
	}
}

func TestSlotJSON(t *testing.T) {
	f := &File{}
	if err := json.Unmarshal([]byte(`{"children":[{"name":"X","days":{"Mo":["Mathe",{"A":"Werken","B":""}]}}]}`), f); err != nil {
		t.Fatal(err)
	}
	if s := f.Children[0].Days["Mo"]; s[0].A != "Mathe" || s[0].B != "Mathe" || s[1].A != "Werken" || s[1].B != "" {
		t.Fatalf("slots = %+v", s)
	}
}

func TestStableTimetableBinding(t *testing.T) {
	f := &File{}
	if err := json.Unmarshal([]byte(`{"children":[`+
		`{"name":"Hannah","id":"plan-01","periods":[["08:15","09:00"]],"days":{"Mo":["Mathe"]}},`+
		`{"name":"Lukas","periods":[["08:15","09:00"]],"days":{"Mo":["Deutsch"]}}]}`), f); err != nil {
		t.Fatal(err)
	}
	monday := at("2026-09-28 07:00")
	cards := f.Build(monday, berlin)
	if cards[0].Student.ID != "plan-01" {
		t.Fatalf("explicit id kept: %+v", cards[0].Student)
	}
	if cards[1].Student.ID != "plan-lukas" {
		t.Fatalf("local compat id: %+v", cards[1].Student)
	}
	// a rename keeps the stable id, so central bindings survive it
	f.Children[0].Name = "Hannah Meister"
	if got := f.Build(monday, berlin)[0].Student.ID; got != "plan-01" {
		t.Fatalf("rename kept id: %q", got)
	}
	days := f.DaysByID(monday, 2, berlin)
	if len(days["plan-01"]) != 2 || days["plan-01"][0].Date != "2026-09-28" {
		t.Fatalf("days by id: %+v", days["plan-01"])
	}
	// the built-in plan carries a stable non-name id
	if got := plan(t).Build(monday, berlin)[0].Student.ID; got != "plan-01" {
		t.Fatalf("built-in id: %q", got)
	}
	// duplicate explicit ids are a configuration error, never a silent merge
	path := t.TempDir() + "/plan.json"
	if err := os.WriteFile(path, []byte(`{"children":[{"name":"A","id":"x"},{"name":"B","id":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("duplicate ids accepted")
	}
}

func TestDaysWeek(t *testing.T) {
	days := plan(t).Days(at("2026-09-29 18:00"), 7, berlin)["Hannah"]
	if len(days) != 7 || days[0].Date != "2026-09-29" || days[6].Date != "2026-10-05" {
		t.Fatalf("days = %+v", days)
	}
	if len(days[0].Lessons) != 7 {
		t.Errorf("tuesday: %d lessons", len(days[0].Lessons))
	}
	sat := days[4] // 2026-10-03
	if sat.Date != "2026-10-03" || sat.Lessons == nil || len(sat.Lessons) != 0 {
		t.Errorf("saturday = %+v", sat)
	}
}
