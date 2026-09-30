package config

import "testing"

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
