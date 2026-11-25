package server

import (
	"log/slog"
	"net/http"
	"time"

	"familydash/internal/briefing"
	"familydash/internal/scene"
	"familydash/internal/scenes"
)

// previewDuration is the fixed Convex preview window (sendPreview sets
// until = now + 2m); the feed carries no preview start, so Since derives.
const previewDuration = 2 * time.Minute

// sceneLayout is the active scene's tile grid with its feed revision.
type sceneLayout struct {
	Revision int             `json:"revision"`
	Widgets  []scenes.Widget `json:"widgets"`
}

// scenePayload is the wall's scene state: what runs plus its tile grid.
type scenePayload struct {
	Scene  scene.Current `json:"scene"`
	Layout sceneLayout   `json:"layout"`
}

// resolveScene computes the current wall scene: a ?scene= override wins over
// everything (local testing), then an active preview, else the provider
// schedule; widgets come from the feed (or the built-in defaults when the
// feed never loaded).
func (s *Server) resolveScene(now time.Time, override string) scenePayload {
	feed, revision := scenes.DefaultFeed(), 0
	fromFeed := false
	if s.scenes != nil {
		if snap, err := s.scenes.Snapshot(); err == nil {
			feed, revision, fromFeed = snap.Feed, snap.Feed.Revision, true
		}
	}
	if override != "" {
		for _, candidate := range feed.Scenes {
			if candidate.ID == override {
				return scenePayload{
					Scene: scene.Current{
						ID: candidate.ID, Name: candidate.Name, Kind: candidate.Kind,
						SchoolDay: scene.IsSchoolDay(now), Forced: true,
						Palette: paletteFor(candidate, now),
					},
					Layout: sceneLayout{Revision: revision, Widgets: candidate.Widgets},
				}
			}
		}
		slog.Warn("dashboard: unknown scene, ignoring", "scene", override)
	}
	if preview := feed.Preview; preview != nil && preview.Until > now.UnixMilli() {
		until := time.UnixMilli(preview.Until).In(s.cfg.Location)
		name, kind, palette := preview.SceneID, "layout", ""
		for _, candidate := range feed.Scenes {
			if candidate.ID == preview.SceneID {
				name, kind, palette = candidate.Name, candidate.Kind, paletteFor(candidate, now)
			}
		}
		return scenePayload{
			Scene: scene.Current{
				ID: preview.SceneID, Name: name, Kind: kind,
				Since: until.Add(-previewDuration), Until: until,
				SchoolDay: scene.IsSchoolDay(now), Preview: true, Palette: palette,
			},
			Layout: sceneLayout{Revision: revision, Widgets: preview.Widgets},
		}
	}
	payload := func(cur scene.Current) scenePayload {
		cur.Palette = scenePalette(feed, cur, now)
		return scenePayload{
			Scene:  cur,
			Layout: sceneLayout{Revision: revision, Widgets: widgetsFor(feed, cur.ID)},
		}
	}
	if !fromFeed {
		// No feed yet: SCENE_* times (provider) with built-in layouts.
		return payload(s.sceneProvider.At(now, s.cfg.Location))
	}
	// The schedule comes from the same snapshot as the widgets, so a request
	// can never mix feed versions; the configured force still applies.
	sched, err := feed.Schedule()
	if err != nil {
		slog.Warn("dashboard: feed schedule rejected, using provider", "err", err)
		return payload(s.sceneProvider.At(now, s.cfg.Location))
	}
	sched.Force = s.sceneProvider.Force()
	return payload(sched.At(now, s.cfg.Location))
}

// paletteFor is the palette a layout scene shows: its explicit one, else the
// default for the hour its schedule starts it today. Night scenes have none.
func paletteFor(sc scenes.Scene, now time.Time) string {
	if sc.Kind == "night" {
		return ""
	}
	if sc.Palette != "" {
		return sc.Palette
	}
	start := sc.Start.SchoolDay
	if !scene.IsSchoolDay(now) {
		start = sc.Start.Weekend
	}
	if start == nil {
		if start = sc.Start.SchoolDay; start == nil {
			start = sc.Start.Weekend
		}
	}
	if start != nil {
		if minute, err := scene.ParseClock(*start); err == nil && minute >= 0 {
			return scene.DefaultPalette(time.Date(2000, 1, 1, minute/60, minute%60, 0, 0, time.UTC))
		}
	}
	return "paper"
}

// scenePalette resolves the palette for a scheduled scene; an id the feed
// does not know (stale provider schedule) falls back to the start time.
func scenePalette(feed scenes.Feed, cur scene.Current, now time.Time) string {
	for _, candidate := range feed.Scenes {
		if candidate.ID == cur.ID {
			return paletteFor(candidate, now)
		}
	}
	if cur.Kind == "night" {
		return ""
	}
	return scene.DefaultPalette(cur.Since)
}

func widgetsFor(feed scenes.Feed, id string) []scenes.Widget {
	for _, candidate := range feed.Scenes {
		if candidate.ID == id {
			return candidate.Widgets
		}
	}
	return []scenes.Widget{}
}

// previewBriefing serves the existing card matching the preview layout's
// briefing widget, without triggering generation; layouts without one (or
// empty slots) fall back to the scheduled card.
func (s *Server) previewBriefing(widgets []scenes.Widget) *briefing.Briefing {
	for _, w := range widgets {
		if w.Type != "briefing" || w.Props == nil {
			continue
		}
		switch w.Props.Kind {
		case "today":
			return s.briefing.Card(briefing.Morning)
		case "tomorrow":
			return s.briefing.Card(briefing.Evening)
		}
	}
	return s.briefing.Snapshot()
}

// handleScene serves the light scene state for the frontend's fast poll.
func (s *Server) handleScene(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.resolveScene(s.clock(), r.URL.Query().Get("scene")))
}
