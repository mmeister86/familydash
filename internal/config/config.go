// Package config loads all settings from environment variables so the
// container can be configured entirely through the Unraid template.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"familydash/internal/besteschule"
	"familydash/internal/news"
	"familydash/internal/scene"
	"familydash/internal/uptime"
	"familydash/internal/waste"
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
	SchoolStudents []string              // optional filter: student names or ids
	SchoolWeekFix  []besteschule.WeekFix // BESTESCHULE_WEEK_FIX=Fr:PH=A
	SchoolRefresh  time.Duration

	TimetableFile string // optional JSON override for the built-in fixed timetable
	TimetableOff  bool

	// VielfaltMenü school lunch, one login per child (VIELFALT_n_*)
	Meals        []MealAccount
	MealsRefresh time.Duration

	// Bin collection rules (WASTE_n_NAME/_DAY/_WEEKS/_COLOR)
	Waste      []waste.Bin
	WasteShift bool // WASTE_HOLIDAY_SHIFT, default on

	// Time-of-day layouts (SCENE_<NAME>[_WEEKEND]=HH:MM, SCENE_FORCE)
	Scenes scene.Schedule

	// Photo slideshow (PHOTOS_DIR, PHOTOS_INTERVAL, PHOTOS_SHUFFLE)
	PhotosDir      string // "" = off
	PhotosInterval time.Duration
	PhotosShuffle  bool
	PhotosRefresh  time.Duration

	// Background picture of the night scene (NIGHT_BG): a file path, "" = look
	// for bg.jpeg/bg.jpg/bg.png/bg.webp in /data, "off" = plain black
	NightBG string

	// Weather pictures behind the weather (WEATHER_BG_DIR): klar.jpg, regen.jpg, …
	// "" = off; missing files fall back to a built-in gradient
	WeatherBGDir string

	// Headlines (NEWS_*): Google News searches + top stories + extra RSS feeds
	NewsFeeds    []news.Feed
	NewsRefresh  time.Duration
	NewsMaxAge   time.Duration
	NewsPerGroup int

	// Things 3 to-dos of one area (THINGS_*), read via the things3 CLI
	ThingsEmail    string
	ThingsPassword string
	ThingsArea     string
	ThingsBin      string
	ThingsStateDir string // sync cache of the CLI
	ThingsRefresh  time.Duration

	// Uptime Kuma status page in the footer (UPTIME_*)
	UptimeBase     string // http://host:3001, "" = off
	UptimeSlug     string // from UPTIME_URL …/status/<slug>
	UptimeAPIKey   string // optional: /metrics for certificate expiry
	UptimeCertWarn int    // warn at or below this many days
	UptimeRefresh  time.Duration

	// AI card: morning briefing / evening outlook (BRIEFING_*, GEMINI_*)
	Briefing       string // auto (on when GEMINI_API_KEY is set), on (rule-based without a key), off
	BriefingLead   time.Duration
	BriefingMinGap time.Duration
	GeminiKey      string
	GeminiModel    string
	GeminiThinking string // low | medium | high, "default" = model default
	GeminiBackend  string // auto | gemini (AI Studio) | vertex (Vertex AI express mode)
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

		PhotosDir:      env("PHOTOS_DIR", "/data/pictures"),
		PhotosInterval: envDuration("PHOTOS_INTERVAL", 45*time.Second),
		PhotosShuffle:  !strings.EqualFold(env("PHOTOS_SHUFFLE", "on"), "off"),
		PhotosRefresh:  envDuration("PHOTOS_REFRESH", 5*time.Minute),
		NightBG:        env("NIGHT_BG", ""),
		WeatherBGDir:   env("WEATHER_BG_DIR", "/data/wetter"),

		NewsRefresh:  envDuration("NEWS_REFRESH", 20*time.Minute),
		NewsMaxAge:   envDuration("NEWS_MAX_AGE", 48*time.Hour),
		NewsPerGroup: envInt("NEWS_PER_GROUP", 12),

		ThingsEmail:    env("THINGS_EMAIL", ""),
		ThingsPassword: env("THINGS_PASSWORD", ""),
		ThingsArea:     env("THINGS_AREA", "Familie"),
		ThingsBin:      env("THINGS_BIN", "/things3"),
		ThingsStateDir: env("THINGS_STATE_DIR", "/data/things"),
		ThingsRefresh:  envDuration("THINGS_REFRESH", 5*time.Minute),

		UptimeAPIKey:   env("UPTIME_API_KEY", ""),
		UptimeCertWarn: envInt("UPTIME_CERT_WARN_DAYS", 14),
		UptimeRefresh:  envDuration("UPTIME_REFRESH", time.Minute),

		Briefing:       strings.ToLower(env("BRIEFING", "auto")),
		BriefingLead:   envDuration("BRIEFING_LEAD", 20*time.Minute),
		BriefingMinGap: envDuration("BRIEFING_MIN_GAP", 20*time.Minute),
		GeminiKey:      env("GEMINI_API_KEY", ""),
		GeminiModel:    env("GEMINI_MODEL", "gemini-3.8-flash"),
		GeminiThinking: strings.ToLower(env("GEMINI_THINKING", "low")),
		GeminiBackend:  strings.ToLower(env("GEMINI_BACKEND", "auto")),
	}
	switch c.GeminiBackend {
	case "auto", "gemini", "vertex":
	default:
		return nil, fmt.Errorf("GEMINI_BACKEND %q: want auto, gemini or vertex", c.GeminiBackend)
	}
	if c.GeminiThinking == "default" || c.GeminiThinking == "off" {
		c.GeminiThinking = ""
	}
	if raw := env("UPTIME_URL", ""); raw != "" {
		base, slug, err := uptime.ParsePageURL(raw)
		if err != nil {
			return nil, fmt.Errorf("UPTIME_URL: %w", err)
		}
		c.UptimeBase, c.UptimeSlug = base, slug
	}
	if strings.EqualFold(c.PhotosDir, "off") {
		c.PhotosDir = ""
	}
	if strings.EqualFold(c.WeatherBGDir, "off") {
		c.WeatherBGDir = ""
	}
	c.loadNews()
	if err := c.loadScenes(); err != nil {
		return nil, err
	}
	if c.TimetableOff {
		c.TimetableFile = ""
	}
	for _, s := range strings.Split(env("BESTESCHULE_STUDENTS", ""), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.SchoolStudents = append(c.SchoolStudents, s)
		}
	}
	fixes, err := besteschule.ParseWeekFixes(env("BESTESCHULE_WEEK_FIX", ""))
	if err != nil {
		return nil, fmt.Errorf("BESTESCHULE_WEEK_FIX: %w", err)
	}
	c.SchoolWeekFix = fixes

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

	// WASTE_1_NAME=Restabfall, WASTE_1_DAY=Mi, WASTE_1_WEEKS=gerade … (up to 10)
	wasteColors := []string{"#8A8F98", "#F2C94C", "#4F8EF7", "#8B5E3C", "#46C28E"}
	c.WasteShift = !strings.EqualFold(env("WASTE_HOLIDAY_SHIFT", "on"), "off")
	for i := 1; i <= 10; i++ {
		name := env(fmt.Sprintf("WASTE_%d_NAME", i), "")
		if name == "" {
			continue
		}
		wd, err := waste.ParseWeekday(env(fmt.Sprintf("WASTE_%d_DAY", i), ""))
		if err != nil {
			return nil, fmt.Errorf("WASTE_%d_DAY: %w", i, err)
		}
		weeks, err := waste.ParseParity(env(fmt.Sprintf("WASTE_%d_WEEKS", i), ""))
		if err != nil {
			return nil, fmt.Errorf("WASTE_%d_WEEKS: %w", i, err)
		}
		c.Waste = append(c.Waste, waste.Bin{
			Name:    name,
			Color:   env(fmt.Sprintf("WASTE_%d_COLOR", i), wasteColors[(i-1)%len(wasteColors)]),
			Weekday: wd,
			Weeks:   weeks,
		})
	}

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

