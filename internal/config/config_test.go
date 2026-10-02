package config

import (
	"strings"
	"testing"

	"familydash/internal/scene"
)

func TestEnvStripsQuotes(t *testing.T) {
	t.Setenv("A", `"https://example.com/a.ics"`)
	t.Setenv("B", `'Kinder/Schule'`)
	t.Setenv("C", `  plain  `)
	t.Setenv("D", `"unbalanced`)
	for key, want := range map[string]string{"A": "https://example.com/a.ics", "B": "Kinder/Schule", "C": "plain", "D": `"unbalanced`} {
		if got := env(key, ""); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

func TestCalendarColumn(t *testing.T) {
	t.Setenv("CALENDAR_1_URL", "https://example.com/familie.ics")
	t.Setenv("CALENDAR_2_URL", "https://example.com/dienst.ics")
	t.Setenv("CALENDAR_4_URL", "https://example.com/schule.ics")
	t.Setenv("CALENDAR_4_PANEL", "school")
	t.Setenv("CALENDAR_5_URL", "https://example.com/feiertage.ics")
	t.Setenv("CALENDAR_5_COLUMN", "1") // into Familie
	t.Setenv("CALENDAR_6_URL", "https://example.com/x.ics")
	t.Setenv("CALENDAR_6_COLUMN", "4") // target is a school card → ignored
	t.Setenv("CALENDAR_7_URL", "https://example.com/y.ics")
	t.Setenv("CALENDAR_7_COLUMN", "5") // target is merged itself → ignored
	t.Setenv("CALENDAR_8_URL", "https://example.com/z.ics")
	t.Setenv("CALENDAR_8_COLUMN", "9") // missing target → ignored

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]int{1: -1, 2: -1, 4: -1, 5: 0, 6: -1, 7: -1, 8: -1}
	for _, cal := range c.Calendars {
		if cal.Into != want[cal.Num] {
			t.Errorf("CALENDAR_%d: Into = %d, want %d", cal.Num, cal.Into, want[cal.Num])
		}
	}
}

func TestScenes(t *testing.T) {
	t.Setenv("SCENE_EVENING", "18:30")
	t.Setenv("SCENE_DAY_WEEKEND", "off")
	t.Setenv("SCENE_FORCE", "Night")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	find := func(list []scene.Start, name string) int {
		for _, s := range list {
			if s.Name == name {
				return s.Minute
			}
		}
		return -1
	}
	if m := find(c.Scenes.SchoolDay, scene.Evening); m != 18*60+30 {
		t.Errorf("school-day evening = %d", m)
	}
	if m := find(c.Scenes.Weekend, scene.Evening); m != 18*60+30 {
		t.Errorf("weekend evening should fall back to SCENE_EVENING, got %d", m)
	}
	if m := find(c.Scenes.Weekend, scene.Day); m != -1 {
		t.Errorf("weekend day should be off, got %d", m)
	}
	if m := find(c.Scenes.Weekend, scene.Morning); m != 7*60+30 {
		t.Errorf("weekend morning default = %d", m)
	}
	if c.Scenes.Force != scene.Night {
		t.Errorf("force = %q", c.Scenes.Force)
	}

	t.Setenv("SCENE_FORCE", "brunch")
	if _, err := Load(); err == nil {
		t.Error("SCENE_FORCE=brunch: want error")
	}
}

func TestPhotosDir(t *testing.T) {
	c, _ := Load()
	if c.PhotosDir != "/data/pictures" {
		t.Errorf("default PHOTOS_DIR = %q", c.PhotosDir)
	}
	t.Setenv("PHOTOS_DIR", "off")
	if c, _ := Load(); c.PhotosDir != "" {
		t.Errorf("PHOTOS_DIR=off → %q", c.PhotosDir)
	}
}

func TestUptime(t *testing.T) {
	c, err := Load()
	if err != nil || c.UptimeEnabled() {
		t.Fatalf("uptime should be off without UPTIME_URL: %v", err)
	}

	t.Setenv("UPTIME_URL", "http://192.168.188.127:3001/status/dashboard")
	t.Setenv("UPTIME_API_KEY", `"uk1_secret"`)
	if c, err = Load(); err != nil {
		t.Fatal(err)
	}
	if !c.UptimeEnabled() || c.UptimeBase != "http://192.168.188.127:3001" || c.UptimeSlug != "dashboard" ||
		c.UptimeAPIKey != "uk1_secret" || c.UptimeCertWarn != 14 || c.UptimeRefresh.Minutes() != 1 {
		t.Errorf("uptime config: %+v", c)
	}

	t.Setenv("UPTIME_URL", "http://192.168.188.127:3001")
	if _, err := Load(); err == nil {
		t.Error("URL without /status/<slug> should fail")
	}
}

func TestNewsFeeds(t *testing.T) {
	t.Setenv("NEWS_LOCAL", "Crimmitschau, Landkreis Zwickau")
	t.Setenv("NEWS_1_URL", "https://www.tagesschau.de/index~rss2.xml")
	t.Setenv("NEWS_1_NAME", "Tagesschau")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var groups []string
	for _, f := range c.NewsFeeds {
		groups = append(groups, f.Group)
	}
	if got := strings.Join(groups, ","); got != "Region,Region,Deutschland,Tagesschau" {
		t.Errorf("groups %q", got)
	}

	t.Setenv("NEWS", "off")
	if c, _ = Load(); c.NewsEnabled() {
		t.Error("NEWS=off still has feeds")
	}
}

func TestFamilyApp(t *testing.T) {
	t.Setenv("CALENDAR_1_URL", "https://example.com/familie.ics")
	t.Setenv("CALENDAR_3_URL", "https://example.com/sport.ics")
	t.Setenv("FAMILY_APP_SITE_URL", "https://familybackend-http.matthias.lol/")
	t.Setenv("FAMILY_APP_INGEST_TOKEN", "w")
	t.Setenv("FAMILY_APP_LUKAS_CALENDARS", "1, 3")
	t.Setenv("THINGS_EMAIL", "a@b.c")
	t.Setenv("THINGS_PASSWORD", "x")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.FamilyAppURL != "https://familybackend-http.matthias.lol" || !c.FamilyAppPushEnabled() {
		t.Errorf("url/push: %q %v", c.FamilyAppURL, c.FamilyAppPushEnabled())
	}
	if got := c.FamilyAppCalendars["lukas"]; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("calendars: %v", c.FamilyAppCalendars)
	}
	// auto without a read token: Things stays
	if c.FamilyAppTodos() || !c.UseThings() {
		t.Error("auto without FAMILY_APP_DASHBOARD_TOKEN should keep Things")
	}
	t.Setenv("FAMILY_APP_DASHBOARD_TOKEN", "r")
	if c, _ = Load(); !c.FamilyAppTodos() || c.UseThings() {
		t.Error("auto with a read token should switch to the app")
	}
	t.Setenv("TODOS_SOURCE", "things")
	if c, _ = Load(); c.FamilyAppTodos() || !c.UseThings() {
		t.Error("TODOS_SOURCE=things should keep Things")
	}
}

