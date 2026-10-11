package scenes

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"familydash/internal/familyapp"
	"familydash/internal/scene"
)

// ScenesPollInterval is the default feed poll cadence.
const ScenesPollInterval = 15 * time.Second

// Snapshot is the last accepted feed with its fetch time.
type Snapshot struct {
	Feed      Feed
	FetchedAt time.Time
}

// Service polls GET /dashboard/scenes and keeps the last-good snapshot.
// Only a fully validated response replaces the state; every failure keeps
// serving the previous snapshot. Without any snapshot the wall falls back
// to its SCENE_* schedule and built-in layouts.
type Service struct {
	client     *familyapp.Client
	token      string
	cachePath  string
	backend    string
	provider   *scene.Provider
	swappedRev int
	mu         sync.RWMutex
	refreshMu  sync.Mutex
	snap       *Snapshot
	lastErr    error
	configErr  string
}

// NewService builds the scenes source. A missing backend URL or token does
// not fail; the snapshot stays unavailable until a poll succeeds. A strictly
// validated persistent cache is loaded when it belongs to this backend.
// Accepted feeds are pushed into provider (nil = no sync).
func NewService(client *familyapp.Client, token, cachePath string, provider *scene.Provider) *Service {
	s := &Service{client: client, token: token, cachePath: cachePath, provider: provider, swappedRev: -1}
	if client != nil {
		s.backend = normalizeScenesBackend(client.BaseURL)
	}
	switch {
	case client == nil || strings.TrimSpace(client.BaseURL) == "":
		s.configErr = "FAMILY_APP_SITE_URL fehlt – Szenen-Feed nicht verfügbar"
	case strings.TrimSpace(token) == "":
		s.configErr = "FAMILY_APP_SCENES_TOKEN fehlt – Szenen-Feed nicht verfügbar"
	}
	if s.configErr == "" && cachePath != "" {
		if snap, err := loadScenesCache(cachePath, s.backend); err != nil {
			// A missing file is the normal first start, not a warning.
			if !os.IsNotExist(err) {
				s.lastErr = err
				slog.Warn("scenes cache rejected", "path", cachePath, "err", err)
			}
		} else {
			s.snap = snap
			s.maybeSwap(snap.Feed)
		}
	}
	return s
}

// Refresh fetches the feed once. Only a fully validated response replaces
// the last-good snapshot (and the cache file); every failure keeps the
// previous state.
func (s *Service) Refresh(ctx context.Context) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.configErr != "" {
		s.mu.Lock()
		s.lastErr = errors.New(s.configErr)
		s.mu.Unlock()
		return
	}
	data, err := s.client.GetScenesFeed(ctx, s.token)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		slog.Warn("scenes refresh failed", "err", err)
		return
	}
	feed, err := ParseScenesFeed(data)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		slog.Warn("scenes refresh rejected", "err", err)
		return
	}
	raw := append([]byte(nil), data...)
	fetchedAt := time.Now().Truncate(time.Millisecond)
	s.mu.Lock()
	s.snap = &Snapshot{Feed: feed, FetchedAt: fetchedAt}
	s.lastErr = nil
	s.mu.Unlock()
	s.maybeSwap(feed)
	if s.cachePath != "" {
		if err := saveScenesCache(s.cachePath, s.backend, fetchedAt, raw); err != nil {
			// Warning only: the accepted RAM data stays.
			slog.Warn("scenes cache not saved", "path", s.cachePath, "err", err)
		}
	}
}

// maybeSwap pushes newly accepted feeds into the provider schedule. The
// revision guard keeps swaps (and force warnings) to actual feed changes;
// failures never touch the provider, so the last schedule survives outages.
func (s *Service) maybeSwap(feed Feed) {
	if s.provider == nil || feed.Revision == s.swappedRev {
		return
	}
	sched, err := feed.Schedule()
	if err != nil {
		slog.Warn("scenes schedule rejected", "err", err)
		return
	}
	s.provider.SetSchedule(sched)
	s.swappedRev = feed.Revision
}

// Run polls every interval until ctx ends.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = ScenesPollInterval
	}
	s.Refresh(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Refresh(ctx)
		}
	}
}

// Snapshot returns a copy of the last-good snapshot, or an error when
// nothing was ever accepted (callers fall back to SCENE_* then).
func (s *Service) Snapshot() (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snap != nil {
		return cloneSnapshot(*s.snap), nil
	}
	switch {
	case s.configErr != "":
		return Snapshot{}, errors.New(s.configErr)
	case s.lastErr != nil:
		return Snapshot{}, s.lastErr
	default:
		return Snapshot{}, errors.New("Szenen-Feed noch nicht geladen")
	}
}

func cloneStart(start Start) Start {
	out := start
	if start.SchoolDay != nil {
		value := *start.SchoolDay
		out.SchoolDay = &value
	}
	if start.Weekend != nil {
		value := *start.Weekend
		out.Weekend = &value
	}
	return out
}

func cloneWidgets(in []Widget) []Widget {
	if in == nil {
		return nil
	}
	out := make([]Widget, len(in))
	for i, widget := range in {
		dup := widget
		if widget.Props != nil {
			props := *widget.Props
			if widget.Props.Wide != nil {
				wide := *widget.Props.Wide
				props.Wide = &wide
			}
			if widget.Props.Hints != nil {
				hints := *widget.Props.Hints
				props.Hints = &hints
			}
			dup.Props = &props
		}
		out[i] = dup
	}
	return out
}

func cloneSnapshot(snap Snapshot) Snapshot {
	out := snap
	out.Feed.Scenes = make([]Scene, len(snap.Feed.Scenes))
	for i, scene := range snap.Feed.Scenes {
		dup := scene
		dup.Start = cloneStart(scene.Start)
		dup.Widgets = cloneWidgets(scene.Widgets)
		out.Feed.Scenes[i] = dup
	}
	if snap.Feed.Preview != nil {
		preview := *snap.Feed.Preview
		preview.Widgets = cloneWidgets(snap.Feed.Preview.Widgets)
		out.Feed.Preview = &preview
	}
	return out
}
