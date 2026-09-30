// Package photos serves a folder of pictures for the slideshow on the wall
// display. The folder is rescanned regularly, so photos dropped in via SMB
// show up without a restart. It may not exist yet – the slideshow simply
// stays hidden until it does.
package photos

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// Formats every browser on the Pi can show. HEIC (iPhone default) is not
// among them – export as JPEG ("Most Compatible" in the iPhone camera settings).
var exts = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".webp": "image/webp", ".gif": "image/gif", ".avif": "image/avif",
}

const maxPhotos = 2000

type Photo struct {
	Path string `json:"path"` // relative to the folder, "/" separated
	V    int64  `json:"v"`    // mtime, busts the browser cache when a file is replaced
}

type Snapshot struct {
	Items    []Photo `json:"items"`
	Interval int     `json:"interval"` // seconds per photo
	Shuffle  bool    `json:"shuffle"`
	Error    string  `json:"error,omitempty"`
}

type Service struct {
	Dir      string
	Interval time.Duration
	Shuffle  bool

	mu     sync.RWMutex
	items  []Photo
	byPath map[string]bool
	err    string
}

func NewService(dir string, interval time.Duration, shuffle bool) *Service {
	return &Service{Dir: dir, Interval: interval, Shuffle: shuffle, byPath: map[string]bool{}}
}

func (s *Service) Run(ctx context.Context, every time.Duration) {
	s.Scan()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Scan()
		}
	}
}

// Scan walks the folder (including subfolders, e.g. one per album) and
// replaces the list. Hidden files are skipped: macOS leaves "._foo.jpg"
// next to every file copied over SMB, Synology leaves "@eaDir".
func (s *Service) Scan() {
	var items []Photo
	err := fs.WalkDir(os.DirFS(s.Dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return nil // unreadable subfolder: skip it, keep the rest
		}
		name := d.Name()
		if p != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@")) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if _, ok := exts[strings.ToLower(path.Ext(name))]; !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() == 0 {
			return nil
		}
		items = append(items, Photo{Path: p, V: info.ModTime().Unix()})
		if len(items) >= maxPhotos {
			return fs.SkipAll
		}
		return nil
	})

	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	byPath := make(map[string]bool, len(items))
	for _, it := range items {
		byPath[it.Path] = true
	}

	msg := ""
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			msg = "Ordner fehlt: " + s.Dir
		} else {
			msg = err.Error()
		}
	}

	s.mu.Lock()
	changed := len(items) != len(s.items) || msg != s.err
	s.items, s.byPath, s.err = items, byPath, msg
	s.mu.Unlock()
	if changed {
		if msg != "" {
			slog.Info("photos", "dir", s.Dir, "status", msg)
		} else {
			slog.Info("photos", "dir", s.Dir, "count", len(items))
		}
	}
}

func (s *Service) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &Snapshot{
		Items:    append([]Photo{}, s.items...),
		Interval: int(s.Interval / time.Second),
		Shuffle:  s.Shuffle,
		Error:    s.err,
	}
}

// ServeHTTP serves /photos/<path>. Only files from the last scan are served,
// and they're opened through os.Root, so neither "../" nor a symlink can
// reach anything outside the folder.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	s.mu.RLock()
	ok := s.byPath[p]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	f, err := root.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", exts[strings.ToLower(path.Ext(p))])
	// URLs carry ?v=<mtime>, so the Pi can cache each photo for good.
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
