package waste

import (
	"reflect"
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

// Crimmitschau, Karl-Marx-Straße (Landratsamt Zwickau)
func crimmitschau(shift bool) *Service {
	return &Service{Shift: shift, Bins: []Bin{
		{Name: "Restabfall", Weekday: time.Wednesday, Weeks: Even},
		{Name: "Gelbe Tonne", Weekday: time.Thursday, Weeks: Even},
		{Name: "Blaue Tonne", Weekday: time.Thursday, Weeks: Odd},
	}}
}

func dates(ps []Pickup) map[string][]string {
	out := map[string][]string{}
	for _, p := range ps {
		out[p.Name] = p.Dates
	}
	return out
}

func TestRules(t *testing.T) {
	// Wed 30.09.2026 is KW 40 (even)
	got := dates(crimmitschau(false).Snapshot(at("2026-09-30 18:00"), berlin))
	want := map[string][]string{
		"Restabfall":  {"2026-09-30", "2026-10-14"}, // today still listed
		"Gelbe Tonne": {"2026-10-01", "2026-10-15"},
		"Blaue Tonne": {"2026-10-08", "2026-10-22"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestHolidayShift(t *testing.T) {
	// Buß- und Bettag Wed 18.11.2026 (KW 47, odd) → Blaue Tonne Thu 19.11. moves to Fri 20.11.
	ps := crimmitschau(true).Snapshot(at("2026-11-16 08:00"), berlin)
	blue := ps[2]
	if blue.Dates[0] != "2026-11-20" || len(blue.Shifted) != 1 || blue.Shifted[0] != "2026-11-20" {
		t.Fatalf("blue = %+v", blue)
	}
	// without shifting it stays on Thursday
	if d := crimmitschau(false).Snapshot(at("2026-11-16 08:00"), berlin)[2].Dates[0]; d != "2026-11-19" {
		t.Fatalf("unshifted = %s", d)
	}
	// Christmas week 2026: Fri 25.12. is after Wed/Thu → no shift (KW 52, even)
	ps = crimmitschau(true).Snapshot(at("2026-12-21 08:00"), berlin)
	if ps[0].Dates[0] != "2026-12-23" || ps[1].Dates[0] != "2026-12-24" {
		t.Fatalf("christmas = %v", dates(ps))
	}
	// a shifted date from last week stays visible on its new day
	ps = crimmitschau(true).Snapshot(at("2026-11-20 07:00"), berlin)
	if ps[2].Dates[0] != "2026-11-20" {
		t.Fatalf("shifted today = %v", ps[2].Dates)
	}
}

func TestYearBoundary(t *testing.T) {
	// KW 53/2026 (odd) is followed by KW 1/2027 (odd): two blue weeks in a row
	ps := crimmitschau(false).Snapshot(at("2026-12-28 08:00"), berlin)
	if got := ps[2].Dates; !reflect.DeepEqual(got, []string{"2026-12-31", "2027-01-07"}) {
		t.Fatalf("blue = %v", got)
	}
}

func TestHolidays(t *testing.T) {
	for _, d := range []string{"2027-03-26", "2027-03-29", "2027-05-06", "2027-05-17", "2026-11-18", "2026-10-31", "2027-11-17"} {
		if !IsHoliday(at(d + " 12:00")) {
			t.Errorf("%s should be a holiday", d)
		}
	}
	for _, d := range []string{"2026-11-19", "2027-03-30", "2026-12-24"} {
		if IsHoliday(at(d + " 12:00")) {
			t.Errorf("%s is no holiday", d)
		}
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"Mi", "mittwochs", "Mittwoch", "wed", "Wednesday"} {
		if wd, err := ParseWeekday(s); err != nil || wd != time.Wednesday {
			t.Errorf("%q → %v %v", s, wd, err)
		}
	}
	if wd, _ := ParseWeekday("donnerstags"); wd != time.Thursday {
		t.Error("donnerstags")
	}
	if p, _ := ParseParity("ungerade"); p != Odd {
		t.Error("ungerade")
	}
	if _, err := ParseParity("manchmal"); err == nil {
		t.Error("manchmal accepted")
	}
}
