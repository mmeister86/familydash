package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindNightBG(t *testing.T) {
	dir := t.TempDir()
	jpeg := filepath.Join(dir, "bg.jpeg")
	jpg := filepath.Join(dir, "bg.jpg")
	txt := filepath.Join(dir, "bg.txt")
	empty := filepath.Join(dir, "empty.png")
	for _, f := range []string{jpg, txt} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// missing file, wrong extension and empty file are skipped
	if p, _ := findNightBG([]string{jpeg, txt, empty, jpg}); p != jpg {
		t.Fatalf("got %q, want %q", p, jpg)
	}
	if p, info := findNightBG([]string{jpeg, txt}); p != "" || info != nil {
		t.Fatalf("expected nothing, got %q", p)
	}
	if p, _ := findNightBG(nil); p != "" {
		t.Fatalf("expected nothing for no candidates, got %q", p)
	}
}
