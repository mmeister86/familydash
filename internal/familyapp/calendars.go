// Central calendar feed: in CALENDAR_SOURCE=convex mode the wall reads all
// calendars exclusively from the Companion backend (GET
// /dashboard/calendars) and keeps the last accepted snapshot in RAM and on
// disk. Only fully validated responses replace last-good data; failures
// never blank the display and never start local ICS polling.
package familyapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"familydash/internal/calendar"
)

// CalendarFeedVersion, CalendarFeedScope and CalendarFeedTimezone pin the
// HTTP read contract v1 shared with the Companion backend.
const (
	CalendarFeedVersion  = 1
	CalendarFeedScope    = "family-calendars"
	CalendarFeedTimezone = "Europe/Berlin"
	maxCalendarFeedBytes = 4 << 20
	calendarCacheScope   = "family-calendars"
	calendarCacheVersion = 1
)

// ConvexPollInterval is the Go poll interval in convex mode; the provider
// intervals stay centrally configured.
const ConvexPollInterval = time.Minute

// ---------------------------------------------------------------- wire types

// The wire structs mirror CalendarFeedV1. Slices and the window are pointers
// so presence-aware validation can tell an explicitly empty array from a
// missing one; unknown additive fields decode-ignored.
type feedWindow struct {
	FromDate string `json:"fromDate"`
	ToDate   string `json:"toDate"`
	FromMs   int64  `json:"fromMs"`
	ToMs     int64  `json:"toMs"`
}

type feedPerson struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type feedBinding struct {
	PersonID   string `json:"personId"`
	Kind       string `json:"kind"`
	ExternalID string `json:"externalId"`
}

type feedCalendar struct {
	ID                string      `json:"id"`
	SourceKey         string      `json:"sourceKey"`
	Name              string      `json:"name"`
	Color             string      `json:"color"`
	Panel             string      `json:"panel"`
	Order             int         `json:"order"`
	IntoCalendarID    *string     `json:"intoCalendarId"`
	PersonIDs         []string    `json:"personIds"`
	LastAttemptAt     *int64      `json:"lastAttemptAt"`
	LastSuccessAt     *int64      `json:"lastSuccessAt"`
	Freshness         string      `json:"freshness"`
	LastAttemptStatus *string     `json:"lastAttemptStatus"`
	Error             string      `json:"error"`
	Coverage          *feedWindow `json:"coverage"`
}

type feedEvent struct {
	Key             string  `json:"key"`
	UID             string  `json:"uid"`
	IdentityQuality string  `json:"identityQuality"`
	RecurrenceID    *string `json:"recurrenceId"`
	Title           string  `json:"title"`
	Location        string  `json:"location"`
	StartMs         int64   `json:"startMs"`
	EndMs           int64   `json:"endMs"`
	AllDay          bool    `json:"allDay"`
	Timezone        string  `json:"timezone"`
	StartDate       *string `json:"startDate"`
	EndDate         *string `json:"endDate"`
	CalendarID      string  `json:"calendarId"`
}

type calendarFeed struct {
	Version               *int            `json:"version"`
	Scope                 *string         `json:"scope"`
	Timezone              *string         `json:"timezone"`
	ConfigurationRevision *int64          `json:"configurationRevision"`
	GeneratedAt           *int64          `json:"generatedAt"`
	Window                *feedWindow     `json:"window"`
	People                *[]feedPerson   `json:"people"`
	Bindings              *[]feedBinding  `json:"bindings"`
	Calendars             *[]feedCalendar `json:"calendars"`
	Events                *[]feedEvent    `json:"events"`
}

// ------------------------------------------------------------- validation

func feedErr(format string, a ...any) error {
	return fmt.Errorf("calendar feed: "+format, a...)
}

