package familyapp

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"familydash/internal/todo"
)

// TodosResponse is GET /todos?days=2 (see docs/FAMILY_APP.md).
type TodosResponse struct {
	Date   string       `json:"date"` // today in Europe/Berlin, YYYY-MM-DD
	People []WirePerson `json:"people"`
	Tasks  []WireTask   `json:"tasks"`
}

type WirePerson struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Role   string `json:"role"` // parent | child
	Color  string `json:"color"`
	Points int    `json:"points"`
}

type WireTask struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Assignee  string `json:"assignee,omitempty"` // slug, "" = whole family
	Date      string `json:"date,omitempty"`     // YYYY-MM-DD, "" = undated ("anytime")
	Status    string `json:"status"`             // open | pending | done | missed
	Points    int    `json:"points,omitempty"`
	Recurring bool   `json:"recurring,omitempty"`
}

// TodoService polls GET /todos and keeps the last good list, like every
// other source: a hiccup of the backend never blanks the wall.
type TodoService struct {
	Client *Client
	Loc    *time.Location

	mu   sync.RWMutex
	last *todo.List
	err  string
}

func NewTodoService(c *Client, loc *time.Location) *TodoService {
	return &TodoService{Client: c, Loc: loc}
}

func (s *TodoService) Run(ctx context.Context, every time.Duration) {
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

func (s *TodoService) Refresh(ctx context.Context) {
	var resp TodosResponse
	err := s.Client.get(ctx, "/todos?days=2", &resp)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if err.Error() != s.err {
			slog.Warn("familyapp: todos refresh failed", "err", err)
		}
		s.err = err.Error()
		return
	}
	s.last, s.err = Convert(resp, time.Now(), s.Loc), ""
}

// Snapshot returns the last good list plus the last error (nil if nothing yet).
func (s *TodoService) Snapshot() *todo.List {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.last == nil {
		if s.err == "" {
			return nil
		}
		return &todo.List{Name: listName, Source: todo.SourceFamilyApp, Tasks: []todo.Task{}, Error: s.err}
	}
	l := *s.last
	l.Error = s.err
	return &l
}

const listName = "Familienapp"

// Convert turns the backend's answer into the wall's list: today's tasks
// (plus overdue one-offs, open undated "anytime" tasks and what was done
// today), tomorrow's tasks for the evening outlook, and the people with
// their points. Missed recurring tasks and finished undated ones are left out.
func Convert(r TodosResponse, now time.Time, loc *time.Location) *todo.List {
	today := r.Date
	if today == "" {
		today = now.In(loc).Format("2006-01-02")
	}
	tomorrow := today
	if d, err := time.ParseInLocation("2006-01-02", today, loc); err == nil {
		tomorrow = d.AddDate(0, 0, 1).Format("2006-01-02")
	}

	names := map[string]string{}
	l := &todo.List{Name: listName, Source: todo.SourceFamilyApp, Tasks: []todo.Task{}, UpdatedAt: now}
	for _, p := range r.People {
		names[p.Slug] = p.Name
		l.People = append(l.People, todo.Person{Slug: p.Slug, Name: p.Name, Role: p.Role, Color: p.Color, Points: p.Points})
	}

	type ranked struct {
		t    todo.Task
		rank int // 0 overdue, 1 open, 2 waiting, 3 done
	}
	var day []ranked
	for _, w := range r.Tasks {
		if w.Status == "pending" {
			l.Pending++
		}
		title := strings.TrimSpace(w.Title)
		if title == "" || w.Status == "missed" {
			continue
		}
		t := todo.Task{ID: w.ID, Title: title, Who: names[w.Assignee], Points: w.Points,
			Done: w.Status == "done", Pending: w.Status == "pending"}
		if w.Date == "" {
			// Undated ("anytime") tasks stay on the wall until they are done.
			// Done ones are dropped: without a date we can't tell "done today".
			if w.Status == "open" || w.Status == "pending" {
				day = append(day, ranked{t, rankOf(t, false)})
			}
			continue
		}
		if !w.Recurring {
			t.Deadline = w.Date // one-offs get "heute fällig" / "überfällig" from the deadline
		}
		switch {
		case w.Date < today:
			if w.Status == "open" || w.Status == "pending" {
				day = append(day, ranked{t, rankOf(t, true)})
			}
		case w.Date == today:
			day = append(day, ranked{t, rankOf(t, false)})
		case w.Date == tomorrow:
			if !t.Done {
				l.Tomorrow = append(l.Tomorrow, t)
			}
		}
	}
	sort.SliceStable(day, func(i, j int) bool { return day[i].rank < day[j].rank })
	for _, d := range day {
		l.Tasks = append(l.Tasks, d.t)
	}
	return l
}

func rankOf(t todo.Task, overdue bool) int {
	switch {
	case t.Done:
		return 3
	case t.Pending:
		return 2
	case overdue:
		return 0
	}
	return 1
}
