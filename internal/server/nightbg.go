package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The night scene can show a full-screen picture behind the dimmed clock.
// The file is looked up on every request, so dropping in a new bg.jpeg (or
// deleting it) takes effect with the next poll – no restart needed.

type nightBG struct {
	V int64 `json:"v"` // mtime, busts the browser cache when the file is replaced
}

var bgTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".webp": "image/webp", ".avif": "image/avif",
}

// findNightBG returns the first candidate that is a readable image file.
func findNightBG(candidates []string) (string, os.FileInfo) {
	for _, p := range candidates {
		if _, ok := bgTypes[strings.ToLower(filepath.Ext(p))]; !ok {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		return p, info
	}
	return "", nil
}

func (s *Server) handleNightBG(w http.ResponseWriter, r *http.Request) {
	p, info := findNightBG(s.cfg.NightBGCandidates())
	if info == nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", bgTypes[strings.ToLower(filepath.Ext(p))])
	// The URL carries ?v=<mtime>, so the Pi keeps it cached until it changes.
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
