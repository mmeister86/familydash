package scenes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFeedFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "dashboard-scenes-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func feedMap(t *testing.T) map[string]any {
	t.Helper()
	var feed map[string]any
	if err := json.Unmarshal(loadFeedFixture(t), &feed); err != nil {
		t.Fatal(err)
	}
	return feed
}

func feedBytes(t *testing.T, feed map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func findScene(t *testing.T, feed map[string]any, id string) map[string]any {
	t.Helper()
	for _, s := range feed["scenes"].([]any) {
		if m := s.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("scene %q not in fixture", id)
	return nil
}

func findWidget(t *testing.T, scene map[string]any, id string) map[string]any {
	t.Helper()
	for _, w := range scene["widgets"].([]any) {
		if m := w.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("widget %q not in scene", id)
	return nil
}

func sceneStart(scene map[string]any) map[string]any {
	return scene["start"].(map[string]any)
}

func widgetProps(widget map[string]any) map[string]any {
	return widget["props"].(map[string]any)
}

func TestParseFixture(t *testing.T) {
	feed, err := ParseScenesFeed(loadFeedFixture(t))
	if err != nil {
		t.Fatalf("fixture rejected: %v", err)
	}
	if feed.Version != 1 {
		t.Errorf("version = %d, want 1", feed.Version)
	}
	if feed.Revision != 12 {
		t.Errorf("revision = %d, want 12", feed.Revision)
	}
	if feed.Grid.Cols != 12 || feed.Grid.Rows != 24 {
		t.Errorf("grid = %+v, want 12x24", feed.Grid)
	}
	if len(feed.Scenes) != 3 {
		t.Fatalf("scenes = %d, want 3", len(feed.Scenes))
	}
	ids := []string{feed.Scenes[0].ID, feed.Scenes[1].ID, feed.Scenes[2].ID}
	if strings.Join(ids, ",") != "morning,s-k3f9q2,night" {
		t.Errorf("scene order = %v", ids)
	}
	if feed.Scenes[2].Widgets != nil {
		t.Errorf("night should have no widgets, got %+v", feed.Scenes[2].Widgets)
	}
	if feed.Preview == nil {
		t.Fatal("preview missing")
	}
	if feed.Preview.SceneID != "s-k3f9q2" || feed.Preview.Until != 1791230400000 {
		t.Errorf("preview = %+v", feed.Preview)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := ParseScenesFeed(nil); err == nil {
		t.Error("expected an error for an empty body")
	}
}

func TestParseOversize(t *testing.T) {
	data := bytes.Repeat([]byte("x"), (256<<10)+1)
	_, err := ParseScenesFeed(data)
	if err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Errorf("expected a 256 KiB error, got %v", err)
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	feed := feedMap(t)
	feed["future"] = "tolerant"
	findWidget(t, findScene(t, feed, "morning"), "w1")["future"] = 1
	if _, err := ParseScenesFeed(feedBytes(t, feed)); err != nil {
		t.Errorf("unknown fields should be ignored: %v", err)
	}
}

func TestParseInvalid(t *testing.T) {
	morningWidget := func(t *testing.T, feed map[string]any, id string) map[string]any {
		return findWidget(t, findScene(t, feed, "morning"), id)
	}
	cases := []struct {
		name    string
		mutate  func(t *testing.T, feed map[string]any)
		wantErr string
	}{
		{"version", func(t *testing.T, feed map[string]any) { feed["version"] = 2 }, "version"},
		{"grid", func(t *testing.T, feed map[string]any) { feed["grid"].(map[string]any)["cols"] = 10 }, "grid"},
		{"unknownKind", func(t *testing.T, feed map[string]any) {
			findScene(t, feed, "morning")["kind"] = "dawn"
		}, "kind"},
		{"nightWithWidgets", func(t *testing.T, feed map[string]any) {
			morning := findScene(t, feed, "morning")
			findScene(t, feed, "night")["widgets"] = morning["widgets"]
		}, "night"},
		{"noNight", func(t *testing.T, feed map[string]any) {
			kept := []any{}
			for _, s := range feed["scenes"].([]any) {
				if s.(map[string]any)["id"] != "night" {
					kept = append(kept, s)
				}
			}
			feed["scenes"] = kept
		}, "exactly one"},
		{"secondNight", func(t *testing.T, feed map[string]any) {
			findScene(t, feed, "s-k3f9q2")["kind"] = "night"
		}, "exactly one"},
		{"tooManyLayouts", func(t *testing.T, feed map[string]any) {
			base := findScene(t, feed, "s-k3f9q2")
			scenes := feed["scenes"].([]any)
			for i := range 7 {
				raw, err := json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
				var dup map[string]any
				if err := json.Unmarshal(raw, &dup); err != nil {
					t.Fatal(err)
				}
				dup["id"] = fmt.Sprintf("s-extra-%d", i)
				dup["name"] = fmt.Sprintf("Extra %d", i)
				dup["start"] = map[string]any{"schoolDay": fmt.Sprintf("%02d:00", i), "weekend": nil}
				scenes = append(scenes, dup)
			}
			feed["scenes"] = scenes
		}, "at most 8"},
		{"dupName", func(t *testing.T, feed map[string]any) {
			findScene(t, feed, "night")["name"] = "Morgen"
		}, "duplicate scene name"},
		{"dupID", func(t *testing.T, feed map[string]any) {
			findScene(t, feed, "night")["id"] = "morning"
		}, "duplicate scene id"},
		{"longName", func(t *testing.T, feed map[string]any) {
			findScene(t, feed, "morning")["name"] = strings.Repeat("x", 31)
		}, "1-30"},
		{"badTime", func(t *testing.T, feed map[string]any) {
			sceneStart(findScene(t, feed, "morning"))["schoolDay"] = "6:45"
		}, "HH:MM"},
		{"neverRuns", func(t *testing.T, feed map[string]any) {
			start := sceneStart(findScene(t, feed, "morning"))
			start["schoolDay"] = nil
			start["weekend"] = nil
		}, "never runs"},
		{"dupStart", func(t *testing.T, feed map[string]any) {
			sceneStart(findScene(t, feed, "s-k3f9q2"))["schoolDay"] = "06:45"
		}, "already taken"},
		{"weekendUncovered", func(t *testing.T, feed map[string]any) {
			sceneStart(findScene(t, feed, "morning"))["weekend"] = nil
		}, "Sa/So"},
		{"outOfBounds", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w1")["w"] = 13
		}, "exceeds"},
		{"overlap", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w3")["x"] = 0
		}, "overlap"},
		{"dupWidgetID", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w3")["id"] = "w2"
		}, "duplicate widget id"},
		{"dupWidgetType", func(t *testing.T, feed map[string]any) {
			w3 := morningWidget(t, feed, "w3")
			w3["type"] = "clock"
			delete(w3, "props")
		}, "twice"},
		{"unknownType", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w1")["type"] = "toaster"
		}, "unknown widget type"},
		{"belowMinSize", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w1")["w"] = 3
		}, "at least"},
		{"briefingKind", func(t *testing.T, feed map[string]any) {
			delete(widgetProps(morningWidget(t, feed, "w2")), "kind")
		}, "kind"},
		{"schoolMealDay", func(t *testing.T, feed map[string]any) {
			delete(widgetProps(morningWidget(t, feed, "w3")), "mealDay")
		}, "mealDay"},
		{"foreignProps", func(t *testing.T, feed map[string]any) {
			widgetProps(morningWidget(t, feed, "w1"))["kind"] = "today"
		}, "unknown setting"},
		{"fallbackUnknown", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w2")["fallback"] = "toaster"
		}, "fallback"},
		{"fallbackSelf", func(t *testing.T, feed map[string]any) {
			morningWidget(t, feed, "w2")["fallback"] = "briefing"
		}, "own fallback"},
		{"previewInvalidWidget", func(t *testing.T, feed map[string]any) {
			feed["preview"].(map[string]any)["widgets"] = []any{
				map[string]any{"id": "p1", "type": "clock", "x": 0, "y": 0, "w": 13, "h": 4},
			}
		}, "exceeds"},
		{"previewUnknownScene", func(t *testing.T, feed map[string]any) {
			feed["preview"].(map[string]any)["sceneId"] = "nope"
		}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feed := feedMap(t)
			tc.mutate(t, feed)
			_, err := ParseScenesFeed(feedBytes(t, feed))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err, tc.wantErr)
			}
		})
	}
}
