package besteschule

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Service struct {
	baseURL string
	token   string
	loc     *time.Location
	only    []string
	client  *http.Client

	mu     sync.RWMutex
	raw    *Raw
	update time.Time
	err    string
}

func NewService(baseURL, token string, loc *time.Location, only []string) *Service {
	return &Service{baseURL: baseURL, token: token, loc: loc, only: only,
		client: &http.Client{Timeout: 20 * time.Second}}
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
	raw, err := s.Fetch(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		slog.Warn("beste.schule refresh failed", "err", err)
		s.err = err.Error()
		return
	}
	s.raw, s.update, s.err = raw, time.Now(), ""
}

// Snapshot is computed from the cached responses on every call, so the
// timetable flips to the next school day without waiting for a refetch.
func (s *Service) Snapshot() *School {
	s.mu.RLock()
	raw, update, errMsg := s.raw, s.update, s.err
	s.mu.RUnlock()
	if raw == nil {
		if errMsg == "" {
			return nil
		}
		return &School{Students: []Student{}, Error: errMsg}
	}
	sc := Build(raw, time.Now(), s.loc, s.only)
	sc.UpdatedAt, sc.Error = update, errMsg
	return &sc
}

type authError struct{}

func (authError) Error() string {
	return "beste.schule lehnt den Token ab – neuen Personal Access Token erstellen"
}

// Fetch loads all raw responses. Only students and the timetable are
// required; the other routes are optional and simply stay empty on errors.
func (s *Service) Fetch(ctx context.Context) (*Raw, error) {
	now := time.Now().In(s.loc)
	day := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }
	raw := &Raw{Groups: map[string]any{}, Journal: map[string]any{}}

	var err error
	if raw.Students, err = s.get(ctx, "students", nil); err != nil {
		return nil, err
	}
	if raw.Timetable, err = s.get(ctx, "time-tables/current", url.Values{"include": {"lessons.times"}}); err != nil {
		if _, ok := err.(authError); ok {
			return nil, err
		}
		slog.Warn("beste.schule timetable", "err", err)
	}
	raw.Substitutions, err = s.get(ctx, "substitution-plans/days", url.Values{
		"include":       {"lessons,subject,teachers,rooms,notes"},
		"filter[range]": {day(-1) + "," + day(14)},
		"per_page":      {"250"},
	})
	if err != nil {
		slog.Warn("beste.schule substitutions", "err", err)
	}

	students := studentList(raw.Students)
	ids := []string{""}
	if len(students) > 0 {
		ids = ids[:0]
		for _, st := range students {
			ids = append(ids, idOf(st))
		}
	}
	for _, id := range ids {
		filter := url.Values{}
		if id != "" {
			filter.Set("filter[student]", id)
		}
		if len(students) > 1 {
			q := clone(filter)
			q.Set("include", "students")
			q.Set("per_page", "100")
			if g, err := s.get(ctx, "groups", q); err == nil {
				raw.Groups[id] = g
			}
		}
		q := clone(filter)
		q.Set("include", "notes.type")
		q.Set("filter[range]", day(0)+","+day(examDays))
		q.Set("per_page", "100")
		j, err := s.get(ctx, "journal/lessons", q)
		if err != nil {
			q = clone(filter)
			q.Set("include", "days,days.notes,days.notes.type,lessons,lessons.notes,lessons.notes.type,subject,room,teacher,group,time,notes,notes.type")
			j, err = s.get(ctx, "journal/weeks", q)
		}
		if err != nil {
			slog.Warn("beste.schule journal", "student", id, "err", err)
			continue
		}
		raw.Journal[id] = j
	}
	return raw, nil
}

func clone(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func (s *Service) get(ctx context.Context, route string, q url.Values) (any, error) {
	u := s.baseURL + "/" + route
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("beste.schule %s: %w", route, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, authError{}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("beste.schule %s: HTTP %d", route, resp.StatusCode)
	}
	var v any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&v); err != nil {
		return nil, fmt.Errorf("beste.schule %s: %w", route, err)
	}
	return v, nil
}
