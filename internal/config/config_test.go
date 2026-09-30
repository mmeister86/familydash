package config

import (
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
