// Package config loads all settings from environment variables so the
// container can be configured entirely through the Unraid template.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Calendar struct {
	Name  string
	Color string
	URL   string
}

type Config struct {
	ListenAddr string
	DataDir    string
	Location   *time.Location

	Calendars       []Calendar
	CalendarDays    int
	CalendarRefresh time.Duration

	WeatherLat     float64
	WeatherLon     float64
	WeatherName    string
	WeatherRefresh time.Duration

	RemindersToken string
	RemindersStale time.Duration
}

// Default colors used when a calendar has no explicit color.
var palette = []string{"#4F8EF7", "#F76C5E", "#46C28E", "#F2B53A", "#A77BF3", "#3CC4D8"}

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:      env("LISTEN_ADDR", ":8080"),
		DataDir:         env("DATA_DIR", "/data"),
		CalendarDays:    envInt("CALENDAR_DAYS", 7),
		CalendarRefresh: envDuration("CALENDAR_REFRESH", 5*time.Minute),
		WeatherName:     env("WEATHER_NAME", ""),
		WeatherRefresh:  envDuration("WEATHER_REFRESH", 15*time.Minute),
		RemindersToken:  env("REMINDERS_TOKEN", ""),
		RemindersStale:  envDuration("REMINDERS_STALE_AFTER", 2*time.Hour),
	}

	tz := env("TZ", "Europe/Berlin")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("TZ %q: %w", tz, err)
	}
	c.Location = loc

	if c.WeatherLat, err = envFloat("WEATHER_LAT", 0); err != nil {
		return nil, err
	}
	if c.WeatherLon, err = envFloat("WEATHER_LON", 0); err != nil {
		return nil, err
	}

	// CALENDAR_1_URL, CALENDAR_1_NAME, CALENDAR_1_COLOR, CALENDAR_2_… (up to 20, gaps allowed)
	for i := 1; i <= 20; i++ {
		url := strings.TrimSpace(os.Getenv(fmt.Sprintf("CALENDAR_%d_URL", i)))
		if url == "" {
			continue
		}
		c.Calendars = append(c.Calendars, Calendar{
			URL:   url,
			Name:  env(fmt.Sprintf("CALENDAR_%d_NAME", i), fmt.Sprintf("Kalender %d", i)),
			Color: env(fmt.Sprintf("CALENDAR_%d_COLOR", i), palette[(i-1)%len(palette)]),
		})
	}
	return c, nil
}

func (c *Config) WeatherEnabled() bool { return c.WeatherLat != 0 || c.WeatherLon != 0 }

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return f, nil
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}
