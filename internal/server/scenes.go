package server

import (
	"log/slog"
	"net/http"
	"time"

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
	if s.scenes != nil {
		if snap, err := s.scenes.Snapshot(); err == nil {
			feed, revision = snap.Feed, snap.Feed.Revision
		}
	}
	if override != "" {
		for _, candidate := range feed.Scenes {
			if candidate.ID == override {
				return scenePayload{
					Scene: scene.Current{
						ID: candidate.ID, Name: candidate.Name, Kind: candidate.Kind,
						SchoolDay: scene.IsSchoolDay(now), Forced: true,
					},
					Layout: sceneLayout{Revision: revision, Widgets: candidate.Widgets},
				}
			}
		}
		slog.Warn("dashboard: unknown scene, ignoring", "scene", override)
	}
	if preview := feed.Preview; preview != nil && preview.Until > now.UnixMilli() {
		until := time.UnixMilli(preview.Until).In(s.cfg.Location)
		name, kind := preview.SceneID, "layout"
		for _, candidate := range feed.Scenes {
			if candidate.ID == preview.SceneID {
				name, kind = candidate.Name, candidate.Kind
			}
		}
		return scenePayload{
			Scene: scene.Current{
				ID: preview.SceneID, Name: name, Kind: kind,
				Since: until.Add(-previewDuration), Until: until,
				SchoolDay: scene.IsSchoolDay(now), Preview: true,
			},
			Layout: sceneLayout{Revision: revision, Widgets: preview.Widgets},
		}
	}
	cur := s.sceneProvider.At(now, s.cfg.Location)
	return scenePayload{
		Scene:  cur,
		Layout: sceneLayout{Revision: revision, Widgets: widgetsFor(feed, cur.ID)},
	}
}

func widgetsFor(feed scenes.Feed, id string) []scenes.Widget {
	for _, candidate := range feed.Scenes {
		if candidate.ID == id {
			return candidate.Widgets
		}
	}
	return []scenes.Widget{}
}

// handleScene serves the light scene state for the frontend's fast poll.
func (s *Server) handleScene(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.resolveScene(time.Now().In(s.cfg.Location), r.URL.Query().Get("scene")))
}