// ParseCalendarFeed strictly validates a v1 feed body and converts it into a
// central snapshot. Anything unexpected (missing version/arrays, wrong
// scope/timezone, broken references, duplicate identities, bad panels or
// column links, oversize) is an error; permitted additive fields are
// ignored. loc is the fallback location if the feed timezone cannot be
// loaded; the feed timezone itself always governs calendar-day logic.
func ParseCalendarFeed(data []byte, loc *time.Location) (calendar.Snapshot, error) {
	if len(data) == 0 {
		return calendar.Snapshot{}, feedErr("empty body")
	}
	if len(data) > maxCalendarFeedBytes {
		return calendar.Snapshot{}, feedErr("body exceeds 4 MiB (%d bytes)", len(data))
	}
	var f calendarFeed
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return calendar.Snapshot{}, feedErr("invalid JSON: %v", err)
	}
	if f.Version == nil || f.Scope == nil || f.Timezone == nil ||
		f.ConfigurationRevision == nil || f.GeneratedAt == nil || f.Window == nil ||
		f.People == nil || f.Bindings == nil || f.Calendars == nil || f.Events == nil {
		return calendar.Snapshot{}, feedErr("missing required v1 field (version/scope/timezone/revision/generatedAt/window/people/bindings/calendars/events)")
	}
	if *f.Version != CalendarFeedVersion {
		return calendar.Snapshot{}, feedErr("version %d, want %d", *f.Version, CalendarFeedVersion)
	}
	if *f.Scope != CalendarFeedScope {
		return calendar.Snapshot{}, feedErr("scope %q, want %q", *f.Scope, CalendarFeedScope)
	}
	if *f.Timezone != CalendarFeedTimezone {
		return calendar.Snapshot{}, feedErr("timezone %q, want %q", *f.Timezone, CalendarFeedTimezone)
	}
	feedLoc, err := time.LoadLocation(*f.Timezone)
	if err != nil {
		if loc == nil {
			return calendar.Snapshot{}, feedErr("unknown timezone %q", *f.Timezone)
		}
		feedLoc = loc
	}
	winStart, winEnd, err := checkWindow(*f.Window, feedLoc, "window")
	if err != nil {
		return calendar.Snapshot{}, err
	}

	people := make([]calendar.Person, 0, len(*f.People))
	personIDs := map[string]bool{}
	slugs := map[string]bool{}
	for i, p := range *f.People {
		if p.ID == "" || p.Slug == "" || p.Name == "" {
			return calendar.Snapshot{}, feedErr("people[%d]: id/slug/name required", i)
		}
		if p.Role != "parent" && p.Role != "child" {
			return calendar.Snapshot{}, feedErr("people[%d]: role %q, want parent|child", i, p.Role)
		}
		if personIDs[p.ID] {
			return calendar.Snapshot{}, feedErr("duplicate person id %q", p.ID)
		}
		if slugs[p.Slug] {
			return calendar.Snapshot{}, feedErr("duplicate person slug %q", p.Slug)
		}
		personIDs[p.ID] = true
		slugs[p.Slug] = true
		people = append(people, calendar.Person{ID: p.ID, Slug: p.Slug, Name: p.Name, Role: p.Role})
	}

	bindings := make([]calendar.PersonBinding, 0, len(*f.Bindings))
	seenBinding := map[string]bool{}
	for i, b := range *f.Bindings {
		if b.ExternalID == "" {
			return calendar.Snapshot{}, feedErr("bindings[%d]: externalId required", i)
		}
		if b.Kind != "besteschule" && b.Kind != "timetable" {
			return calendar.Snapshot{}, feedErr("bindings[%d]: kind %q, want besteschule|timetable", i, b.Kind)
		}
		if !personIDs[b.PersonID] {
			return calendar.Snapshot{}, feedErr("bindings[%d]: unknown personId %q", i, b.PersonID)
		}
		k := b.Kind + "\x00" + b.ExternalID
		if seenBinding[k] {
			return calendar.Snapshot{}, feedErr("duplicate binding %q/%q", b.Kind, b.ExternalID)
		}
		seenBinding[k] = true
		bindings = append(bindings, calendar.PersonBinding{PersonID: b.PersonID, Kind: b.Kind, ExternalID: b.ExternalID})
	}

	cals := *f.Calendars
	byID := map[string]int{}
	byKey := map[string]bool{}
	for i, c := range cals {
		if c.ID == "" || c.SourceKey == "" || c.Name == "" || c.Color == "" {
			return calendar.Snapshot{}, feedErr("calendars[%d]: id/sourceKey/name/color required", i)
		}
		if c.Panel != "column" && c.Panel != "school" {
			return calendar.Snapshot{}, feedErr("calendars[%d]: panel %q, want column|school", i, c.Panel)
		}
		if byID[c.ID] != 0 {
			return calendar.Snapshot{}, feedErr("duplicate calendar id %q", c.ID)
		}
		byID[c.ID] = i + 1
		if byKey[c.SourceKey] {
			return calendar.Snapshot{}, feedErr("duplicate calendar sourceKey %q", c.SourceKey)
		}
		byKey[c.SourceKey] = true
		switch c.Freshness {
		case "neverLoaded", "fresh", "stale", "disabled":
		default:
			return calendar.Snapshot{}, feedErr("calendars[%d]: freshness %q unknown", i, c.Freshness)
		}
		if c.LastAttemptStatus != nil && *c.LastAttemptStatus != "success" && *c.LastAttemptStatus != "error" {
			return calendar.Snapshot{}, feedErr("calendars[%d]: lastAttemptStatus %q unknown", i, *c.LastAttemptStatus)
		}
		seen := map[string]bool{}
		for _, pid := range c.PersonIDs {
			if !personIDs[pid] {
				return calendar.Snapshot{}, feedErr("calendars[%d]: unknown personId %q", i, pid)
			}
			if seen[pid] {
				return calendar.Snapshot{}, feedErr("calendars[%d]: duplicate personId %q", i, pid)
			}
			seen[pid] = true
		}
		if c.Coverage != nil {
			if _, _, err := checkWindow(*c.Coverage, feedLoc, fmt.Sprintf("calendars[%d].coverage", i)); err != nil {
				return calendar.Snapshot{}, err
			}
		}
	}
	// Column links: every intoCalendarId must point at a real root column.
	// Dangling targets, self links, chains and cycles all fail this single
	// check: a cycle member always points at another linked calendar, never
	// at a root.
	into := map[string]string{}
	for _, c := range cals {
		if c.IntoCalendarID != nil && *c.IntoCalendarID != "" {
			into[c.ID] = *c.IntoCalendarID
		}
	}
	for id, target := range into {
		if _, ok := byID[target]; !ok {
			return calendar.Snapshot{}, feedErr("calendar %q: unknown intoCalendarId %q", id, target)
		}
		if _, linked := into[target]; linked || target == id {
			return calendar.Snapshot{}, feedErr("calendar %q: intoCalendarId %q is not a root column", id, target)
		}
	}

	seenEvent := map[string]bool{}
	for i, e := range *f.Events {
		if e.Key == "" || e.UID == "" {
			return calendar.Snapshot{}, feedErr("events[%d]: key/uid required", i)
		}
		if e.IdentityQuality != "provider" && e.IdentityQuality != "fallback" {
			return calendar.Snapshot{}, feedErr("events[%d]: identityQuality %q unknown", i, e.IdentityQuality)
		}
		if e.Timezone == "" {
			return calendar.Snapshot{}, feedErr("events[%d]: timezone required", i)
		}
		if e.EndMs < e.StartMs {
			return calendar.Snapshot{}, feedErr("events[%d]: end before start", i)
		}
		if _, ok := byID[e.CalendarID]; !ok {
			return calendar.Snapshot{}, feedErr("events[%d]: unknown calendarId %q", i, e.CalendarID)
		}
		k := e.CalendarID + "\x00" + e.Key
		if seenEvent[k] {
			return calendar.Snapshot{}, feedErr("duplicate event key %q in calendar %q", e.Key, e.CalendarID)
		}
		seenEvent[k] = true
		if e.AllDay {
			if e.StartDate == nil || e.EndDate == nil || *e.StartDate == "" || *e.EndDate == "" {
				return calendar.Snapshot{}, feedErr("events[%d]: all-day event needs startDate/endDate", i)
			}
			sd, err := time.ParseInLocation("2006-01-02", *e.StartDate, feedLoc)
			if err != nil {
				return calendar.Snapshot{}, feedErr("events[%d]: bad startDate: %v", i, err)
			}
			ed, err := time.ParseInLocation("2006-01-02", *e.EndDate, feedLoc)
			if err != nil {
				return calendar.Snapshot{}, feedErr("events[%d]: bad endDate: %v", i, err)
			}
			if !ed.After(sd) {
				return calendar.Snapshot{}, feedErr("events[%d]: endDate not after startDate", i)
			}
			if msMidnight(e.StartMs, feedLoc) != *e.StartDate || msMidnight(e.EndMs, feedLoc) != *e.EndDate {
				return calendar.Snapshot{}, feedErr("events[%d]: all-day ms do not match dates", i)
			}
		}
	}

	return convertFeed(f, feedLoc, winStart, winEnd, people, bindings), nil
}

