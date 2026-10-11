package scene

import (
	"testing"
	"time"
)

func mustBuild(t *testing.T, m map[string]string, fallback map[string]string) []Start {
	t.Helper()
	list, err := Build(func(n string) string {
		if v, ok := m[n]; ok {
			return v
		}
		return fallback[n]
	})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func defaults(t *testing.T) *Schedule {
	return &Schedule{
		SchoolDay: mustBuild(t, DefaultSchoolDay, nil),
		Weekend:   mustBuild(t, DefaultWeekend, DefaultSchoolDay),
	}
}

func TestAt(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := defaults(t)
	cases := []struct {
		when      string
		want      string
		until     string
		schoolDay bool
	}{
		// Wed 2026-09-30
		{"2026-09-30 06:44", Night, "2026-09-30 06:45", true},
		{"2026-09-30 06:45", Morning, "2026-09-30 09:00", true},
		{"2026-09-30 08:59", Morning, "2026-09-30 09:00", true},
		{"2026-09-30 12:00", Day, "2026-09-30 14:00", true},
		{"2026-09-30 16:30", Afternoon, "2026-09-30 19:00", true},
		{"2026-09-30 19:10", Evening, "2026-09-30 21:30", true},
		{"2026-09-30 23:00", Night, "2026-10-01 06:45", true},
		// Fri night → Saturday morning starts later
		{"2026-10-02 22:00", Night, "2026-10-03 07:30", true},
		{"2026-10-03 07:00", Night, "2026-10-03 07:30", false},
		{"2026-10-03 08:00", Morning, "2026-10-03 10:00", false},
		{"2026-10-03 10:00", Day, "2026-10-03 14:00", false},
		// Sun night → Monday 06:45
		{"2026-10-04 22:00", Night, "2026-10-05 06:45", false},
	}
	for _, c := range cases {
		now, _ := time.ParseInLocation("2006-01-02 15:04", c.when, loc)
		got := s.At(now, loc)
		if got.ID != c.want {
			t.Errorf("%s: scene %q, want %q", c.when, got.ID, c.want)
		}
		if u := got.Until.Format("2006-01-02 15:04"); u != c.until {
			t.Errorf("%s: until %s, want %s", c.when, u, c.until)
		}
		if got.SchoolDay != c.schoolDay {
			t.Errorf("%s: schoolDay %v", c.when, got.SchoolDay)
		}
	}
}

func TestCurrentIdentity(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := defaults(t)
	morning, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-30 06:45", loc)
	got := s.At(morning, loc)
	if got.ID != Morning || got.Name != "Morgen" || got.Kind != "layout" {
		t.Errorf("morning identity = %+v", got)
	}
	night, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-30 23:00", loc)
	got = s.At(night, loc)
	if got.ID != Night || got.Name != "Nacht" || got.Kind != "night" {
		t.Errorf("night identity = %+v", got)
	}
}

func TestSkipAndForce(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := &Schedule{
		SchoolDay: mustBuild(t, map[string]string{Afternoon: "off"}, DefaultSchoolDay),
		Weekend:   mustBuild(t, DefaultWeekend, DefaultSchoolDay),
	}
	now := time.Date(2026, 9, 30, 16, 0, 0, 0, loc)
	if got := s.At(now, loc).ID; got != Day {
		t.Errorf("afternoon off: got %q, want day", got)
	}
	s.Force = Night
	if got := s.At(now, loc); got.ID != Night || !got.Forced {
		t.Errorf("force: got %+v", got)
	}
	s.Force = "brunch"
	if got := s.At(now, loc); got.ID != Day || got.Forced {
		t.Errorf("unknown force should be ignored, got %+v", got)
	}
}

func TestEmptySchedule(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := &Schedule{}
	if got := s.At(time.Now(), loc); got.ID != Day || got.Name != "Tag" || got.Kind != "layout" {
		t.Errorf("empty schedule: %+v", got)
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int{"06:00": 360, "6:05": 365, "21.30": 1290, "7": 420, "off": -1} {
		got, err := ParseClock(in)
		if err != nil || got != want {
			t.Errorf("ParseClock(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"24:00", "abc", "7:61"} {
		if _, err := ParseClock(bad); err == nil {
			t.Errorf("ParseClock(%q): want error", bad)
		}
	}
}
