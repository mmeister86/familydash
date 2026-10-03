package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed zoneinfo → works in a scratch image

	"familydash/internal/besteschule"
	"familydash/internal/briefing"
	"familydash/internal/bring"
	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/familyapp"
	"familydash/internal/news"
	"familydash/internal/photos"
	"familydash/internal/server"
	"familydash/internal/timetable"
	"familydash/internal/todo"
	"familydash/internal/uptime"
	"familydash/internal/vielfalt"
	"familydash/internal/waste"
	"familydash/internal/weather"
	"familydash/web"
)

// version is set at build time (-ldflags "-X main.version=…"). The frontend
// reloads itself when it changes, so the kiosk picks up new UI after an update.
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz and exit (for Docker HEALTHCHECK)")
	schoolDump := flag.Bool("besteschule-dump", false, "print the raw beste.schule API responses as JSON and exit")
	schoolPreview := flag.Bool("besteschule-preview", false, "print what the dashboard would show from beste.schule and exit")
	mealsPreview := flag.Bool("vielfalt-preview", false, "log in to VielfaltMenü, print the ordered meals and exit")
	briefPreview := flag.String("briefing-preview", "", "morning|evening: load all sources once, print the facts and the AI card and exit")
	appPreview := flag.Bool("familyapp-preview", false, "load all sources once, print what would be pushed to the family app (nothing is sent) and its to-dos, and exit")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if *healthcheck {
		os.Exit(probe(cfg.ListenAddr))
	}
	for _, w := range cfg.Warnings {
		slog.Warn("config", "problem", w)
	}
	if *schoolDump || *schoolPreview {
		os.Exit(school(cfg, *schoolDump))
	}
	if *mealsPreview {
		os.Exit(meals(cfg))
	}
	if *briefPreview != "" {
		os.Exit(brief(cfg, briefing.Kind(*briefPreview)))
	}
	if *appPreview {
		os.Exit(appPreviewRun(cfg))
	}
	if version == "dev" {
		version = fmt.Sprintf("dev-%d", time.Now().Unix())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cal := newCalendarSource(cfg)
	if cfg.CalendarSource == "convex" {
		go cal.Run(ctx, familyapp.ConvexPollInterval)
	} else {
		go cal.Run(ctx, cfg.CalendarRefresh)
	}

	var wx *weather.Service
	if cfg.WeatherEnabled() {
		wx = weather.NewService(cfg.WeatherLat, cfg.WeatherLon, cfg.WeatherName, cfg.Location)
		if u := os.Getenv("WEATHER_API_URL"); u != "" {
			wx.BaseURL = u
		}
		go wx.Run(ctx, cfg.WeatherRefresh)
	}

	var br *bring.Service
	if cfg.BringEnabled() {
		br = bring.NewService(cfg.BringEmail, cfg.BringPassword, cfg.BringList, cfg.BringLocale)
		if u := os.Getenv("BRING_API_URL"); u != "" { // for local testing
			br.BaseURL, br.LocaleURL = u+"/rest/", u+"/locale/"
		}
		go br.Run(ctx, cfg.BringRefresh)
	}

	var sc *besteschule.Service
	if cfg.SchoolEnabled() {
		sc = besteschule.NewService(cfg.SchoolURL, cfg.SchoolToken, cfg.Location, cfg.SchoolStudents)
		sc.SetWeekFixes(cfg.SchoolWeekFix)
		go sc.Run(ctx, cfg.SchoolRefresh)
	}

	var plan *timetable.File
	if !cfg.TimetableOff {
		if plan, err = timetable.Load(cfg.TimetableFile); err != nil {
			slog.Error("timetable", "err", err) // keep running without it
			plan = nil
		}
	}

	var ml *vielfalt.Service
	if cfg.MealsEnabled() {
		ml = newMeals(cfg)
		go ml.Run(ctx, cfg.MealsRefresh)
	}

	var ws *waste.Service
	if len(cfg.Waste) > 0 {
		ws = &waste.Service{Bins: cfg.Waste, Shift: cfg.WasteShift}
	}

	var ph *photos.Service
	if cfg.PhotosDir != "" {
		ph = photos.NewService(cfg.PhotosDir, cfg.PhotosInterval, cfg.PhotosShuffle)
		go ph.Run(ctx, cfg.PhotosRefresh)
	}

	// to-dos come from the family app (GET /todos); unset tokens = no card
	var todos todo.Source
	if cfg.TodosEnabled() {
		fa := familyapp.NewTodoService(newAppClient(cfg), cfg.Location)
		go fa.Run(ctx, cfg.FamilyAppRefresh)
		todos = fa
	}

	var up *uptime.Service
	if cfg.UptimeEnabled() {
		up = uptime.NewService(cfg.UptimeBase, cfg.UptimeSlug, cfg.UptimeAPIKey, cfg.UptimeCertWarn)
		go up.Run(ctx, cfg.UptimeRefresh)
	}

	var nw *news.Service
	if cfg.NewsEnabled() {
		nw = news.NewService(cfg.NewsFeeds, cfg.NewsMaxAge, cfg.NewsPerGroup)
		go nw.Run(ctx, cfg.NewsRefresh)
	}

	src := server.Sources{Weather: wx, Bring: br, School: sc, Plan: plan, Meals: ml, Waste: ws, Photos: ph, Todos: todos, Uptime: up, News: nw}
	if cfg.BriefingEnabled() {
		src.Briefing = newBriefing(cfg, cal, src)
		go src.Briefing.Run(ctx)
	}
	if cfg.FamilyAppPushEnabled() {
		go newPusher(cfg, cal, src).Run(ctx, time.Minute)
	}
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.New(cfg, version, cal, src, web.Static()).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	slog.Info("familydash started", "version", version, "addr", cfg.ListenAddr,
		"calendars", len(cfg.Calendars), "weather", cfg.WeatherEnabled(), "tz", cfg.Location.String(),
		"bring", cfg.BringEnabled(), "besteschule", cfg.SchoolEnabled(), "timetable", plan != nil, "vielfalt", len(cfg.Meals), "waste", len(cfg.Waste),
		"photos", cfg.PhotosDir, "todos", todoMode(cfg), "familyappPush", cfg.FamilyAppPushEnabled(), "briefing", briefingMode(cfg), "uptime", cfg.UptimeEnabled(), "news", len(cfg.NewsFeeds), "weatherBg", cfg.WeatherBGDir, "scene", cfg.Scenes.At(time.Now(), cfg.Location).Name)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// school prints beste.schule data for debugging the parser against a real
// account: `docker exec familydash /familydash -besteschule-dump > dump.json`
func school(cfg *config.Config, rawDump bool) int {
	if !cfg.SchoolEnabled() {
		fmt.Fprintln(os.Stderr, "BESTESCHULE_TOKEN is not set")
		return 1
	}
	s := besteschule.NewService(cfg.SchoolURL, cfg.SchoolToken, cfg.Location, cfg.SchoolStudents)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := s.Fetch(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var out any = raw
	if !rawDump {
		out = besteschule.Build(raw, time.Now(), cfg.Location, cfg.SchoolStudents, cfg.SchoolWeekFix...)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newMeals(cfg *config.Config) *vielfalt.Service {
	accs := make([]vielfalt.Account, len(cfg.Meals))
	for i, m := range cfg.Meals {
		accs[i] = vielfalt.Account{Name: m.Name, User: m.User, Password: m.Password, Color: m.Color}
	}
	s := vielfalt.NewService(accs, cfg.Location)
	if u := os.Getenv("VIELFALT_API_URL"); u != "" { // for local testing
		s.LoginURL, s.IBSURL = u, u
	}
	return s
}

// meals checks the VielfaltMenü logins: `docker exec familydash /familydash -vielfalt-preview`
func meals(cfg *config.Config) int {
	if !cfg.MealsEnabled() {
		fmt.Fprintln(os.Stderr, "VIELFALT_1_USER / VIELFALT_1_PASSWORD are not set")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s := newMeals(cfg)
	s.Refresh(ctx)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	snap := s.Snapshot()
	if err := enc.Encode(snap); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, c := range snap.Children {
		if c.Error != "" {
			return 1
		}
	}
	return 0
}

// newCalendarSource selects the calendar backend once for every consumer
// (server, briefing, pusher, previews). Convex mode never creates the local
// ICS poller — not even when credentials, HTTP or validation fail; an
// invalid chosen convex config yields a visible unavailable-source state,
// never a silent fallback.
func newCalendarSource(cfg *config.Config) calendar.Source {
	if cfg.CalendarSource == "convex" {
		return familyapp.NewCalendarService(newAppClient(cfg), cfg.FamilyAppCalendarToken, cfg.CalendarCacheFile, cfg.Location)
	}
	return calendar.NewService(cfg)
}

// newBriefing wires the AI card to the other sources (nil ones are skipped).
func newBriefing(cfg *config.Config, cal calendar.Source, src server.Sources) *briefing.Service {
	b := &briefing.Service{
		Sched: &cfg.Scenes, Loc: cfg.Location, Lead: cfg.BriefingLead, MinGap: cfg.BriefingMinGap,
		Gather: func(now time.Time) briefing.Data {
			d := briefing.Data{Calendar: cal.Snapshot()}
			if src.Weather != nil {
				d.Weather = src.Weather.Snapshot()
			}
			if src.School != nil {
				d.School = src.School.Snapshot()
			}
			if src.Plan != nil {
				d.Timetables = src.Plan.Build(now, cfg.Location)
			}
			if src.Meals != nil {
				d.Meals = src.Meals.Snapshot()
			}
			if src.Waste != nil {
				d.Waste = src.Waste.Snapshot(now, cfg.Location)
			}
			if src.Todos != nil {
				d.Todos = src.Todos.Snapshot()
			}
			return d
		},
	}
	if cfg.GeminiKey != "" {
		b.Model = briefing.NewGemini(cfg.GeminiKey, cfg.GeminiModel, cfg.GeminiThinking, cfg.GeminiBackend)
	}
	return b
}

func briefingMode(cfg *config.Config) string {
	switch {
	case !cfg.BriefingEnabled():
		return "off"
	case cfg.GeminiKey == "":
		return "rules"
	}
	return cfg.GeminiModel
}

// brief loads every source once and prints facts + card:
// `docker exec familydash /familydash -briefing-preview=evening`
func brief(cfg *config.Config, kind briefing.Kind) int {
	if kind != briefing.Morning && kind != briefing.Evening {
		fmt.Fprintln(os.Stderr, "-briefing-preview: want morning or evening")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cal := newCalendarSource(cfg)
	cal.Refresh(ctx)
	var src server.Sources
	if cfg.WeatherEnabled() {
		src.Weather = weather.NewService(cfg.WeatherLat, cfg.WeatherLon, cfg.WeatherName, cfg.Location)
		src.Weather.Refresh(ctx)
	}
	if cfg.SchoolEnabled() {
		src.School = besteschule.NewService(cfg.SchoolURL, cfg.SchoolToken, cfg.Location, cfg.SchoolStudents)
		src.School.SetWeekFixes(cfg.SchoolWeekFix)
		src.School.Refresh(ctx)
	}
	if !cfg.TimetableOff {
		if plan, err := timetable.Load(cfg.TimetableFile); err == nil {
			src.Plan = plan
		}
	}
	if cfg.MealsEnabled() {
		src.Meals = newMeals(cfg)
		src.Meals.Refresh(ctx)
	}
	if len(cfg.Waste) > 0 {
		src.Waste = &waste.Service{Bins: cfg.Waste, Shift: cfg.WasteShift}
	}
	src.Todos = loadTodosOnce(ctx, cfg)
	b := newBriefing(cfg, cal, src)
	card, facts, err := b.Generate(ctx, kind, time.Now())
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(map[string]any{"facts": facts, "briefing": card})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newAppClient(cfg *config.Config) *familyapp.Client {
	return familyapp.NewClient(cfg.FamilyAppURL, cfg.FamilyAppIngestToken, cfg.FamilyAppReadToken)
}

func todoMode(cfg *config.Config) string {
	if cfg.TodosEnabled() {
		return todo.SourceFamilyApp
	}
	return "off"
}

// loadTodosOnce fetches the family app's to-dos once (previews).
func loadTodosOnce(ctx context.Context, cfg *config.Config) todo.Source {
	if cfg.TodosEnabled() {
		s := familyapp.NewTodoService(newAppClient(cfg), cfg.Location)
		s.Refresh(ctx)
		return s
	}
	return nil
}

// newPusher sends each child's week and the briefings to the family app.
func newPusher(cfg *config.Config, cal calendar.Source, src server.Sources) *familyapp.Pusher {
	p := &familyapp.Pusher{
		Client: newAppClient(cfg), Loc: cfg.Location, Heartbeat: cfg.FamilyAppHeartbeat, ExtraCals: cfg.FamilyAppCalendars,
		Gather: func(now time.Time) familyapp.Inputs {
			in := familyapp.Inputs{Calendar: cal.Snapshot()}
			if src.School != nil {
				in.School = src.School.Snapshot()
				in.SchoolDays = src.School.Days(now, familyapp.Days)
			}
			if src.Plan != nil {
				in.Timetables = src.Plan.Build(now, cfg.Location)
				in.PlanDays = src.Plan.Days(now, familyapp.Days, cfg.Location)
			}
			if src.Meals != nil {
				in.Meals = src.Meals.Snapshot()
			}
			return in
		},
	}
	if src.Briefing != nil {
		p.Briefings = src.Briefing.Cards
	}
	return p
}

// appPreviewRun loads every source once and prints the payloads the wall
// would push to the family app – nothing is sent – plus the to-dos it reads:
// `docker exec familydash /familydash -familyapp-preview`
func appPreviewRun(cfg *config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cal := newCalendarSource(cfg)
	cal.Refresh(ctx)
	var src server.Sources
	if cfg.SchoolEnabled() {
		src.School = besteschule.NewService(cfg.SchoolURL, cfg.SchoolToken, cfg.Location, cfg.SchoolStudents)
		src.School.SetWeekFixes(cfg.SchoolWeekFix)
		src.School.Refresh(ctx)
	}
	if !cfg.TimetableOff {
		if plan, err := timetable.Load(cfg.TimetableFile); err == nil {
			src.Plan = plan
		}
	}
	if cfg.MealsEnabled() {
		src.Meals = newMeals(cfg)
		src.Meals.Refresh(ctx)
	}
	now := time.Now()
	p := newPusher(cfg, cal, src)
	out := map[string]any{
		"children": familyapp.BuildChildren(p.Gather(now), now, cfg.Location, cfg.FamilyAppCalendars),
		"push":     cfg.FamilyAppPushEnabled(),
		"todos":    todoMode(cfg),
	}
	if cfg.TodosEnabled() {
		out["todoList"] = loadTodosOnce(ctx, cfg).Snapshot()
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func probe(addr string) int {
	host := addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + host + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
