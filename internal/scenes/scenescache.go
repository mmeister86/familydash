package scenes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const scenesCacheVersion = 1
const scenesCacheScope = "wall-scenes"

// scenesCacheEnvelope persists the last accepted feed with its fetch time,
// so a restart never makes data look fresher than it is. Only the raw feed,
// the fetch time and the nonsecret scope are stored — never tokens.
type scenesCacheEnvelope struct {
	Version     int             `json:"version"`
	Backend     string          `json:"backend"`
	Scope       string          `json:"scope"`
	FetchedAtMs int64           `json:"fetchedAtMs"`
	Feed        json.RawMessage `json:"feed"`
}

// normalizeScenesBackend canonicalizes the backend site URL for cache
// binding: surrounding whitespace and a trailing slash are dropped, scheme
// and host are lowercased. (Mirrors the calendar cache; each feed owns its
// cache file, so the helper lives here.)
func normalizeScenesBackend(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimRight(u, "/")
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		p.Scheme = strings.ToLower(p.Scheme)
		p.Host = strings.ToLower(p.Host)
		return strings.TrimRight(p.String(), "/")
	}
	return strings.ToLower(u)
}

// saveScenesCache atomically persists a validated feed: temp file in the
// same directory, fsync, owner-only permissions, rename. A failure is
// reported to the caller (warn, keep RAM) and never leaves a half file.
func saveScenesCache(path, backend string, fetchedAt time.Time, feed []byte) error {
	env := scenesCacheEnvelope{
		Version:     scenesCacheVersion,
		Backend:     normalizeScenesBackend(backend),
		Scope:       scenesCacheScope,
		FetchedAtMs: fetchedAt.UnixMilli(),
		Feed:        append(json.RawMessage(nil), feed...),
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".scenes-cache-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncScenesCacheDir(path)
}

func syncScenesCacheDir(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil // best effort: the rename already succeeded
	}
	defer dir.Close()
	return dir.Sync()
}

// loadScenesCache reads a persisted feed and validates it exactly like a
// live response. A version/scope/backend mismatch or an invalid feed is an
// error.
func loadScenesCache(path, backend string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFeedBytes {
		return nil, fmt.Errorf("scenes cache: file exceeds 256 KiB")
	}
	var env scenesCacheEnvelope
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&env); err != nil {
		return nil, fmt.Errorf("scenes cache: %v", err)
	}
	if env.Version != scenesCacheVersion {
		return nil, fmt.Errorf("scenes cache: version %d, want %d", env.Version, scenesCacheVersion)
	}
	if env.Scope != scenesCacheScope {
		return nil, fmt.Errorf("scenes cache: scope %q, want %q", env.Scope, scenesCacheScope)
	}
	if normalizeScenesBackend(env.Backend) != normalizeScenesBackend(backend) {
		return nil, fmt.Errorf("scenes cache: backend %q is not this backend", env.Backend)
	}
	if len(env.Feed) == 0 {
		return nil, fmt.Errorf("scenes cache: empty feed")
	}
	feed, err := ParseScenesFeed(env.Feed)
	if err != nil {
		return nil, fmt.Errorf("scenes cache: %v", err)
	}
	return &Snapshot{Feed: feed, FetchedAt: time.UnixMilli(env.FetchedAtMs)}, nil
}
