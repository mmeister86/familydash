// Package scenes pulls the wall schedule and tile layouts from the family
// app (GET /dashboard/scenes, see docs/FAMILY_APP.md) with a persistent
// last-good cache. Anything the backend serves is strictly validated; a
// rejected feed keeps the previous snapshot, and without any snapshot the
// wall falls back to its SCENE_* schedule and built-in layouts.
package scenes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"familydash/internal/layout"
)

// maxFeedBytes bounds the scenes feed; 8 scenes with 7 widgets fit easily.
// Must match the fetch cap in familyapp (maxScenesFeedBytes).
const maxFeedBytes = 256 << 10

const feedVersion = 1

// Start holds HH:MM start times; nil means the scene does not run there.
type Start struct {
	SchoolDay *string `json:"schoolDay"`
	Weekend   *string `json:"weekend"`
}

// WidgetProps carries the per-type settings; only the keys valid for the
// widget type may be present.
type WidgetProps struct {
	Wide    *bool  `json:"wide,omitempty"`
	Hints   *bool  `json:"hints,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Mode    string `json:"mode,omitempty"`
	MealDay string `json:"mealDay,omitempty"`
}

// Widget is one tile on the 12x24 scene grid.
type Widget struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	X        int          `json:"x"`
	Y        int          `json:"y"`
	W        int          `json:"w"`
	H        int          `json:"h"`
	Props    *WidgetProps `json:"props,omitempty"`
	Fallback string       `json:"fallback,omitempty"`
}

// Scene is one wall scene; night scenes carry no widgets.
type Scene struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Start   Start    `json:"start"`
	Widgets []Widget `json:"widgets,omitempty"`
}

// Preview forces one scene onto the wall until Until (unix millis).
type Preview struct {
	SceneID string   `json:"sceneId"`
	Until   int64    `json:"until"`
	Widgets []Widget `json:"widgets"`
}

// Grid is the fixed portrait tile grid both repos share.
type Grid struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// Feed is the parsed GET /dashboard/scenes response.
type Feed struct {
	Version  int      `json:"version"`
	Revision int      `json:"revision"`
	Grid     Grid     `json:"grid"`
	Scenes   []Scene  `json:"scenes"`
	Preview  *Preview `json:"preview,omitempty"`
}

// Raw mirrors keep widget props undecoded so unknown prop keys can be
// rejected with a precise error; the envelope itself tolerates additive
// fields like the calendar feed does.
type widgetJSON struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	X        int             `json:"x"`
	Y        int             `json:"y"`
	W        int             `json:"w"`
	H        int             `json:"h"`
	Props    json.RawMessage `json:"props"`
	Fallback string          `json:"fallback"`
}

type sceneJSON struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Kind    string       `json:"kind"`
	Start   Start        `json:"start"`
	Widgets []widgetJSON `json:"widgets"`
}

type previewJSON struct {
	SceneID string       `json:"sceneId"`
	Until   int64        `json:"until"`
	Widgets []widgetJSON `json:"widgets"`
}

type feedJSON struct {
	Version  int          `json:"version"`
	Revision int          `json:"revision"`
	Grid     Grid         `json:"grid"`
	Scenes   []sceneJSON  `json:"scenes"`
	Preview  *previewJSON `json:"preview"`
}

func scenesErr(format string, a ...any) error {
	return fmt.Errorf("scenes feed: "+format, a...)
}

var timePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ParseScenesFeed strictly validates a v1 scenes feed body with the same
// rules as the Convex backend. Unknown additive fields are ignored.
func ParseScenesFeed(data []byte) (Feed, error) {
	if len(data) == 0 {
		return Feed{}, scenesErr("empty body")
	}
	if len(data) > maxFeedBytes {
		return Feed{}, scenesErr("body exceeds 256 KiB (%d bytes)", len(data))
	}
	var raw feedJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return Feed{}, scenesErr("invalid JSON: %v", err)
	}
	return validateFeed(raw)
}

func validateFeed(raw feedJSON) (Feed, error) {
	if raw.Version != feedVersion {
		return Feed{}, scenesErr("version %d, want %d", raw.Version, feedVersion)
	}
	if raw.Grid.Cols != layout.GridCols || raw.Grid.Rows != layout.GridRows {
		return Feed{}, scenesErr("grid %dx%d, want %dx%d",
			raw.Grid.Cols, raw.Grid.Rows, layout.GridCols, layout.GridRows)
	}
	feed := Feed{Version: raw.Version, Revision: raw.Revision, Grid: raw.Grid}
	names := map[string]bool{}
	layouts := 0
	nights := 0
	for _, rs := range raw.Scenes {
		if strings.TrimSpace(rs.ID) == "" {
			return Feed{}, scenesErr("every scene needs an id")
		}
		if rs.Kind != "layout" && rs.Kind != "night" {
			return Feed{}, scenesErr("scene %q has unknown kind %q", rs.ID, rs.Kind)
		}
		name := strings.TrimSpace(rs.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 30 {
			return Feed{}, scenesErr("scene name must be 1-30 characters")
		}
		if lower := strings.ToLower(name); names[lower] {
			return Feed{}, scenesErr("duplicate scene name %q", name)
		} else {
			names[lower] = true
		}
		if err := validateStart(rs.ID, rs.Start); err != nil {
			return Feed{}, err
		}
		if rs.Kind == "night" {
			nights++
			if len(rs.Widgets) > 0 {
				return Feed{}, scenesErr("night scene must not have widgets")
			}
			feed.Scenes = append(feed.Scenes, Scene{ID: rs.ID, Name: name, Kind: rs.Kind, Start: rs.Start})
			continue
		}
		layouts++
		widgets, err := validateWidgets(rs.Widgets)
		if err != nil {
			return Feed{}, err
		}
		feed.Scenes = append(feed.Scenes, Scene{ID: rs.ID, Name: name, Kind: rs.Kind, Start: rs.Start, Widgets: widgets})
	}
	if nights != 1 {
		return Feed{}, scenesErr("want exactly one night scene, got %d", nights)
	}
	if layouts > 8 {
		return Feed{}, scenesErr("at most 8 layout scenes, got %d", layouts)
	}
	if err := validateSchedule(feed.Scenes); err != nil {
		return Feed{}, err
	}
	if raw.Preview != nil {
		if strings.TrimSpace(raw.Preview.SceneID) == "" {
			return Feed{}, scenesErr("preview needs a sceneId")
		}
		known := false
		for _, scene := range feed.Scenes {
			if scene.ID == raw.Preview.SceneID {
				known = true
				break
			}
		}
		if !known {
			return Feed{}, scenesErr("preview scene %q is unknown", raw.Preview.SceneID)
		}
		widgets, err := validateWidgets(raw.Preview.Widgets)
		if err != nil {
			return Feed{}, err
		}
		feed.Preview = &Preview{SceneID: raw.Preview.SceneID, Until: raw.Preview.Until, Widgets: widgets}
	}
	return feed, nil
}

func validateStart(sceneID string, start Start) error {
	for _, value := range []*string{start.SchoolDay, start.Weekend} {
		if value != nil && !timePattern.MatchString(*value) {
			return scenesErr("%q is not a valid time (want HH:MM)", *value)
		}
	}
	if start.SchoolDay == nil && start.Weekend == nil {
		return scenesErr("scene %q never runs (schoolDay and weekend are null)", sceneID)
	}
	return nil
}

var allowedProps = map[string][]string{
	"clock":    {"wide", "hints"},
	"briefing": {"kind"},
	"school":   {"mode", "mealDay"},
}

func validateProps(widgetType string, raw json.RawMessage) (*WidgetProps, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, scenesErr("widget %q has invalid props: %v", widgetType, err)
	}
	allowed := map[string]bool{}
	for _, key := range allowedProps[widgetType] {
		allowed[key] = true
	}
	for key := range keys {
		if !allowed[key] {
			return nil, scenesErr("widget %q has unknown setting %q", widgetType, key)
		}
	}
	var props WidgetProps
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, scenesErr("widget %q has invalid props: %v", widgetType, err)
	}
	switch widgetType {
	case "briefing":
		if props.Kind != "today" && props.Kind != "tomorrow" {
			return nil, scenesErr("briefing widget needs kind \"today\" or \"tomorrow\"")
		}
	case "school":
		if props.Mode != "full" && props.Mode != "plan" && props.Mode != "plan-meal" {
			return nil, scenesErr("school widget needs mode \"full\", \"plan\" or \"plan-meal\"")
		}
		if props.MealDay != "today" && props.MealDay != "tomorrow" {
			return nil, scenesErr("school widget needs mealDay \"today\" or \"tomorrow\"")
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	return &props, nil
}

func validateWidgets(in []widgetJSON) ([]Widget, error) {
	out := make([]Widget, 0, len(in))
	ids := map[string]bool{}
	types := map[string]bool{}
	for _, raw := range in {
		if strings.TrimSpace(raw.ID) == "" {
			return nil, scenesErr("every widget needs an id")
		}
		def, ok := layout.Lookup(raw.Type)
		if !ok {
			return nil, scenesErr("unknown widget type %q", raw.Type)
		}
		if raw.X < 0 || raw.Y < 0 || raw.W < 1 || raw.H < 1 ||
			raw.X+raw.W > layout.GridCols || raw.Y+raw.H > layout.GridRows {
			return nil, scenesErr("widget %q exceeds the %dx%d grid", raw.ID, layout.GridCols, layout.GridRows)
		}
		if raw.W < def.MinW || raw.H < def.MinH {
			return nil, scenesErr("widget %q needs at least %dx%d tiles", raw.ID, def.MinW, def.MinH)
		}
		props, err := validateProps(raw.Type, raw.Props)
		if err != nil {
			return nil, err
		}
		if raw.Fallback != "" {
			if _, ok := layout.Lookup(raw.Fallback); !ok {
				return nil, scenesErr("unknown fallback type %q", raw.Fallback)
			}
			if raw.Fallback == raw.Type {
				return nil, scenesErr("widget cannot be its own fallback")
			}
		}
		if ids[raw.ID] {
			return nil, scenesErr("duplicate widget id %q", raw.ID)
		}
		ids[raw.ID] = true
		if types[raw.Type] {
			return nil, scenesErr("widget %q appears twice (each type once per scene)", raw.Type)
		}
		types[raw.Type] = true
		out = append(out, Widget{ID: raw.ID, Type: raw.Type, X: raw.X, Y: raw.Y, W: raw.W, H: raw.H, Props: props, Fallback: raw.Fallback})
	}
	for i := range out {
		for _, other := range out[i+1:] {
			a, b := out[i], other
			if a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H {
				return nil, scenesErr("widgets %q and %q overlap", a.ID, b.ID)
			}
		}
	}
	return out, nil
}

func validateSchedule(scenes []Scene) error {
	days := []struct {
		label string
		at    func(Start) *string
	}{
		{"Mo-Fr", func(s Start) *string { return s.SchoolDay }},
		{"Sa/So", func(s Start) *string { return s.Weekend }},
	}
	for _, day := range days {
		seen := map[string]string{}
		for _, scene := range scenes {
			start := day.at(scene.Start)
			if start == nil {
				continue
			}
			if other, dup := seen[*start]; dup {
				return scenesErr("start %s is already taken by scene %q (%s)", *start, other, day.label)
			}
			seen[*start] = scene.Name
		}
		covered := false
		for _, scene := range scenes {
			if scene.Kind == "layout" && day.at(scene.Start) != nil {
				covered = true
				break
			}
		}
		if !covered {
			return scenesErr("no layout scene runs on %s", day.label)
		}
	}
	return nil
}
