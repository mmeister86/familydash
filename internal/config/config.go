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
	Panel string // "column" (own column in the calendar row) or "school" (card next to beste.schule)
	Num   int    // n from CALENDAR_n_*
	// Into is the index (in Config.Calendars) of the column this calendar is
	// shown in together with its own events, or -1 for its own column.
	// Set via CALENDAR_n_COLUMN=<m>, e.g. holidays inside the family column.
	Into int
}

type Config struct {
	ListenAddr string
	Location   *time.Location

	Calendars       []Calendar
	CalendarDays    int
	CalendarRefresh time.Duration

	WeatherLat     float64
	WeatherLon     float64
	WeatherName    string
	WeatherRefresh time.Duration

	BringEmail    string
	BringPassword string
	BringList     string // list name; empty = the account's default list
	BringLocale   string
	BringRefresh  time.Duration

	SchoolToken    string
	SchoolURL      string
	SchoolStudents []string // optional filter: student names or ids
	SchoolRefresh  time.Duration

	TimetableFile string // optional JSON override for the built-in fixed timetable
	TimetableOff  bool

	// VielfaltMenü school lunch, one login per child (VIELFALT_n_*)
	Meals        []MealAccount
	MealsRefresh time.Duration
}

type MealAccount struct {
	Name     string // VIELFALT_n_NAME, empty = name from the portal
	User     string // VIELFALT_n_USER = Kundennummer
	Password string // VIELFALT_n_PASSWORD
	Color    string // VIELFALT_n_COLOR
}

// Default colors used when a calendar has no explicit color.
var palette = []string{"#4F8EF7", "#F76C5E", "#46C28E", "#F2B53A", "#A77BF3", "#3CC4D8"}

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:      env("LISTEN_ADDR", ":8080"),
		CalendarDays:    envInt("CALENDAR_DAYS", 7),
		CalendarRefresh: envDuration("CALENDAR_REFRESH", 5*time.Minute),
		WeatherName:     env("WEATHER_NAME", ""),
		WeatherRefresh:  envDuration("WEATHER_REFRESH", 15*time.Minute),

		BringEmail:    env("BRING_EMAIL", ""),
		BringPassword: env("BRING_PASSWORD", ""),
		BringList:     env("BRING_LIST", ""),
		BringLocale:   env("BRING_LOCALE", "de-DE"),
		BringRefresh:  envDuration("BRING_REFRESH", 2*time.Minute),

		SchoolToken:   env("BESTESCHULE_TOKEN", ""),
		SchoolURL:     strings.TrimRight(env("BESTESCHULE_URL", "https://beste.schule/api"), "/"),
		SchoolRefresh: envDuration("BESTESCHULE_REFRESH", 15*time.Minute),

		TimetableFile: env("TIMETABLE_FILE", ""),
		MealsRefresh:  envDuration("VIELFALT_REFRESH", 30*time.Minute),
		TimetableOff:  strings.EqualFold(env("TIMETABLE_FILE", ""), "off"),
	}
	if c.TimetableOff {
		c.TimetableFile = ""
	}
	for _, s := range strings.Split(env("BESTESCHULE_STUDENTS", ""), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.SchoolStudents = append(c.SchoolStudents, s)
		}
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
	intoNum := map[int]int{} // calendar index -> requested target calendar number
	for i := 1; i <= 20; i++ {
		url := env(fmt.Sprintf("CALENDAR_%d_URL", i), "")
		if url == "" {
			continue
		}
		panel := strings.ToLower(env(fmt.Sprintf("CALENDAR_%d_PANEL", i), "column"))
		if panel != "school" {
			panel = "column"
		}
		c.Calendars = append(c.Calendars, Calendar{
			URL:   url,
			Name:  env(fmt.Sprintf("CALENDAR_%d_NAME", i), fmt.Sprintf("Kalender %d", i)),
			Color: env(fmt.Sprintf("CALENDAR_%d_COLOR", i), palette[(i-1)%len(palette)]),
			Panel: panel,
			Num:   i,
			Into:  -1,
		})
		if n := envInt(fmt.Sprintf("CALENDAR_%d_COLUMN", i), 0); n > 0 && n != i {
			intoNum[len(c.Calendars)-1] = n
		}
	}
	c.resolveColumns(intoNum)

	// VIELFALT_1_USER, VIELFALT_1_PASSWORD, VIELFALT_1_NAME, VIELFALT_1_COLOR, VIELFALT_2_… (up to 6)
	for i := 1; i <= 6; i++ {
		user := env(fmt.Sprintf("VIELFALT_%d_USER", i), "")
		pass := env(fmt.Sprintf("VIELFALT_%d_PASSWORD", i), "")
		if user == "" || pass == "" {
			continue
		}
		c.Meals = append(c.Meals, MealAccount{
			User:     user,
			Password: pass,
			Name:     env(fmt.Sprintf("VIELFALT_%d_NAME", i), ""),
			Color:    env(fmt.Sprintf("VIELFALT_%d_COLOR", i), palette[(i+1)%len(palette)]),
		})
	}
	return c, nil
}

// resolveColumns turns CALENDAR_n_COLUMN=<m> into an index. The target must
// exist, be a column itself and not be merged into another column; otherwise
// the setting is ignored and the calendar keeps its own column.
func (c *Config) resolveColumns(intoNum map[int]int) {
	byNum := map[int]int{}
	for i, cal := range c.Calendars {
		byNum[cal.Num] = i
	}
	for i, n := range intoNum {
		t, ok := byNum[n]
		if !ok || c.Calendars[i].Panel != "column" || c.Calendars[t].Panel != "column" {
			continue
		}
		if _, merged := intoNum[t]; merged {
			continue // no chains: the target must stay a real column
		}
		c.Calendars[i].Into = t
	}
}

func (c *Config) WeatherEnabled() bool { return c.WeatherLat != 0 || c.WeatherLon != 0 }
func (c *Config) BringEnabled() bool   { return c.BringEmail != "" && c.BringPassword != "" }
func (c *Config) SchoolEnabled() bool  { return c.SchoolToken != "" }
func (c *Config) MealsEnabled() bool   { return len(c.Meals) > 0 }

// env reads a variable, trims whitespace and one pair of surrounding quotes.
// docker --env-file passes quotes through literally, so KEY="value" would
// otherwise end up with the quotes inside the value.
func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil && v > 0 {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v := env(key, "")
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
	if d, err := time.ParseDuration(env(key, "")); err == nil && d > 0 {
		return d
	}
	return def
}
