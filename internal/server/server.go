package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/bring"
	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/timetable"
	"familydash/internal/weather"
)

type Server struct {
	cfg      *config.Config
	version  string
	calendar *calendar.Service
	weather  *weather.Service     // nil if not configured
	bring    *bring.Service       // nil if not configured
	school   *besteschule.Service // nil if not configured
	plan     *timetable.File      // nil if not configured
	static   fs.FS
}

func New(cfg *config.Config, version string, cal *calendar.Service, wx *weather.Service, br *bring.Service, sc *besteschule.Service, plan *timetable.File, static fs.FS) *Server {
	return &Server{cfg: cfg, version: version, calendar: cal, weather: wx, bring: br, school: sc, plan: plan, static: static}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	files := http.FileServerFS(s.static)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The kiosk never gets a hard refresh – always revalidate.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	return logRequests(mux)
}

type dashboard struct {
	Version  string              `json:"version"`
	Now      time.Time           `json:"now"`
	Timezone string              `json:"timezone"`
	Days     int                 `json:"days"`
	Calendar calendar.Snapshot   `json:"calendar"`
	Weather  *weather.Weather    `json:"weather,omitempty"`
	Shopping *bring.List         `json:"shopping,omitempty"`
	School   *besteschule.School `json:"school,omitempty"`
	// Fixed timetables (TIMETABLE_FILE / built-in) for children without beste.schule
	Timetables []timetable.Card `json:"timetables,omitempty"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	d := dashboard{
		Version:  s.version,
		Now:      time.Now().In(s.cfg.Location),
		Timezone: s.cfg.Location.String(),
		Days:     s.cfg.CalendarDays,
		Calendar: s.calendar.Snapshot(),
	}
	if d.Calendar.Events == nil {
		d.Calendar.Events = []calendar.Event{}
	}
	if s.weather != nil {
		d.Weather = s.weather.Snapshot()
	}
	if s.bring != nil {
		d.Shopping = s.bring.Snapshot()
	}
	if s.school != nil {
		d.School = s.school.Snapshot()
	}
	if s.plan != nil {
		d.Timetables = s.plan.Build(d.Now, s.cfg.Location)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, d)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write response", "err", err)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests logs errors and API calls except the once-a-minute dashboard poll.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/api/dashboard" && rec.status < 400 {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || rec.status >= 400 {
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start).Round(time.Millisecond))
		}
	})
}