func checkWindow(w feedWindow, loc *time.Location, what string) (time.Time, time.Time, error) {
	fd, err := time.ParseInLocation("2006-01-02", w.FromDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, feedErr("%s: bad fromDate: %v", what, err)
	}
	td, err := time.ParseInLocation("2006-01-02", w.ToDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, feedErr("%s: bad toDate: %v", what, err)
	}
	if !td.After(fd) {
		return time.Time{}, time.Time{}, feedErr("%s: toDate not after fromDate", what)
	}
	if w.FromMs >= w.ToMs {
		return time.Time{}, time.Time{}, feedErr("%s: toMs not after fromMs", what)
	}
	if msMidnight(w.FromMs, loc) != w.FromDate {
		return time.Time{}, time.Time{}, feedErr("%s: fromMs is not midnight of fromDate", what)
	}
	if msMidnight(w.ToMs, loc) != w.ToDate {
		return time.Time{}, time.Time{}, feedErr("%s: toMs is not midnight of toDate", what)
	}
	return time.UnixMilli(w.FromMs).In(loc), time.UnixMilli(w.ToMs).In(loc), nil
}

// msMidnight renders ms as a local calendar date, or "" when it is not
// exactly local midnight.
func msMidnight(ms int64, loc *time.Location) string {
	t := time.UnixMilli(ms).In(loc)
	if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 || t.Nanosecond() != 0 {
		return ""
	}
	return t.Format("2006-01-02")
}