// loadScenes reads SCENE_MORNING … SCENE_NIGHT (school days, Mon–Fri) and
// SCENE_<NAME>_WEEKEND (Sat/Sun; falls back to the school-day value).
// A value of "off" skips that scene on those days.
func (c *Config) loadScenes() error {
	key := func(name string) string { return "SCENE_" + strings.ToUpper(name) }
	school, err := scene.Build(func(n string) string { return env(key(n), scene.DefaultSchoolDay[n]) })
	if err != nil {
		return fmt.Errorf("SCENE_*: %w", err)
	}
	weekend, err := scene.Build(func(n string) string {
		def := scene.DefaultWeekend[n]
		if def == "" {
			def = env(key(n), scene.DefaultSchoolDay[n])
		}
		return env(key(n)+"_WEEKEND", def)
	})
	if err != nil {
		return fmt.Errorf("SCENE_*_WEEKEND: %w", err)
	}
	force := strings.ToLower(env("SCENE_FORCE", ""))
	if force != "" && !scene.Valid(force) {
		return fmt.Errorf("SCENE_FORCE %q: want one of %s", force, strings.Join(scene.Names, ", "))
	}
	c.Scenes = scene.Schedule{SchoolDay: school, Weekend: weekend, Force: force}
	return nil
}

// loadNews builds the feed list, in display order:
//   - NEWS_LOCAL=Crimmitschau,Landkreis Zwickau → one Google News search per
//     term (last NEWS_LOCAL_DAYS days), shown together as NEWS_LOCAL_NAME
//   - NEWS_TOP=on (default) → Google News top stories Germany
//   - NEWS_1_URL/NEWS_1_NAME … NEWS_5_* → any other RSS feed
//
// NEWS=off switches the whole card off.
func (c *Config) loadNews() {
	if strings.EqualFold(env("NEWS", "on"), "off") {
		return
	}
	localName := env("NEWS_LOCAL_NAME", "Region")
	days := envInt("NEWS_LOCAL_DAYS", 2)
	for _, term := range strings.Split(env("NEWS_LOCAL", ""), ",") {
		if term = strings.TrimSpace(term); term != "" {
			c.NewsFeeds = append(c.NewsFeeds, news.Feed{Group: localName, URL: news.GoogleSearch(term, days)})
		}
	}
	if !strings.EqualFold(env("NEWS_TOP", "on"), "off") {
		c.NewsFeeds = append(c.NewsFeeds, news.Feed{Group: env("NEWS_TOP_NAME", "Deutschland"), URL: news.GoogleTop()})
	}
	for i := 1; i <= 5; i++ {
		if u := env(fmt.Sprintf("NEWS_%d_URL", i), ""); u != "" {
			c.NewsFeeds = append(c.NewsFeeds, news.Feed{Group: env(fmt.Sprintf("NEWS_%d_NAME", i), "News"), URL: u})
		}
	}
}

