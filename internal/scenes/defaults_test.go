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
