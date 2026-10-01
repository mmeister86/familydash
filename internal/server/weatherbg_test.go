package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeatherBGLookup(t *testing.T) {
	dir := t.TempDir()
	touch := func(n string) { os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644) }

	if pickWeatherBG(dir, "rain", true) != nil {
		t.Fatal("empty dir should give nil")
	}
	touch("standard.jpg")
	touch("regen.jpg")
	touch("nacht.JPG")

	cases := []struct {
		icon string
		day  bool
		want string
	}{
		{"rain", true, "regen"},
		{"drizzle", true, "regen"},
		{"rain", false, "nacht"},  // no regen-nacht → nacht beats the daytime picture
		{"sun", true, "standard"}, // no klar.jpg
		{"unknown", true, "standard"},
	}
	for _, c := range cases {
		got := pickWeatherBG(dir, c.icon, c.day)
		if got == nil || got.Name != c.want {
			t.Errorf("%s day=%v: got %+v, want %s", c.icon, c.day, got, c.want)
		}
	}
	touch("regen-nacht.jpeg")
	if got := pickWeatherBG(dir, "rain", false); got == nil || got.Name != "regen-nacht" {
		t.Errorf("night variant: %+v", got)
	}
	if pickWeatherBG("", "rain", true) != nil {
		t.Error("off should give nil")
	}
}

func TestWeatherBGOnlyKnownNames(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "regen/../x", "foo", "nacht-nacht", ""} {
		if validWeatherBG(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, ok := range []string{"klar", "bewoelkt-nacht", "standard", "nacht"} {
		if !validWeatherBG(ok) || strings.Contains(ok, "/") {
			t.Errorf("%q rejected", ok)
		}
	}
}