// ------------------------------------------------------------- conversion

// convertFeed assumes a fully validated feed. Calendars are sorted by order
// then source key; the numeric id/cal/into indexes are display-only and
// reassigned after sorting, while stable IDs, persons, bindings and
// per-source status travel additively. Events keep their received order and
// full coverage; consumers apply their own display windows.
func convertFeed(f calendarFeed, feedLoc *time.Location, winStart, winEnd time.Time, people []calendar.Person, bindings []calendar.PersonBinding) calendar.Snapshot {
	cals := append([]feedCalendar(nil), *f.Calendars...)
	sort.SliceStable(cals, func(i, j int) bool {
		if cals[i].Order != cals[j].Order {
			return cals[i].Order < cals[j].Order
		}
		return cals[i].SourceKey < cals[j].SourceKey
	})
	index := map[string]int{}
	for i, c := range cals {
		index[c.ID] = i
	}
	infos := make([]calendar.CalendarInfo, 0, len(cals))
	for i, c := range cals {
		into := -1
		if c.IntoCalendarID != nil && *c.IntoCalendarID != "" {
			into = index[*c.IntoCalendarID]
		}
		st := calendar.SourceStatus{Freshness: c.Freshness, Error: c.Error}
		if c.LastAttemptAt != nil {
			st.LastAttemptAt = time.UnixMilli(*c.LastAttemptAt).In(feedLoc)
		}
		if c.LastSuccessAt != nil {
			st.LastSuccessAt = time.UnixMilli(*c.LastSuccessAt).In(feedLoc)
		}
		if c.LastAttemptStatus != nil {
			st.LastAttemptStatus = *c.LastAttemptStatus
		}
		pids := append([]string(nil), c.PersonIDs...)
		if pids == nil {
			pids = []string{}
		}
		infos = append(infos, calendar.CalendarInfo{
			ID: i, Name: c.Name, Color: c.Color, Panel: c.Panel, Into: into,
			CalendarID: c.ID, PersonIDs: pids, Status: st,
		})
	}

	events := make([]calendar.Event, 0, len(*f.Events))
	for _, e := range *f.Events {
		ci := index[e.CalendarID]
		title := e.Title
		if title == "" {
			title = "(ohne Titel)"
		}
		ev := calendar.Event{
			Cal: ci, Calendar: cals[ci].Name, Color: cals[ci].Color,
			Title: title, Location: e.Location, AllDay: e.AllDay,
			CalendarID: e.CalendarID, Key: e.Key, UID: e.UID, IdentityQuality: e.IdentityQuality,
		}
		if e.AllDay {
			ev.StartDate, ev.EndDate = *e.StartDate, *e.EndDate
			sd, _ := time.ParseInLocation("2006-01-02", *e.StartDate, feedLoc)
			ed, _ := time.ParseInLocation("2006-01-02", *e.EndDate, feedLoc)
			ev.Start = sd
			ev.End = ed
		} else {
			ev.Start = time.UnixMilli(e.StartMs).In(feedLoc)
			ev.End = time.UnixMilli(e.EndMs).In(feedLoc)
		}
		events = append(events, ev)
	}

	snap := calendar.Snapshot{
		Calendars: infos, Events: events,
		Central: true, People: people, Bindings: bindings,
		ConfigurationRevision: *f.ConfigurationRevision,
		WindowStart:           winStart, WindowEnd: winEnd,
	}
	// UpdatedAt is the oldest successful active source timestamp. Disabled
	// sources are not required; a required source without success keeps it
	// zero. With no required sources the feed generation time applies.
	updated := time.UnixMilli(*f.GeneratedAt).In(feedLoc)
	first := true
	anyRequired := false
	for _, c := range cals {
		if c.Freshness == "disabled" {
			continue
		}
		anyRequired = true
		if c.LastSuccessAt == nil {
			updated = time.Time{}
			break
		}
		if st := time.UnixMilli(*c.LastSuccessAt).In(feedLoc); first || st.Before(updated) {
			updated = st
			first = false
		}
	}
	if !anyRequired {
		updated = time.UnixMilli(*f.GeneratedAt).In(feedLoc)
	}
	snap.UpdatedAt = updated
	return snap
}

