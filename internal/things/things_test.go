package things

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// today.json and done.json are real output of things3 v0.10.0 (`find --json`)
// for the CLI's own test journals; rich.json uses the same shape.
func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fake answers the two calls of fetch: the Today query and the done-today query.
func fake(t *testing.T, today, done []byte, calls *[][]string) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		*calls = append(*calls, args)
		if contains(args, "--completed-on") {
			return done, nil
		}
		return today, nil
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func titles(ts []Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Title
		if t.Done {
			out[i] += " ✓"
		}
	}
	return out
}

func TestRealOutput(t *testing.T) {
	var calls [][]string
	s := &Service{Area: "Familie", Timeout: 5e9, Exec: fake(t, read(t, "today.json"), read(t, "done.json"), &calls)}
	s.Refresh(context.Background())
	l := s.Snapshot()
	if l == nil || l.Error != "" {
		t.Fatalf("snapshot: %+v", l)
	}
	// CLI returns "Evening walk" first; Things shows This Evening below
	if got, want := titles(l.Tasks), []string{"Morning run", "Evening walk", "Done task ✓"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	if !l.Tasks[1].Evening || l.Tasks[0].Evening {
		t.Errorf("evening flags wrong: %+v", l.Tasks)
	}

	// only read-only commands, area passed through, second call without sync
	want := [][]string{
		{"--no-color", "--log-level", "error", "find", "--json", "--today", "--area", "Familie"},
		{"--no-color", "--log-level", "error", "--no-sync", "find", "--json", "--area", "Familie", "--completed-on", "=today"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}
}

func TestBuild(t *testing.T) {
	open, err := parse(read(t, "rich.json"))
	if err != nil {
		t.Fatal(err)
	}
	ts := build(open, nil)
	// projects and blank titles dropped; today order, evening last
	if got, want := titles(ts), []string{"Spülmaschine ausräumen", "Geschenk für Oma", "Zimmer aufräumen", "Wäsche aufhängen"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tasks = %q, want %q", got, want)
	}
	oma := ts[1]
	if oma.Project != "Geburtstag" || oma.Deadline != "2026-10-01" {
		t.Errorf("project/deadline: %+v", oma)
	}
	if c := ts[2].Checklist; c == nil || c.Done != 2 || c.Total != 3 {
		t.Errorf("checklist = %+v, want 2/3 (canceled counts as done)", c)
	}
	if !reflect.DeepEqual(ts[0].Tags, []string{"Lukas"}) {
		t.Errorf("tags = %q", ts[0].Tags)
	}
}

func TestErrorKeepsLastList(t *testing.T) {
	fail := false
	var calls [][]string
	ok := fake(t, read(t, "today.json"), []byte("[]"), &calls)
	s := &Service{Area: "Familie", Timeout: 5e9, Exec: func(ctx context.Context, args ...string) ([]byte, error) {
		if fail {
			return nil, errors.New("login failed")
		}
		return ok(ctx, args...)
	}}

	s.Refresh(context.Background())
	fail = true
	s.Refresh(context.Background())
	l := s.Snapshot()
	if l.Error != "login failed" || len(l.Tasks) != 2 {
		t.Errorf("want last list + error, got %+v", l)
	}

	// nothing good yet: empty list with the error
	s2 := &Service{Area: "Familie", Timeout: 5e9, Exec: func(context.Context, ...string) ([]byte, error) { return nil, errors.New("boom") }}
	s2.Refresh(context.Background())
	if l := s2.Snapshot(); l == nil || l.Error != "boom" || l.Tasks == nil || len(l.Tasks) != 0 {
		t.Errorf("got %+v", l)
	}
	if (&Service{}).Snapshot() != nil {
		t.Error("no refresh yet → nil")
	}
}

func TestGarbageOutput(t *testing.T) {
	// e.g. the CLI printing a usage error to stdout with exit code 0
	var calls [][]string
	s := &Service{Area: "Familie", Timeout: 5e9, Exec: fake(t, []byte("Invalid date expression"), nil, &calls)}
	s.Refresh(context.Background())
	if l := s.Snapshot(); l == nil || !strings.Contains(l.Error, "unerwartete Antwort") {
		t.Errorf("got %+v", l)
	}
}

// The real runner: a fake CLI script checks what it gets from us.
func TestExecRunner(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "things3")
	state := filepath.Join(dir, "state")
	script := `#!/bin/sh
[ "$THINGS3_EMAIL" = "me@example.com" ] || { echo "Error: wrong email" >&2; exit 1; }
[ "$THINGS3_PASSWORD" = "geheim" ] || { echo "Error: wrong password" >&2; exit 1; }
[ "$XDG_STATE_HOME" = "` + state + `" ] || { echo "Error: wrong state dir $XDG_STATE_HOME" >&2; exit 1; }
[ -z "$BRING_PASSWORD" ] || { echo "Error: leaked env" >&2; exit 1; }
[ "$(pwd)" = "` + state + `" ] || { echo "Error: wrong cwd" >&2; exit 1; }
case "$*" in
  *--completed-on*) echo '[]' ;;
  *) echo '[{"id":"x","title":"Müll raus","status":"incomplete","type":"todo","start":{"evening":false},"tags":[],"dates":{},"checklist":[],"indexes":{"sort_index":1,"today_sort_index":0}}]' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BRING_PASSWORD", "must-not-leak")

	s := NewService(bin, "me@example.com", "geheim", "Familie", state)
	s.Refresh(context.Background())
	l := s.Snapshot()
	if l == nil || l.Error != "" || len(l.Tasks) != 1 || l.Tasks[0].Title != "Müll raus" {
		t.Fatalf("got %+v", l)
	}
	if fi, err := os.Stat(state); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("state dir: %v %v", fi, err)
	}

	// stderr's first line becomes the error, without "Error: "
	s = NewService(bin, "me@example.com", "falsch", "Familie", state)
	s.Refresh(context.Background())
	if l := s.Snapshot(); l.Error != "wrong password" {
		t.Errorf("error = %q", l.Error)
	}

	// /data not mounted → falls back to a temp folder instead of failing
	t.Setenv("TMPDIR", filepath.Join(dir, "tmp"))
	os.WriteFile(filepath.Join(dir, "blocker"), nil, 0o644)
	fallback := filepath.Join(dir, "tmp", "things3")
	os.WriteFile(bin, []byte(strings.ReplaceAll(script, state, fallback)), 0o755)
	s = NewService(bin, "me@example.com", "geheim", "Familie", filepath.Join(dir, "blocker", "things"))
	s.Refresh(context.Background())
	if l := s.Snapshot(); l.Error != "" || len(l.Tasks) != 1 {
		t.Errorf("fallback: %+v", l)
	}

	s = NewService(filepath.Join(dir, "missing"), "a", "b", "Familie", state)
	s.Refresh(context.Background())
	if l := s.Snapshot(); !strings.Contains(l.Error, "things3-CLI fehlt") {
		t.Errorf("error = %q", l.Error)
	}
}
