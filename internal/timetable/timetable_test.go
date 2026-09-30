package timetable

import (
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
