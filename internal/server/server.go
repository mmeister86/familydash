package server

import (
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/reminders"
	"familydash/internal/weather"
)

type Server struct {
	cfg       *config.Config
	version   string
	calendar  *calendar.Service
	weather   *weather.Service // nil if not configured
	reminders *reminders.Store
	static    fs.FS
}

func New(cfg *config.Config, version string, cal *calendar.Service, wx *weather.Service, rem *reminders.Store, static fs.FS) *Server {
	return &Server{cfg: cfg, version: version, calendar: cal, weather: wx, reminders: rem, static: static}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("POST /api/reminders", s.handlePushReminders)
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
	Version   string             `json:"version"`
	Now       time.Time          `json:"now"`
	Timezone  string             `json:"timezone"`
	Days      int                `json:"days"`
	Calendar  calendar.Snapshot  `json:"calendar"`
	Weather   *weather.Weather   `json:"weather,omitempty"`
	Reminders reminders.Snapshot `json:"reminders"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().In(s.cfg.Location)
	d := dashboard{
		Version:   s.version,
		Now:       now,
		Timezone:  s.cfg.Location.String(),
		Days:      s.cfg.CalendarDays,
		Calendar:  s.calendar.Snapshot(),
		Reminders: s.reminders.Snapshot(now),
	}
	if d.Calendar.Events == nil {
		d.Calendar.Events = []calendar.Event{}
	}
	if s.weather != nil {
		d.Weather = s.weather.Snapshot()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handlePushReminders(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RemindersToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "REMINDERS_TOKEN is not set on the server"})
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.RemindersToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var p reminders.Push
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.reminders.Apply(p, time.Now()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

// logRequests logs everything except the once-a-minute dashboard poll.
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
