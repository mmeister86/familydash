package calendar

import (
	"testing"
	"time"
)

var berlinTest, _ = time.LoadLocation("Europe/Berlin")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, berlinTest)
	if err != nil {
		panic(err)
	}
	return t
}

func centralFixture() Snapshot {
	succ := at("2026-10-02 06:55")
	return Snapshot{
		Central:               true,
		ConfigurationRevision: 7,
		UpdatedAt:             succ,
		People: []Person{
			{ID: "u-lukas", Slug: "lukas", Name: "Lukas Meister", Role: "child"},
			{ID: "u-hannah", Slug: "hannah", Name: "Hannah Meister", Role: "child"},
		},
		Bindings: []PersonBinding{
			{PersonID: "u-lukas", Kind: "besteschule", ExternalID: "bs-lukas"},
			{PersonID: "u-hannah", Kind: "timetable", ExternalID: "plan-01"},
		},
		Calendars: []CalendarInfo{
			{ID: 0, CalendarID: "cal-lukas", Name: "Schule Lukas", Color: "#F2B53A", Panel: "school", Into: -1,
				PersonIDs: []string{"u-lukas"},
				Status:    SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"}},
			{ID: 1, CalendarID: "cal-hannah", Name: "Schule Hannah", Color: "#46C28E", Panel: "school", Into: -1,
				PersonIDs: []string{"u-hannah"},
				Status:    SourceStatus{LastAttemptAt: succ, LastSuccessAt: succ, Freshness: "fresh", LastAttemptStatus: "success"}},
		},
	}
}

func TestPersonForSourceExactMatch(t *testing.T) {
	s := centralFixture()
	b, ok := PersonForSource(s, "besteschule", "bs-lukas")
	if !ok || b.PersonID != "u-lukas" {
		t.Fatalf("besteschule/bs-lukas = %+v %v, want u-lukas", b, ok)
	}
	b, ok = PersonForSource(s, "timetable", "plan-01")
	if !ok || b.PersonID != "u-hannah" {
		t.Fatalf("timetable/plan-01 = %+v %v, want u-hannah", b, ok)
	}
	// wrong kind or unknown id never resolve
	if _, ok := PersonForSource(s, "timetable", "bs-lukas"); ok {
		t.Error("kind mismatch resolved")
	}
	if _, ok := PersonForSource(s, "besteschule", "bs-unknown"); ok {
		t.Error("unknown external id resolved")
	}
	if _, ok := PersonForSource(s, "", ""); ok {
		t.Error("empty lookup resolved")
	}
}

func TestPersonForSourceNeverGuesses(t *testing.T) {
	s := centralFixture()
	// same names exist, but without an exact binding nothing resolves
	if _, ok := PersonForSource(s, "besteschule", "Lukas Meister"); ok {
		t.Error("name resolved without a stable binding")
	}
	// local mode never resolves bindings, even if some were present
	s.Central = false
	if _, ok := PersonForSource(s, "besteschule", "bs-lukas"); ok {
		t.Error("local mode resolved a binding")
	}
}

func TestCalendarsForPersonIncludesAllAssigned(t *testing.T) {
	s := centralFixture()
	got := CalendarsForPerson(s, "u-lukas")
	if len(got) != 1 || got[0].CalendarID != "cal-lukas" {
		t.Fatalf("calendars for u-lukas = %+v", got)
	}
	if got := CalendarsForPerson(s, "u-unknown"); len(got) != 0 {
		t.Fatalf("unknown person has calendars: %+v", got)
	}
}

func TestAssignedCalendarStateOldestAndMissing(t *testing.T) {
	s := centralFixture()
	oldest, missing := AssignedCalendarState(s, "u-lukas", "school")
	if len(missing) != 0 || !oldest.Equal(at("2026-10-02 06:55")) {
		t.Fatalf("state = %v %v", oldest, missing)
	}
	// a required calendar that never loaded is reported missing, never fresh
	s.Calendars[0].Status = SourceStatus{Freshness: "neverLoaded"}
	if _, missing := AssignedCalendarState(s, "u-lukas", "school"); len(missing) != 1 || missing[0] != "cal-lukas" {
		t.Fatalf("missing = %v, want [cal-lukas]", missing)
	}
	// disabled calendars are not required
	s.Calendars[0].Status = SourceStatus{Freshness: "disabled"}
	if _, missing := AssignedCalendarState(s, "u-lukas", "school"); len(missing) != 0 {
		t.Fatalf("disabled counts as missing: %v", missing)
	}
	// other panels do not count towards the school contribution
	if _, missing := AssignedCalendarState(s, "u-hannah", "column"); len(missing) != 0 {
		t.Fatalf("column contribution: %v", missing)
	}
}
