package main

import (
	"context"
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

	"familydash/internal/calendar"
	"familydash/internal/config"
	"familydash/internal/reminders"
	"familydash/internal/server"
	"familydash/internal/weather"
	"familydash/web"
)

// version is set at build time (-ldflags "-X main.version=…"). The frontend
// reloads itself when it changes, so the kiosk picks up new UI after an update.
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz and exit (for Docker HEALTHCHECK)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if *healthcheck {
		os.Exit(probe(cfg.ListenAddr))
	}
	if version == "dev" {
		version = fmt.Sprintf("dev-%d", time.Now().Unix())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rem, err := reminders.NewStore(cfg.DataDir, cfg.RemindersStale)
	if err != nil {
		slog.Error("reminders store", "err", err)
		os.Exit(1)
	}

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

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.New(cfg, version, cal, wx, rem, web.Static()).Handler(),
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
		"remindersPush", cfg.RemindersToken != "")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
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
