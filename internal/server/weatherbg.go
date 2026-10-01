package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Weather pictures: the clock/weather card shows a photo matching the current
// weather, taken from WEATHER_BG_DIR (/data/wetter). Files are named after the
// condition, optionally with a night variant:
//
//	klar  heiter  bewoelkt  nebel  regen  schnee  gewitter   (+ "-nacht")
//	nacht    – any weather while it's dark, if there's no "<name>-nacht"
//	standard – anything without a picture of its own
//
// Lookup at night:  regen-nacht → nacht → regen → standard
// Lookup by day:    regen → standard
// Nothing found → the frontend paints a gradient for the weather instead.
// Files are looked up on every poll, so new pictures show up without a restart.

type weatherBG struct {
	Name string `json:"name"` // file name without extension, e.g. "regen-nacht"
	V    int64  `json:"v"`    // mtime, busts the browser cache
}

// weatherBGName maps the weather icon key to the picture name.
var weatherBGName = map[string]string{
	"sun": "klar", "moon": "klar",
	"partly": "heiter", "partly-night": "heiter",
	"cloud":   "bewoelkt",
	"fog":     "nebel",
	"drizzle": "regen", "rain": "regen",
	"sleet": "schnee", "snow": "schnee",
	"thunder": "gewitter",
}

var weatherBGExts = []string{".jpg", ".jpeg", ".webp", ".png"}

// weatherBGCandidates lists the picture names to try, best first.
func weatherBGCandidates(icon string, isDay bool) []string {
	base := weatherBGName[icon]
	var out []string
	if !isDay {
		if base != "" {
			out = append(out, base+"-nacht")
		}
		out = append(out, "nacht")
	}
	if base != "" {
		out = append(out, base)
	}
	return append(out, "standard")
}

// validWeatherBG reports whether name is one of the picture names above –
// the /weather-bg/{name} handler serves nothing else.
func validWeatherBG(name string) bool {
	if name == "nacht" || name == "standard" {
		return true
	}
	base := strings.TrimSuffix(name, "-nacht")
	for _, v := range weatherBGName {
		if v == base {
			return true
		}
	}
	return false
}

func findWeatherBG(dir, name string) (string, os.FileInfo) {
	if dir == "" || !validWeatherBG(name) {
		return "", nil
	}
	for _, ext := range weatherBGExts {
		for _, n := range []string{name + ext, name + strings.ToUpper(ext)} {
			p := filepath.Join(dir, n)
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				return p, info
			}
		}
	}
	return "", nil
}

// pickWeatherBG returns the picture for the current weather, or nil.
func pickWeatherBG(dir, icon string, isDay bool) *weatherBG {
	if dir == "" {
		return nil
	}
	for _, name := range weatherBGCandidates(icon, isDay) {
		if _, info := findWeatherBG(dir, name); info != nil {
			return &weatherBG{Name: name, V: info.ModTime().Unix()}
		}
	}
	return nil
}

func (s *Server) handleWeatherBG(w http.ResponseWriter, r *http.Request) {
	p, info := findWeatherBG(s.cfg.WeatherBGDir, r.PathValue("name"))
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
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
