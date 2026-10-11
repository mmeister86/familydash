package scenes

import (
	"testing"
)

func strptr(s string) *string {
	return &s
}

func customFeed() Feed {
	return Feed{
		Version:  1,
		Revision: 7,
		Grid:     Grid{Cols: 12, Rows: 24},
		Scenes: []Scene{
			{ID: "morning", Name: "Morgen", Kind: "layout", Start: Start{SchoolDay: strptr("06:45"), Weekend: strptr("07:30")}},
			{ID: "s-abc123", Name: "Hausaufgaben", Kind: "layout", Start: Start{SchoolDay: strptr("15:30"), Weekend: nil}},
			{ID: "night", Name: "Nacht", Kind: "night", Start: Start{SchoolDay: strptr("21:30"), Weekend: strptr("21:30")}},
		},
	}
}

func TestFeedSchedule(t *testing.T) {
	sched, err := customFeed().Schedule()
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if len(sched.SchoolDay) != 3 || len(sched.Weekend) != 2 {
		t.Fatalf("starts = %d/%d, want 3/2", len(sched.SchoolDay), len(sched.Weekend))
	}
	got := sched.SchoolDay[1]
	if got.ID != "s-abc123" || got.Name != "Hausaufgaben" || got.Kind != "layout" || got.Minute != 15*60+30 {
		t.Errorf("schoolDay[1] = %+v", got)
	}
	for _, st := range sched.Weekend {
		if st.ID == "s-abc123" {
			t.Errorf("null weekend start should be skipped: %+v", sched.Weekend)
		}
	}
}

func TestFeedScheduleBadTime(t *testing.T) {
	feed := customFeed()
	feed.Scenes[0].Start.SchoolDay = strptr("xx")
	if _, err := feed.Schedule(); err == nil {
		t.Error("bad start time should fail")
	}
}
