// Package things reads today's to-dos of one area from Things 3.
//
// Things has no public API. The image ships the things3 CLI
// (https://github.com/evanpurkhiser/things3-cloud), which talks to Things
// Cloud the same way the apps do, as reverse-engineered by its author. It can
// break whenever Cultured Code changes something; failures only affect this
// card.
//
// familydash only ever runs fixed, read-only CLI commands (find --json). It
// never marks, edits or deletes anything, and it does not use the CLI's
// built-in webserver, which would accept any command from the network.
package things

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const DefaultBin = "/things3"

type Checklist struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type Task struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Project   string     `json:"project,omitempty"`
	Evening   bool       `json:"evening,omitempty"`
	Deadline  string     `json:"deadline,omitempty"` // YYYY-MM-DD
	Done      bool       `json:"done,omitempty"`     // completed today
	Checklist *Checklist `json:"checklist,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
}

type List struct {
	Name      string    `json:"name"`
	Tasks     []Task    `json:"tasks"` // open first (Things' Today order, evening last), then done today
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

// Runner executes the CLI with the given arguments and returns stdout.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

type Service struct {
	Area    string
	Timeout time.Duration
	Exec    Runner

	mu   sync.RWMutex
	last *List
	err  string
}

// NewService runs the real CLI at bin. Login, cache folder and time zone are
// passed explicitly; the child doesn't see the rest of the container's
// environment (other services' passwords).
func NewService(bin, email, password, area, stateDir string) *Service {
	env := []string{
		"THINGS3_EMAIL=" + email,
		"THINGS3_PASSWORD=" + password,
		"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
	}
	for _, k := range []string{"TZ", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy", "PATH"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return &Service{Area: area, Timeout: 2 * time.Minute, Exec: execRunner(bin, stateDir, env)}
}

// cacheDir returns the CLI's state folder. Without a writable /data mount it
// falls back to a temp folder: works the same, only syncs from scratch after
// a container restart.
func cacheDir(want string) (string, error) {
	if err := os.MkdirAll(want, 0o700); err == nil {
		return want, nil
	} else {
		tmp := filepath.Join(os.TempDir(), "things3")
		if err2 := os.MkdirAll(tmp, 0o700); err2 != nil {
			return "", fmt.Errorf("Cache-Ordner %s: %w", want, err)
		}
		return tmp, nil
	}
}

func execRunner(bin, stateDir string, base []string) Runner {
	var warned bool
	return func(ctx context.Context, args ...string) ([]byte, error) {
		dir, err := cacheDir(stateDir)
		if err != nil {
			return nil, err
		}
		if dir != stateDir && !warned {
			slog.Warn("things: cache folder not writable, using a temp folder", "want", stateDir, "using", dir)
			warned = true
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(base[:len(base):len(base)], "XDG_STATE_HOME="+dir, "HOME="+dir)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("things3-CLI fehlt (%s)", bin)
			}
			if ctx.Err() != nil {
				return nil, fmt.Errorf("Things Cloud antwortet nicht (%v)", ctx.Err())
			}
			if msg := firstLine(stderr.String()); msg != "" {
				return nil, errors.New(msg)
			}
			return nil, err
		}
		return stdout.Bytes(), nil
	}
}

func (s *Service) Run(ctx context.Context, every time.Duration) {
	s.Refresh(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Refresh(ctx)
		}
	}
}

func (s *Service) Refresh(ctx context.Context) {
	l, err := s.fetch(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		slog.Warn("things refresh failed", "err", err)
		s.err = err.Error()
		return
	}
	s.last, s.err = l, ""
}

// Snapshot returns the last good list plus the last error (nil if nothing yet).
func (s *Service) Snapshot() *List {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.last == nil {
		if s.err == "" {
			return nil
		}
		return &List{Name: s.Area, Tasks: []Task{}, Error: s.err}
	}
	l := *s.last
	l.Error = s.err
	return &l
}

func (s *Service) fetch(ctx context.Context) (*List, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	// 1) sync with Things Cloud and read what's in Today for the area
	out, err := s.Exec(ctx, "--no-color", "--log-level", "error", "find", "--json", "--today", "--area", s.Area)
	if err != nil {
		return nil, err
	}
	open, err := parse(out)
	if err != nil {
		return nil, err
	}
	// 2) what was ticked off today – from the local cache, no second sync
	out, err = s.Exec(ctx, "--no-color", "--log-level", "error", "--no-sync", "find", "--json", "--area", s.Area, "--completed-on", "=today")
	if err != nil {
		return nil, err
	}
	done, err := parse(out)
	if err != nil {
		return nil, err
	}
	return &List{Name: s.Area, Tasks: build(open, done), UpdatedAt: time.Now()}, nil
}

// wire format of `things3 find --json` (only the fields we use)
type wireTask struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"` // incomplete, completed, canceled
	Type    string `json:"type"`   // todo, project
	Project *struct {
		Title string `json:"title"`
	} `json:"project"`
	Tags []struct {
		Title string `json:"title"`
	} `json:"tags"`
	Start struct {
		Evening bool `json:"evening"`
	} `json:"start"`
	Dates struct {
		Deadline  *string `json:"deadline_at"`
		Completed *string `json:"completed_at"`
	} `json:"dates"`
	Checklist []struct {
		Status string `json:"status"`
	} `json:"checklist"`
	Indexes struct {
		Sort  int `json:"sort_index"`
		Today int `json:"today_sort_index"`
	} `json:"indexes"`
}