// A mistake in the optional family app settings must never stop the wall:
// Load succeeds, the feature is fixed or switched off, and there's a warning.
func TestFamilyAppMistakesAreNotFatal(t *testing.T) {
	cases := map[string]struct {
		env      map[string]string
		url      string
		push     bool
		appTodos bool
		warnings int
	}{
		"no scheme is fixed": {env: map[string]string{"FAMILY_APP_SITE_URL": "familybackend-http.matthias.lol", "FAMILY_APP_INGEST_TOKEN": "w"},
			url: "https://familybackend-http.matthias.lol", push: true},
		"garbage url":  {env: map[string]string{"FAMILY_APP_SITE_URL": "ftp://x", "FAMILY_APP_INGEST_TOKEN": "w"}, warnings: 1},
		"spaces":       {env: map[string]string{"FAMILY_APP_SITE_URL": "https://a b.de", "FAMILY_APP_DASHBOARD_TOKEN": "r"}, warnings: 1},
		"token no url": {env: map[string]string{"FAMILY_APP_INGEST_TOKEN": "w"}, warnings: 1},
		"bad source":   {env: map[string]string{"TODOS_SOURCE": "nope"}, warnings: 1},
		"familyapp without token": {env: map[string]string{"TODOS_SOURCE": "familyapp", "FAMILY_APP_SITE_URL": "https://x.y"},
			url: "https://x.y", warnings: 1},
		"calendars": {env: map[string]string{"FAMILY_APP_SITE_URL": "https://x.y", "FAMILY_APP_DASHBOARD_TOKEN": "r", "FAMILY_APP_HANNAH_CALENDARS": "7,abc"},
			url: "https://x.y", appTodos: true, warnings: 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			c, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			if c.FamilyAppURL != tc.url || c.FamilyAppPushEnabled() != tc.push || c.FamilyAppTodos() != tc.appTodos || len(c.Warnings) != tc.warnings {
				t.Errorf("url=%q push=%v appTodos=%v warnings=%q", c.FamilyAppURL, c.FamilyAppPushEnabled(), c.FamilyAppTodos(), c.Warnings)
			}
		})
	}
}
