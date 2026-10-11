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