func parse(b []byte) ([]wireTask, error) {
	var ts []wireTask
	if err := json.Unmarshal(bytes.TrimSpace(b), &ts); err != nil {
		return nil, fmt.Errorf("unerwartete Antwort der things3-CLI: %w", err)
	}
	return ts, nil
}

func build(open, done []wireTask) []Task {
	open = todos(open, "incomplete")
	done = todos(done, "completed")
	// Things' Today view: normal list first, "This Evening" below
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		if a.Start.Evening != b.Start.Evening {
			return !a.Start.Evening
		}
		if a.Indexes.Today != b.Indexes.Today {
			return a.Indexes.Today < b.Indexes.Today
		}
		return a.Indexes.Sort < b.Indexes.Sort
	})
	// done: most recently completed first
	sort.SliceStable(done, func(i, j int) bool { return deref(done[i].Dates.Completed) > deref(done[j].Dates.Completed) })

	out := make([]Task, 0, len(open)+len(done))
	for _, w := range open {
		out = append(out, convert(w, false))
	}
	for _, w := range done {
		out = append(out, convert(w, true))
	}
	return out
}

func todos(ts []wireTask, status string) []wireTask {
	var out []wireTask
	for _, t := range ts {
		if t.Type == "todo" && t.Status == status && strings.TrimSpace(t.Title) != "" {
			out = append(out, t)
		}
	}
	return out
}

func convert(w wireTask, done bool) Task {
	t := Task{ID: w.ID, Title: strings.TrimSpace(w.Title), Evening: w.Start.Evening, Done: done}
	if w.Project != nil {
		t.Project = w.Project.Title
	}
	if d := deref(w.Dates.Deadline); len(d) >= 10 {
		t.Deadline = d[:10]
	}
	for _, tg := range w.Tags {
		t.Tags = append(t.Tags, tg.Title)
	}
	if n := len(w.Checklist); n > 0 {
		c := &Checklist{Total: n}
		for _, it := range w.Checklist {
			if it.Status == "completed" || it.Status == "canceled" {
				c.Done++
			}
		}
		t.Checklist = c
	}
	return t
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			l = strings.TrimPrefix(l, "Error: ")
			if len(l) > 200 {
				l = l[:200] + "…"
			}
			return l
		}
	}
	return ""
}
