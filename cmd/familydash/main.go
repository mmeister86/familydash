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
	"familydash/internal/bring"
	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/server"
	"familydash/internal/timetable"
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
	if *schoolDump || *schoolPreview {
		os.Exit(school(cfg, *schoolDump))
	}
	if version == "dev" {
		version = fmt.Sprintf("dev-%d", time.Now().Unix())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cal := calendar.NewService(cfg)
	go cal.Run(ctx, cfg.CalendarRefresh)

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
		go sc.Run(ctx, cfg.SchoolRefresh)
	}

	var plan *timetable.File
	if !cfg.TimetableOff {
		if plan, err = timetable.Load(cfg.TimetableFile); err != nil {
			slog.Error("timetable", "err", err) // keep running without it
			plan = nil
		}
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.New(cfg, version, cal, wx, br, sc, plan, web.Static()).Handler(),
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
		"bring", cfg.BringEnabled(), "besteschule", cfg.SchoolEnabled(), "timetable", plan != nil)
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
		out = besteschule.Build(raw, time.Now(), cfg.Location, cfg.SchoolStudents)
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
