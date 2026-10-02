// Package todo is the to-do list the wall shows, whatever it comes from:
// Things 3 (THINGS_*) or the family app (FAMILY_APP_*, TODOS_SOURCE).
package todo

import "time"

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

	// family app only
	Who     string `json:"who,omitempty"`     // name of the person it's for, "" = whole family
	Pending bool   `json:"pending,omitempty"` // a child ticked it off, a parent hasn't confirmed yet
	Points  int    `json:"points,omitempty"`
}

// Person is a family member of the family app with their points.
type Person struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Role   string `json:"role"` // parent | child
	Color  string `json:"color,omitempty"`
	Points int    `json:"points"`
}

type List struct {
	Name   string `json:"name"`
	Source string `json:"source"` // things | familyapp
	// today: open first (overdue first), then waiting for confirmation, then done
	Tasks []Task `json:"tasks"`
	// family app only: tomorrow's tasks (evening outlook), people and how many
	// ticked-off tasks wait for a parent
	Tomorrow  []Task    `json:"tomorrow,omitempty"`
	People    []Person  `json:"people,omitempty"`
	Pending   int       `json:"pending,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

// Source is anything that keeps a to-do list fresh.
type Source interface {
	Snapshot() *List
}

const (
	SourceThings    = "things"
	SourceFamilyApp = "familyapp"
)