// ---------------------------------------------------------------- service

// CalendarService polls the central feed and keeps the last fully validated
// snapshot in RAM and on disk. It implements calendar.Source without any
// local ICS polling.
type CalendarService struct {
	client    *Client
	token     string
	cachePath string
	loc       *time.Location
	backend   string

	refreshMu sync.Mutex // serializes polls so an old fetch cannot win
	mu        sync.RWMutex
	snap      *calendar.Snapshot
	lastErr   error
	configErr string
}

var _ calendar.Source = (*CalendarService)(nil)

// NewCalendarService builds the central source. A missing backend URL or
// token does not fail and never falls back to local polling; the snapshot
// stays in a visible unavailable state until a poll succeeds. A strictly
// validated persistent cache is loaded when it belongs to this backend.
func NewCalendarService(client *Client, token string, cachePath string, loc *time.Location) *CalendarService {
	s := &CalendarService{client: client, token: token, cachePath: cachePath, loc: loc}
	if client != nil {
		s.backend = normalizeCalendarBackend(client.BaseURL)
	}
	switch {
	case client == nil || strings.TrimSpace(client.BaseURL) == "":
		s.configErr = "FAMILY_APP_SITE_URL fehlt – zentrale Kalender nicht verfügbar"
	case strings.TrimSpace(token) == "":
		s.configErr = "FAMILY_APP_CALENDAR_TOKEN fehlt – zentrale Kalender nicht verfügbar"
	}
	if s.configErr == "" && cachePath != "" {
		if snap, err := loadCalendarCache(cachePath, s.backend, loc); err != nil {
			// A missing file is the normal first start, not a warning.
			if !os.IsNotExist(err) {
				s.lastErr = err
				slog.Warn("calendar cache rejected", "path", cachePath, "err", err)
			}
		} else {
			s.snap = snap
		}
	}
	return s
}

// Refresh fetches the feed once. Only a fully validated response replaces
// the last-good snapshot (and the cache file); every failure — credentials,
// HTTP, validation, oversize — keeps the previous state.
func (s *CalendarService) Refresh(ctx context.Context) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.configErr != "" {
		s.mu.Lock()
		s.lastErr = errors.New(s.configErr)
		s.mu.Unlock()
		return
	}
	data, err := s.client.getCalendarFeed(ctx, s.token)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		slog.Warn("calendar refresh failed", "err", err)
		return
	}
	snap, err := ParseCalendarFeed(data, s.loc)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		slog.Warn("calendar refresh rejected", "err", err)
		return
	}
	raw := append([]byte(nil), data...)
	s.mu.Lock()
	s.snap = &snap
	s.lastErr = nil
	s.mu.Unlock()
	if s.cachePath != "" {
		if err := saveCalendarCache(s.cachePath, s.backend, raw); err != nil {
			// Warning only: the accepted RAM data stays.
			slog.Warn("calendar cache not saved", "path", s.cachePath, "err", err)
		}
	}
}

// Run polls every interval until ctx ends.
func (s *CalendarService) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = ConvexPollInterval
	}
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

// Snapshot returns the last-good snapshot, or — when nothing was ever
// accepted — a visible unavailable state instead of silent local data.
func (s *CalendarService) Snapshot() calendar.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snap != nil {
		out := *s.snap
		out.Calendars = append([]calendar.CalendarInfo(nil), s.snap.Calendars...)
		for i := range out.Calendars {
			out.Calendars[i].PersonIDs = append([]string(nil), s.snap.Calendars[i].PersonIDs...)
		}
		out.Events = append([]calendar.Event(nil), s.snap.Events...)
		out.People = append([]calendar.Person(nil), s.snap.People...)
		out.Bindings = append([]calendar.PersonBinding(nil), s.snap.Bindings...)
		return out
	}
	msg := "noch nicht geladen"
	switch {
	case s.configErr != "":
		msg = s.configErr
	case s.lastErr != nil:
		msg = s.lastErr.Error()
	}
	return calendar.Snapshot{
		Central:   true,
		Calendars: []calendar.CalendarInfo{},
		Events:    []calendar.Event{},
		People:    []calendar.Person{},
		Bindings:  []calendar.PersonBinding{},
		Errors:    map[string]string{"Kalender": msg},
	}
}
