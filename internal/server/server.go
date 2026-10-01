package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/briefing"
	"familydash/internal/bring"
	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/news"
	"familydash/internal/photos"
	"familydash/internal/scene"
	"familydash/internal/things"
	"familydash/internal/timetable"
	"familydash/internal/uptime"
	"familydash/internal/vielfalt"
	"familydash/internal/waste"
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
	meals    *vielfalt.Service    // nil if not configured
	waste    *waste.Service       // nil if not configured
	photos   *photos.Service      // nil if not configured
	todos    *things.Service      // nil if not configured
	uptime   *uptime.Service      // nil if not configured
	news     *news.Service        // nil if not configured
	briefing *briefing.Service    // nil if not configured
	static   fs.FS
}

// Sources bundles the optional data sources (nil = not configured).
type Sources struct {
	Weather *weather.Service
	Bring   *bring.Service
	School  *besteschule.Service
	Plan    *timetable.File
	Meals   *vielfalt.Service
	Waste   *waste.Service
	Photos  *photos.Service
	Todos   *things.Service
	Uptime  *uptime.Service
	News    *news.Service
	// AI card (morning briefing / evening outlook)
	Briefing *briefing.Service
}

func New(cfg *config.Config, version string, cal *calendar.Service, src Sources, static fs.FS) *Server {
	return &Server{cfg: cfg, version: version, calendar: cal, weather: src.Weather, bring: src.Bring, school: src.School,
		plan: src.Plan, meals: src.Meals, waste: src.Waste, photos: src.Photos, todos: src.Todos, uptime: src.Uptime, news: src.News, briefing: src.Briefing, static: static}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	if s.photos != nil {
		mux.Handle("GET /photos/{path...}", s.photos)
	}
	mux.HandleFunc("GET /night-bg", s.handleNightBG)
	mux.HandleFunc("GET /weather-bg/{name}", s.handleWeatherBG)
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
	// School lunch ordered via VielfaltMenü, one card per child
	Meals *vielfalt.Meals `json:"meals,omitempty"`
	// Next bin collections (WASTE_n_*)
	Waste []waste.Pickup `json:"waste,omitempty"`
	// Time-of-day layout (SCENE_*)
	Scene scene.Current `json:"scene"`
	// Slideshow (PHOTOS_DIR); files at /photos/<path>?v=<v>
	Photos *photos.Snapshot `json:"photos,omitempty"`
	// Today's to-dos of one Things area (THINGS_*)
	Todos *things.List `json:"todos,omitempty"`
	// Background of the night scene (NIGHT_BG); file at /night-bg?v=<v>
	NightBG *nightBG `json:"nightBg,omitempty"`
	// Monitors of one Uptime Kuma status page, shown in the footer (UPTIME_*)
	Uptime *uptime.Status `json:"uptime,omitempty"`
	// Picture for the current weather (WEATHER_BG_DIR); file at /weather-bg/<name>?v=<v>
	WeatherBG *weatherBG `json:"weatherBg,omitempty"`
	// Headlines (NEWS_*), grouped: region first, then Germany
	News *news.Snapshot `json:"news,omitempty"`
	// AI card: briefing for today (morning) or outlook on tomorrow (evening)
	Briefing *briefing.Briefing `json:"briefing,omitempty"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	d := dashboard{
		Version:  s.version,
		Now:      time.Now().In(s.cfg.Location),
		Timezone: s.cfg.Location.String(),
		Days:     s.cfg.CalendarDays,
		Calendar: s.calendar.Snapshot(),
	}
	d.Scene = s.cfg.Scenes.At(d.Now, s.cfg.Location)
	if d.Calendar.Events == nil {
		d.Calendar.Events = []calendar.Event{}
	}
	if s.weather != nil {
		d.Weather = s.weather.Snapshot()
		if d.Weather != nil && d.Weather.Current.Icon != "" {
			d.WeatherBG = pickWeatherBG(s.cfg.WeatherBGDir, d.Weather.Current.Icon, d.Weather.Current.IsDay)
		}
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
	if s.meals != nil {
		d.Meals = s.meals.Snapshot()
	}
	if s.waste != nil {
		d.Waste = s.waste.Snapshot(d.Now, s.cfg.Location)
	}
	if s.photos != nil {
		d.Photos = s.photos.Snapshot()
	}
	if s.todos != nil {
		d.Todos = s.todos.Snapshot()
	}
	if s.uptime != nil {
		d.Uptime = s.uptime.Snapshot()
	}
	if s.news != nil {
		d.News = s.news.Snapshot()
	}
	if s.briefing != nil {
		// ?scene=morning|evening (testing on a laptop): that scene's card, made on request
		switch k := briefing.Kind(r.URL.Query().Get("scene")); k {
		case briefing.Morning, briefing.Evening:
			d.Briefing = s.briefing.For(k, d.Now)
		default:
			d.Briefing = s.briefing.Snapshot()
		}
	}
	if _, info := findNightBG(s.cfg.NightBGCandidates()); info != nil {
		d.NightBG = &nightBG{V: info.ModTime().Unix()}
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
