// Package reminders holds Apple Reminders pushed in from outside.
//
// Apple offers no server-side API for iCloud Reminders (they left CalDAV with
// the iOS 13 upgrade), so the dashboard can't pull them. Instead a device that
// *can* read them – a Mac running the EventKit bridge, or an iOS Shortcut –
// pushes a snapshot to POST /api/reminders. We persist the last snapshot per
// source to disk so restarts don't blank the display.
package reminders

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Item struct {
	ID        string     `json:"id,omitempty"`
	Title     string     `json:"title"`
	Due       *time.Time `json:"due,omitempty"`
	DueAllDay bool       `json:"dueAllDay,omitempty"`
	Priority  int        `json:"priority,omitempty"` // 1 high … 9 low, 0 none (EventKit semantics)
	Flagged   bool       `json:"flagged,omitempty"`
	Notes     string     `json:"notes,omitempty"`
}

// UnmarshalJSON also accepts a bare string ("Milch") – handy for Shortcuts.
func (i *Item) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		i.Title = s
		return nil
	}
	type alias Item
	return json.Unmarshal(b, (*alias)(i))
}

type List struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Items []Item `json:"items"`
}

// Push is the request body of POST /api/reminders. Either Lists (full
// snapshot for this source) or List + Items/Text (replace a single list).
type Push struct {
	Source string `json:"source,omitempty"`
	Lists  []List `json:"lists,omitempty"`

	List  string `json:"list,omitempty"`
	Color string `json:"color,omitempty"`
	Items []Item `json:"items,omitempty"`
	Text  string `json:"text,omitempty"` // newline-separated titles
}

type sourceState struct {
	Lists     map[string]List `json:"lists"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Store struct {
	path  string
	stale time.Duration

	mu      sync.RWMutex
	sources map[string]*sourceState
}

func NewStore(dataDir string, staleAfter time.Duration) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "reminders.json"), stale: staleAfter, sources: map[string]*sourceState{}}
	b, err := os.ReadFile(s.path)
	switch {
	case os.IsNotExist(err):
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.sources); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	return s, nil
}

func (s *Store) Apply(p Push, now time.Time) error {
	src := strings.TrimSpace(p.Source)
	if src == "" {
		src = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.sources[src]
	if st == nil {
		st = &sourceState{Lists: map[string]List{}}
		s.sources[src] = st
	}
	switch {
	case len(p.Lists) > 0:
		st.Lists = map[string]List{}
		for _, l := range p.Lists {
			st.Lists[l.Name] = clean(l)
		}
	case p.List != "":
		items := p.Items
		for _, line := range strings.Split(p.Text, "\n") {
			if t := strings.TrimSpace(line); t != "" {
				items = append(items, Item{Title: t})
			}
		}
		st.Lists[p.List] = clean(List{Name: p.List, Color: p.Color, Items: items})
	default:
		return fmt.Errorf(`body needs "lists" or "list"`)
	}
	st.UpdatedAt = now
	return s.save()
}

func clean(l List) List {
	out := l
	out.Items = nil
	for _, it := range l.Items {
		it.Title = strings.TrimSpace(it.Title)
		if it.Title != "" {
			out.Items = append(out.Items, it)
		}
	}
	return out
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.sources, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

type Snapshot struct {
	Lists     []List    `json:"lists"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	Stale     bool      `json:"stale"`
}

// Snapshot merges all sources. Lists with the same name are combined.
func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	merged := map[string]*List{}
	var newest time.Time
	for _, st := range s.sources {
		if st.UpdatedAt.After(newest) {
			newest = st.UpdatedAt
		}
		for name, l := range st.Lists {
			m := merged[name]
			if m == nil {
				m = &List{Name: name, Color: l.Color}
				merged[name] = m
			}
			m.Items = append(m.Items, l.Items...)
		}
	}
	snap := Snapshot{Lists: []List{}, UpdatedAt: newest}
	for _, l := range merged {
		sortItems(l.Items)
		snap.Lists = append(snap.Lists, *l)
	}
	sort.Slice(snap.Lists, func(i, j int) bool { return snap.Lists[i].Name < snap.Lists[j].Name })
	snap.Stale = !newest.IsZero() && now.Sub(newest) > s.stale
	return snap
}

// sortItems: overdue/dated first (earliest due), then flagged, priority, title.
func sortItems(items []Item) {
	prio := func(p int) int {
		if p == 0 {
			return 10
		}
		return p
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if (a.Due != nil) != (b.Due != nil) {
			return a.Due != nil
		}
		if a.Due != nil && !a.Due.Equal(*b.Due) {
			return a.Due.Before(*b.Due)
		}
		if a.Flagged != b.Flagged {
			return a.Flagged
		}
		if prio(a.Priority) != prio(b.Priority) {
			return prio(a.Priority) < prio(b.Priority)
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
}
