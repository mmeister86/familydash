package briefing

import (
	"context"
	"errors"
	"testing"
	"time"

	"familydash/internal/scene"
)

func widgetSched() scene.Schedule {
	return scene.Schedule{
		SchoolDay: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 6*60 + 45},
			{ID: "s-hw", Name: "Hausaufgaben", Kind: "layout", Minute: 15*60 + 30},
			{ID: "evening", Name: "Abend", Kind: "layout", Minute: 19 * 60},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
		Weekend: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 7*60 + 30},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
	}
}

func serviceWithPlan(t *testing.T, sched scene.Schedule, plan WidgetPlan) *Service {
	t.Helper()
	d := sample()
	return &Service{
		Model:  nil,
		Sched:  scene.NewProvider(sched),
		Loc:    berlin,
		Lead:   20 * time.Minute,
		MinGap: 20 * time.Minute,
		Gather: func(time.Time) Data {
			return d
		},
		Plan: func() (WidgetPlan, error) {
			return plan, nil
		},
		started: at("2026-10-01 00:00"),
	}
}

func TestKindAtFollowsWidgets(t *testing.T) {
	s := serviceWithPlan(t, widgetSched(), WidgetPlan{"s-hw": {Today: true}})
	// Thu 2026-10-01: the custom scene starts within the lead time.
	if kind, ok := s.KindAt(at("2026-10-01 15:10")); !ok || kind != Morning {
		t.Errorf("KindAt(15:10) = %q,%v, want morning", kind, ok)
	}
	// Morning has no widget and today is used elsewhere: nothing due.
	if kind, ok := s.KindAt(at("2026-10-01 06:25")); ok {
		t.Errorf("KindAt(06:25) = %q, want nothing", kind)
	}
}

func TestKindAtDefaultsWithoutWidgets(t *testing.T) {
	s := serviceWithPlan(t, widgetSched(), WidgetPlan{})
	if kind, ok := s.KindAt(at("2026-10-01 06:30")); !ok || kind != Morning {
		t.Errorf("KindAt(06:30) = %q,%v, want morning", kind, ok)
	}
	if kind, ok := s.KindAt(at("2026-10-01 18:45")); !ok || kind != Evening {
		t.Errorf("KindAt(18:45) = %q,%v, want evening", kind, ok)
	}
	s.Plan = func() (WidgetPlan, error) { return nil, errors.New("feed down") }
	if kind, ok := s.KindAt(at("2026-10-01 06:30")); !ok || kind != Morning {
		t.Errorf("feed down KindAt(06:30) = %q,%v, want morning", kind, ok)
	}
}

func TestSnapshotShowsWidgetScene(t *testing.T) {
	s := serviceWithPlan(t, widgetSched(), WidgetPlan{"s-hw": {Tomorrow: true}})
	s.tick(context.Background(), at("2026-10-01 15:10"))
	b := s.snapshotAt(at("2026-10-01 16:00"))
	if b == nil || b.Kind != Evening || b.Date != "2026-10-02" {
		t.Fatalf("custom scene card: %+v", b)
	}
}

func TestKindAtUsesConfiguredTimesWithoutPlan(t *testing.T) {
	sched := scene.Schedule{
		SchoolDay: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 5 * 60},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
		Weekend: []scene.Start{
			{ID: "morning", Name: "Morgen", Kind: "layout", Minute: 5 * 60},
			{ID: "night", Name: "Nacht", Kind: "night", Minute: 21*60 + 30},
		},
	}
	s := serviceWithPlan(t, sched, nil)
	s.Plan = nil // feed unavailable: no widget info at all
	// Thu 2026-10-01 04:45: configured morning (05:00) starts within the lead.
	if kind, ok := s.KindAt(at("2026-10-01 04:45")); !ok || kind != Morning {
		t.Errorf("KindAt(04:45) = %q,%v, want morning (configured 05:00 start)", kind, ok)
	}
}

func TestKindForScene(t *testing.T) {
	s := serviceWithPlan(t, widgetSched(), WidgetPlan{"s-hw": {Today: true}})
	if kind, ok := s.KindForScene("s-hw"); !ok || kind != Morning {
		t.Errorf("KindForScene(s-hw) = %q,%v, want morning", kind, ok)
	}
	if _, ok := s.KindForScene("morning"); ok {
		t.Error("KindForScene(morning) should fail: no widget, today used elsewhere")
	}
	if _, ok := s.KindForScene("nope"); ok {
		t.Error("KindForScene(nope) should fail")
	}
	plain := serviceWithPlan(t, widgetSched(), WidgetPlan{})
	if kind, ok := plain.KindForScene("morning"); !ok || kind != Morning {
		t.Errorf("legacy KindForScene(morning) = %q,%v", kind, ok)
	}
	if kind, ok := plain.KindForScene("evening"); !ok || kind != Evening {
		t.Errorf("legacy KindForScene(evening) = %q,%v", kind, ok)
	}
}