// NightBGCandidates lists the files tried for the night background, in order.
// The first one that exists wins; none = plain black night clock.
func (c *Config) NightBGCandidates() []string {
	switch {
	case strings.EqualFold(c.NightBG, "off"):
		return nil
	case c.NightBG != "":
		return []string{c.NightBG}
	}
	return []string{"/data/bg.jpeg", "/data/bg.jpg", "/data/bg.png", "/data/bg.webp"}
}

func (c *Config) WeatherEnabled() bool { return c.WeatherLat != 0 || c.WeatherLon != 0 }
func (c *Config) BringEnabled() bool   { return c.BringEmail != "" && c.BringPassword != "" }
func (c *Config) SchoolEnabled() bool  { return c.SchoolToken != "" }
func (c *Config) MealsEnabled() bool   { return len(c.Meals) > 0 }
func (c *Config) ThingsEnabled() bool  { return c.ThingsEmail != "" && c.ThingsPassword != "" }
func (c *Config) UptimeEnabled() bool  { return c.UptimeBase != "" }
func (c *Config) NewsEnabled() bool    { return len(c.NewsFeeds) > 0 }

// BriefingEnabled: with a Gemini key (or BRIEFING=on for the rule-based card).
func (c *Config) BriefingEnabled() bool {
	return c.Briefing != "off" && (c.GeminiKey != "" || c.Briefing == "on")
}

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
