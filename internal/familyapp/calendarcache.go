package familyapp

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

	"familydash/internal/calendar"
)

// calendarCacheEnvelope persists the last accepted feed. Only the raw feed,
// the normalized backend URL and the nonsecret scope are stored — never
// tokens or ICS URLs (the feed itself carries none). The feed is revalidated
// as strictly as a live response on load, so an outdated or foreign cache
// can never surface as current data.
type calendarCacheEnvelope struct {
	Version int             `json:"version"`
	Backend string          `json:"backend"`
	Scope   string          `json:"scope"`
	Feed    json.RawMessage `json:"feed"`
}

// normalizeCalendarBackend canonicalizes the backend site URL for cache
// binding: surrounding whitespace and a trailing slash are dropped, scheme
// and host are lowercased.
func normalizeCalendarBackend(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimRight(u, "/")
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		p.Scheme = strings.ToLower(p.Scheme)
		p.Host = strings.ToLower(p.Host)
		return strings.TrimRight(p.String(), "/")
	}
	return strings.ToLower(u)
}

// saveCalendarCache atomically persists a validated feed: temp file in the
// same directory, fsync, owner-only permissions, rename. A failure is
// reported to the caller (warn, keep RAM) and never leaves a half file.
func saveCalendarCache(path, backend string, feed []byte) error {
	env := calendarCacheEnvelope{
		Version: calendarCacheVersion,
		Backend: normalizeCalendarBackend(backend),
		Scope:   calendarCacheScope,
		Feed:    append(json.RawMessage(nil), feed...),
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".calendar-cache-*.tmp")
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
	return syncCalendarCacheDir(path)
}

func syncCalendarCacheDir(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil // best effort: the rename already succeeded
	}
	defer dir.Close()
	return dir.Sync()
}

// loadCalendarCache reads a persisted feed and validates it exactly like a
// live response. A version/scope/backend mismatch or an invalid feed is an
// error; per-source success times travel with the feed, so a restart never
// makes data look fresher than its providers' last success.
func loadCalendarCache(path, backend string, loc *time.Location) (*calendar.Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCalendarFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCalendarFeedBytes {
		return nil, fmt.Errorf("calendar cache: file exceeds 4 MiB")
	}
	var env calendarCacheEnvelope
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&env); err != nil {
		return nil, fmt.Errorf("calendar cache: %v", err)
	}
	if env.Version != calendarCacheVersion {
		return nil, fmt.Errorf("calendar cache: version %d, want %d", env.Version, calendarCacheVersion)
	}
	if env.Scope != calendarCacheScope {
		return nil, fmt.Errorf("calendar cache: scope %q, want %q", env.Scope, calendarCacheScope)
	}
	if normalizeCalendarBackend(env.Backend) != normalizeCalendarBackend(backend) {
		return nil, fmt.Errorf("calendar cache: backend %q is not this backend", env.Backend)
	}
	if len(env.Feed) == 0 {
		return nil, fmt.Errorf("calendar cache: empty feed")
	}
	snap, err := ParseCalendarFeed(env.Feed, loc)
	if err != nil {
		return nil, fmt.Errorf("calendar cache: %v", err)
	}
	return &snap, nil
}
