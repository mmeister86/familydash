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
