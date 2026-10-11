package scene

import (
	"testing"
	"time"
)

func customSchedule() Schedule {
	return Schedule{
		SchoolDay: []Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 6*60 + 45},
			{ID: "s-abc123", Name: "Hausaufgaben", Kind: "layout", Minute: 15*60 + 30},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
		Weekend: []Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 7*60 + 30},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
	}
}

func TestCustomSchedule(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	sched := customSchedule()
	// Mon 2026-09-28 16:00 → custom scene with its identity.
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 16:00", loc)
	got := sched.At(mon, loc)
	if got.ID != "s-abc123" || got.Name != "Hausaufgaben" || got.Kind != "layout" {
		t.Errorf("mon 16:00 = %+v", got)
	}
	if got.Since.Format("15:04") != "15:30" || got.Until.Format("15:04") != "21:30" {
		t.Errorf("mon 16:00 boundaries = %v..%v", got.Since, got.Until)
	}
	// Sat 2026-10-03 16:00 → custom scene skipped, morning runs on.
	sat, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-03 16:00", loc)
	got = sched.At(sat, loc)
	if got.ID != "morning" {
		t.Errorf("sat 16:00 = %q, want morning", got.ID)
	}
	// Sun 2026-10-04 22:00 → night until Monday 06:45.
	sun, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-04 22:00", loc)
	got = sched.At(sun, loc)
	if got.ID != "night" || got.Kind != "night" {
		t.Errorf("sun 22:00 = %+v", got)
	}
	if u := got.Until.Format("2006-01-02 15:04"); u != "2026-10-05 06:45" {
		t.Errorf("sun 22:00 until = %s", u)
	}
}

func TestProviderSwap(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	fallback := &Schedule{
		SchoolDay: mustBuild(t, DefaultSchoolDay, nil),
		Weekend:   mustBuild(t, DefaultWeekend, DefaultSchoolDay),
	}
	p := NewProvider(*fallback)
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 16:00", loc)
	if got := p.At(mon, loc).ID; got != "afternoon" {
		t.Fatalf("before swap = %q, want afternoon", got)
	}
	p.SetSchedule(customSchedule())
	if got := p.At(mon, loc); got.ID != "s-abc123" || got.Name != "Hausaufgaben" {
		t.Errorf("after swap = %+v", got)
	}
}

func TestForceCustomID(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	sched := customSchedule()
	sched.Force = "s-abc123"
	morning, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 10:00", loc)
	got := sched.At(morning, loc)
	if got.ID != "s-abc123" || got.Name != "Hausaufgaben" || !got.Forced {
		t.Errorf("forced = %+v", got)
	}
}

func TestForceSurvivesSwap(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	fallback := &Schedule{
		SchoolDay: mustBuild(t, DefaultSchoolDay, nil),
		Weekend:   mustBuild(t, DefaultWeekend, DefaultSchoolDay),
		Force:     "evening",
	}
	p := NewProvider(*fallback)
	p.SetSchedule(customSchedule())
	mon, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-28 10:00", loc)
	// evening is gone after the swap: unknown force is ignored, not fatal.
	if got := p.At(mon, loc); got.ID != "morning" || got.Forced {
		t.Errorf("unknown force after swap = %+v", got)
	}
}
