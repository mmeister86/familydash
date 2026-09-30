// Package uptime reads one Uptime Kuma status page (public JSON, no login) and
// reduces it to what the dashboard footer needs: which monitors are up, down,
// pending or in maintenance, and since when. With an API key it also reads
// /metrics for the days left on each monitor's TLS certificate.
package uptime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Monitor states as the footer knows them.
const (
	StateUp          = "up"
	StateDown        = "down"
	StatePending     = "pending"
	StateMaintenance = "maintenance"
	StateUnknown     = "unknown" // no heartbeat yet
)

type Monitor struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Ping     int     `json:"ping,omitempty"`     // ms of the last heartbeat
	Uptime24 float64 `json:"uptime24,omitempty"` // 0…1
	// Since is the first heartbeat of the current state run (only for
	// down/pending/maintenance). It is a lower bound: Kuma only sends the
	// last ~100 beats, so an old outage may have started earlier.
	Since    *time.Time `json:"since,omitempty"`
	Msg      string     `json:"msg,omitempty"`
	CertDays *int       `json:"certDays,omitempty"` // TLS days left, only with UPTIME_API_KEY
}

type Status struct {
	Title     string    `json:"title,omitempty"`
	Monitors  []Monitor `json:"monitors"`
	Incident  string    `json:"incident,omitempty"` // pinned incident on the status page
	CertWarn  int       `json:"certWarn"`           // days; the footer warns at or below
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

type Service struct {
	BaseURL  string // e.g. http://192.168.188.127:3001
	Slug     string // status page slug
	APIKey   string // optional, for /metrics
	CertWarn int
	client   *http.Client

	mu   sync.RWMutex
	last *Status
	err  string
}

// ParsePageURL splits a status page URL like
// http://192.168.188.127:3001/status/dashboard into base URL and slug.
func ParsePageURL(raw string) (base, slug string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("%q is not a URL", raw)
	}
	i := strings.LastIndex(u.Path, "/status/")
	if i < 0 {
		return "", "", fmt.Errorf("%q: want the status page URL, …/status/<slug>", raw)
	}
	slug = strings.Trim(u.Path[i+len("/status/"):], "/")
	if slug == "" || strings.Contains(slug, "/") {
		return "", "", fmt.Errorf("%q: no status page slug after /status/", raw)
	}
	u.Path, u.RawQuery, u.Fragment = u.Path[:i], "", ""
	return strings.TrimRight(u.String(), "/"), slug, nil
}

func NewService(base, slug, apiKey string, certWarn int) *Service {
	return &Service{BaseURL: base, Slug: slug, APIKey: apiKey, CertWarn: certWarn,
		client: &http.Client{Timeout: 10 * time.Second}}
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
	st, err := s.fetch(ctx, time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		slog.Warn("uptime kuma refresh failed", "err", err)
		s.err = err.Error()
		return
	}
	s.last, s.err = st, ""
}

// Snapshot returns the last good status (nil if none yet) plus the last error.
func (s *Service) Snapshot() *Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.last == nil {
		if s.err == "" {
			return nil
		}
		return &Status{Monitors: []Monitor{}, CertWarn: s.CertWarn, Error: s.err}
	}
	st := *s.last
	st.Error = s.err
	return &st
}

// ------------------------------------------------------------------ API

type pageResponse struct {
	Config struct {
		Title string `json:"title"`
	} `json:"config"`
	Incident *struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"incident"`
	PublicGroupList []struct {
		MonitorList []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"monitorList"`
	} `json:"publicGroupList"`
}

type heartbeat struct {
	Status int     `json:"status"` // 0 down, 1 up, 2 pending, 3 maintenance
	Time   string  `json:"time"`   // UTC, "2006-01-02 15:04:05.000"
	Msg    string  `json:"msg"`
	Ping   float64 `json:"ping"`
}

type heartbeatResponse struct {
	HeartbeatList map[string][]heartbeat `json:"heartbeatList"`
	UptimeList    map[string]float64     `json:"uptimeList"`
}

