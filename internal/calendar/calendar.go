package calendar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"familydash/internal/config"
)

// Service periodically fetches all configured feeds and keeps the last good
// result per calendar, so a flaky feed never blanks the wall display.
type Service struct {
	cals   []config.Calendar
	days   int
	loc    *time.Location
	client *http.Client

	mu     sync.RWMutex
	byCal  map[string][]Event // keyed by calendar URL
	errs   map[string]string  // calendar name -> last error
	update time.Time
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cals:   cfg.Calendars,
		days:   cfg.CalendarDays,
		loc:    cfg.Location,
		client: &http.Client{Timeout: 20 * time.Second},
		byCal:  map[string][]Event{},
		errs:   map[string]string{},
	}
}

func (s *Service) Run(ctx context.Context, every time.Duration) {
	if len(s.cals) == 0 {
		return
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

func (s *Service) Refresh(ctx context.Context) {
	now := time.Now().In(s.loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)

	var wg sync.WaitGroup
	for i, c := range s.cals {
		wg.Add(1)
		go func(i int, c config.Calendar) {
			defer wg.Done()
			days := s.days
			if c.Panel == "school" && days < schoolDays {
				days = schoolDays // tests and trips are announced weeks ahead
			}
			evs, err := s.fetch(ctx, c, from, from.AddDate(0, 0, days))
			for j := range evs {
				evs[j].Cal = i
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if err != nil {
				slog.Warn("calendar refresh failed", "calendar", c.Name, "err", err)
				s.errs[c.Name] = err.Error()
				return
			}
			delete(s.errs, c.Name)
			s.byCal[c.URL] = evs
		}(i, c)
	}
	wg.Wait()
	s.mu.Lock()
	s.update = time.Now()
	s.mu.Unlock()
}

func (s *Service) fetch(ctx context.Context, c config.Calendar, from, to time.Time) ([]Event, error) {
	url := c.URL
	if strings.HasPrefix(url, "webcal://") {
		url = "https://" + strings.TrimPrefix(url, "webcal://")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "familydash/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return Expand(body, c.Name, c.Color, from, to, s.loc)
}

// schoolDays is the look-ahead for calendars shown as a school card.
const schoolDays = 21

// CalendarInfo describes a configured calendar, in configuration order.
type CalendarInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Panel string `json:"panel"`
}

type Snapshot struct {
	Calendars []CalendarInfo    `json:"calendars"`
	Events    []Event           `json:"events"`
	Errors    map[string]string `json:"errors,omitempty"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []Event
	for _, evs := range s.byCal {
		all = append(all, evs...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Start.Equal(all[j].Start) {
			return all[i].Start.Before(all[j].Start)
		}
		return all[i].AllDay && !all[j].AllDay
	})
	errs := make(map[string]string, len(s.errs))
	for k, v := range s.errs {
		errs[k] = v
	}
	infos := make([]CalendarInfo, len(s.cals))
	for i, c := range s.cals {
		infos[i] = CalendarInfo{ID: i, Name: c.Name, Color: c.Color, Panel: c.Panel}
	}
	return Snapshot{Calendars: infos, Events: all, Errors: errs, UpdatedAt: s.update}
}
