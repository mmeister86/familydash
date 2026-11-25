package scenes

import (
	"encoding/json"
	"testing"
)

func TestDefaultFeedValid(t *testing.T) {
	feed := DefaultFeed()
	data, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseScenesFeed(data)
	if err != nil {
		t.Fatalf("default feed rejected: %v", err)
	}
	if len(parsed.Scenes) != 5 {
		t.Fatalf("scenes = %d, want 5", len(parsed.Scenes))
	}
	morning := parsed.Scenes[0]
	if morning.ID != "morning" || len(morning.Widgets) != 5 {
		t.Errorf("morning = %+v", morning)
	}
	if night := parsed.Scenes[4]; night.ID != "night" || night.Widgets != nil {
		t.Errorf("night = %+v", night)
	}
}

func TestDefaultHintsMorningOnly(t *testing.T) {
	feed := DefaultFeed()
	for _, s := range feed.Scenes {
		for _, w := range s.Widgets {
			if w.Type != "clock" || w.Props == nil || w.Props.Hints == nil {
				continue
			}
			if want := s.ID == "morning"; *w.Props.Hints != want {
				t.Errorf("%s hints = %v, want %v", s.ID, *w.Props.Hints, want)
			}
		}
	}
}

func TestDefaultFeedFillsGrid(t *testing.T) {
	feed := DefaultFeed()
	for _, s := range feed.Scenes {
		if s.Kind != "layout" {
			continue
		}
		bottom := 0
		for _, w := range s.Widgets {
			if w.Y+w.H > bottom {
				bottom = w.Y + w.H
			}
		}
		if bottom != 24 {
			t.Errorf("%s fills %d rows, want 24", s.ID, bottom)
		}
	}
}

func TestDefaultPalettes(t *testing.T) {
	want := map[string]string{"morning": "paper", "day": "paper", "afternoon": "paper", "evening": "board", "night": ""}
	for _, sc := range DefaultFeed().Scenes {
		if sc.Palette != want[sc.ID] {
			t.Errorf("%s palette = %q, want %q", sc.ID, sc.Palette, want[sc.ID])
		}
	}
}

func TestDefaultAfternoonScene(t *testing.T) {
	var got Scene
	for _, s := range DefaultFeed().Scenes {
		if s.ID == "afternoon" {
			got = s
		}
	}
	type rect struct {
		id, typ    string
		x, y, w, h int
	}
	want := []rect{
		{"clock", "clock", 0, 0, 12, 3},
		{"dayband", "dayband", 0, 3, 12, 4},
		{"school", "school", 0, 7, 12, 8},
		{"week", "week", 0, 15, 12, 7},
		{"shopping", "shopping", 0, 22, 12, 2},
	}
	if len(got.Widgets) != len(want) {
		t.Fatalf("afternoon widgets = %d, want %d", len(got.Widgets), len(want))
	}
	for i, w := range want {
		g := got.Widgets[i]
		if g.ID != w.id || g.Type != w.typ || g.X != w.x || g.Y != w.y || g.W != w.w || g.H != w.h {
			t.Errorf("widget %d = %s/%s %d,%d %dx%d, want %+v", i, g.ID, g.Type, g.X, g.Y, g.W, g.H, w)
		}
	}
	if p := got.Widgets[0].Props; p == nil || p.Wide == nil || !*p.Wide || p.Hints == nil || *p.Hints {
		t.Errorf("clock props = %+v, want wide:true hints:false", p)
	}
	if p := got.Widgets[2].Props; p == nil || p.Mode != "full" || p.MealDay != "today" {
		t.Errorf("school props = %+v, want mode full, mealDay today", p)
	}
}