func (s *Service) fetch(ctx context.Context, now time.Time) (*Status, error) {
	var page pageResponse
	if err := s.getJSON(ctx, "/api/status-page/"+url.PathEscape(s.Slug), &page); err != nil {
		return nil, err
	}
	var beats heartbeatResponse
	if err := s.getJSON(ctx, "/api/status-page/heartbeat/"+url.PathEscape(s.Slug), &beats); err != nil {
		return nil, err
	}
	st := build(&page, &beats, now)
	st.CertWarn = s.CertWarn
	if s.APIKey != "" {
		// Certificates are a bonus: keep the status even if /metrics fails.
		if certs, err := s.fetchCerts(ctx); err != nil {
			slog.Warn("uptime kuma metrics failed", "err", err)
		} else {
			for i := range st.Monitors {
				if d, ok := certs[st.Monitors[i].Name]; ok {
					st.Monitors[i].CertDays = &d
				}
			}
		}
	}
	return st, nil
}

func (s *Service) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("uptime kuma %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("uptime kuma %s: %w", path, err)
	}
	return nil
}

// build merges the monitor list (in status page order) with the heartbeats.
func build(page *pageResponse, beats *heartbeatResponse, now time.Time) *Status {
	st := &Status{Title: page.Config.Title, Monitors: []Monitor{}, UpdatedAt: now}
	if page.Incident != nil {
		st.Incident = strings.TrimSpace(page.Incident.Title)
		if st.Incident == "" {
			st.Incident = strings.TrimSpace(page.Incident.Content)
		}
	}
	seen := map[int]bool{}
	for _, g := range page.PublicGroupList {
		for _, m := range g.MonitorList {
			if seen[m.ID] {
				continue // a monitor can sit in several groups
			}
			seen[m.ID] = true
			id := strconv.Itoa(m.ID)
			mon := Monitor{ID: m.ID, Name: m.Name, State: StateUnknown, Uptime24: beats.UptimeList[id+"_24"]}
			if list := beats.HeartbeatList[id]; len(list) > 0 {
				last := list[len(list)-1]
				mon.State = stateOf(last.Status)
				mon.Ping = int(last.Ping + 0.5)
				mon.Msg = strings.TrimSpace(last.Msg)
				if mon.State != StateUp {
					mon.Since = runStart(list)
				}
			}
			st.Monitors = append(st.Monitors, mon)
		}
	}
	return st
}

func stateOf(code int) string {
	switch code {
	case 0:
		return StateDown
	case 1:
		return StateUp
	case 2:
		return StatePending
	case 3:
		return StateMaintenance
	}
	return StateUnknown
}

// runStart returns the time of the first heartbeat of the trailing run that
// has the same status as the last one (heartbeats are oldest first).
func runStart(list []heartbeat) *time.Time {
	code := list[len(list)-1].Status
	i := len(list) - 1
	for i > 0 && list[i-1].Status == code {
		i--
	}
	t, ok := parseTime(list[i].Time)
	if !ok {
		return nil
	}
	return &t
}

// parseTime reads Kuma's heartbeat time. Kuma 1.x sends UTC without a zone
// ("2026-09-30 20:15:16.689"), be lenient with ISO variants too.
func parseTime(v string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05.999", "2006-01-02T15:04:05.999Z07:00", "2006-01-02T15:04:05.999"} {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ------------------------------------------------------------------ /metrics

var nameLabel = regexp.MustCompile(`monitor_name="((?:[^"\\]|\\.)*)"`)

// fetchCerts reads monitor_cert_days_remaining from the Prometheus endpoint.
// Kuma accepts an API key as basic auth password with an empty user name.
func (s *Service) fetchCerts(ctx context.Context) (map[string]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("", s.APIKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uptime kuma /metrics: HTTP %d (check UPTIME_API_KEY)", resp.StatusCode)
	}
	return parseCerts(resp.Body)
}

func parseCerts(r io.Reader) (map[string]int, error) {
	out := map[string]int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "monitor_cert_days_remaining{") {
			continue
		}
		end := strings.LastIndex(line, "}")
		m := nameLabel.FindStringSubmatch(line[:max(end, 0)])
		if end < 0 || m == nil {
			continue
		}
		fields := strings.Fields(line[end+1:])
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		name := strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\n`, "\n").Replace(m[1])
		out[name] = int(v)
	}
	return out, sc.Err()
}
