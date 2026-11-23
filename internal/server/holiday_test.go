package server

import (
	"testing"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/timetable"
)

var holidayLoc, _ = time.LoadLocation("Europe/Berlin")

func holidayAt(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, holidayLoc)
	if err != nil {
		panic(err)
	}
	return t
}

func holidayPlan(t *testing.T) *timetable.File {
	t.Helper()
	f, err := timetable.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// In the autumn holidays a beste.schule student off without a banner gets the
// fixed timetable's ("Herbstferien bis 24.10.") – once, and school days stay
// untouched.
func TestApplyHolidayNotices(t *testing.T) {
	now := holidayAt("2026-10-12 07:30")
	mkSchool := func() *besteschule.School {
		return &besteschule.School{Students: []besteschule.Student{
			{Name: "Lukas", Day: besteschule.Day{Date: "2026-10-12", NoSchool: true, Lessons: []besteschule.Lesson{}}},
			{Name: "Schule", Day: besteschule.Day{Date: "2026-10-12", Lessons: []besteschule.Lesson{
				{Nr: 1, Start: "07:30", Subject: "Mathe"},
			}}},
		}}
	}

	school := mkSchool()
	applyHolidayNotices(school, holidayPlan(t), now)
	lukas, other := school.Students[0].Day, school.Students[1].Day
	if len(lukas.Notices) != 1 || lukas.Notices[0] != "Herbstferien bis 24.10." {
		t.Fatalf("Lukas: %+v", lukas)
	}
	if len(other.Notices) != 0 {
		t.Fatalf("school day touched: %+v", other)
	}

	// idempotent across polls, and outside holidays nothing happens
	applyHolidayNotices(school, holidayPlan(t), now)
	if len(school.Students[0].Day.Notices) != 1 {
		t.Fatalf("duplicated: %+v", school.Students[0].Day)
	}
	plain := mkSchool()
	applyHolidayNotices(plain, holidayPlan(t), holidayAt("2026-10-09 07:30"))
	if len(plain.Students[0].Day.Notices) != 0 {
		t.Fatalf("outside holidays: %+v", plain.Students[0].Day)
	}
}
