// Package vielfalt shows the school lunch each child has ordered with the
// caterer VielfaltMenü (https://vielfaltmenue.com).
//
// There is no public API. This uses the same requests as the parent portal
// bestellung.vielfaltmenue.com: a login that returns a token, then the weekly
// meal plan as an HTML fragment from ibs.vielfaltmenue.com. Every child has
// its own account (Kundennummer + password). It can break whenever the portal
// changes; failures only affect the meal cards.
package vielfalt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultLoginURL = "https://bestellung.vielfaltmenue.com"
	DefaultIBSURL   = "https://ibs.vielfaltmenue.com"
	defaultIframe   = "ibs4_2019_delegate"
	userAgent       = "Mozilla/5.0 (X11; Linux) familydash"
)

// Account is one child's portal login.
type Account struct {
	Name     string // shown on the card; empty = name from the portal
	User     string // Kundennummer
	Password string
	Color    string
}

type Dish struct {
	Name string `json:"name"`           // main dish (before the first "|")
	Side string `json:"side,omitempty"` // side dishes (after "|")
}

// Day is a delivery day: at least one menu was offered on it.
type Day struct {
	Date    string `json:"date"`    // YYYY-MM-DD
	Ordered []Dish `json:"ordered"` // empty = nothing ordered
}

type Child struct {
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	Days      []Day     `json:"days"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type Meals struct {
	Children []Child `json:"children"`
}

type Service struct {
	LoginURL string
	IBSURL   string

	accounts []Account
	loc      *time.Location
	client   *http.Client

	mu   sync.RWMutex
	last []*Child // last good data per account (nil until the first success)
	errs []string // last error per account
}

func NewService(accounts []Account, loc *time.Location) *Service {
	return &Service{
		LoginURL: DefaultLoginURL, IBSURL: DefaultIBSURL,
		accounts: accounts, loc: loc,
		client: &http.Client{Timeout: 20 * time.Second},
		last:   make([]*Child, len(accounts)),
		errs:   make([]string, len(accounts)),
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
	for i, acc := range s.accounts {
		c, err := s.Fetch(ctx, acc, time.Now())
		s.mu.Lock()
		if err != nil {
			slog.Warn("vielfalt refresh failed", "child", acc.label(i), "err", err)
			s.errs[i] = err.Error()
		} else {
			s.last[i], s.errs[i] = c, ""
		}
		s.mu.Unlock()
	}
}

// Snapshot returns one card per account: the last good data plus the last error.
func (s *Service) Snapshot() *Meals {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := &Meals{Children: make([]Child, 0, len(s.accounts))}
	for i, acc := range s.accounts {
		var c Child
		if s.last[i] != nil {
			c = *s.last[i]
		} else {
			c = Child{Name: acc.label(i), Color: acc.Color, Days: []Day{}}
		}
		c.Error = s.errs[i]
		m.Children = append(m.Children, c)
	}
	return m
}

func (a Account) label(i int) string {
	if a.Name != "" {
		return a.Name
	}
	return fmt.Sprintf("Kind %d", i+1)
}

// Fetch logs in and reads this and next week's plan (next week is needed for
// "tomorrow" on Friday–Sunday). Only today and later days are returned.
func (s *Service) Fetch(ctx context.Context, acc Account, now time.Time) (*Child, error) {
	jar, _ := cookiejar.New(nil)
	client := *s.client
	client.Jar = jar

	auth, err := s.login(ctx, &client, acc)
	if err != nil {
		return nil, err
	}
	name := acc.Name
	if name == "" {
		name = auth.Address.Name1
	}

	now = now.In(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	byDate := map[string]*Day{}
	for _, d := range []time.Time{today, today.AddDate(0, 0, 7)} {
		year, week := d.ISOWeek()
		page, err := s.weekplan(ctx, &client, auth, year, week)
		if err != nil {
			return nil, err
		}
		for _, day := range ParseWeek(page) {
			day := day
			byDate[day.Date] = &day
		}
	}

	c := &Child{Name: name, Color: acc.Color, Days: []Day{}, UpdatedAt: time.Now()}
	for date, d := range byDate {
		if date >= today.Format("2006-01-02") {
			c.Days = append(c.Days, *d)
		}
	}
	sort.Slice(c.Days, func(i, j int) bool { return c.Days[i].Date < c.Days[j].Date })
	return c, nil
}

type loginResponse struct {
	Token   string `json:"token"`
	Iframe  string `json:"iframe"`
	Address struct {
		Name1 string `json:"Name1"`
	} `json:"adresse"`
}

func (s *Service) login(ctx context.Context, client *http.Client, acc Account) (*loginResponse, error) {
	// Open the portal first so the session cookie exists, like a browser would.
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.LoginURL+"/", nil); err == nil {
		req.Header.Set("User-Agent", userAgent)
		if resp, err := client.Do(req); err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
		}
	}

	form := url.Values{"username": {acc.User}, "password": {acc.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.LoginURL+"/frontend/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", s.LoginURL)
	req.Header.Set("Referer", s.LoginURL+"/")
	req.Header.Set("lang", "de")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VielfaltMenü-Login: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("VielfaltMenü-Login fehlgeschlagen – Kundennummer/Passwort prüfen")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("VielfaltMenü-Login: HTTP %d", resp.StatusCode)
	}
	var auth loginResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("VielfaltMenü-Login: unerwartete Antwort")
	}
	if auth.Token == "" {
		return nil, fmt.Errorf("VielfaltMenü-Login fehlgeschlagen – Kundennummer/Passwort prüfen")
	}
	if auth.Iframe == "" {
		auth.Iframe = defaultIframe
	}
	return &auth, nil
}

func (s *Service) weekplan(ctx context.Context, client *http.Client, auth *loginResponse, year, week int) (string, error) {
	u := fmt.Sprintf("%s/%s/Mealplan/Weekplan?year=%d&week=%d", s.IBSURL, url.PathEscape(auth.Iframe), year, week)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Auth", auth.Token)
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", s.LoginURL)
	req.Header.Set("Referer", s.LoginURL+"/")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("VielfaltMenü KW %d: %w", week, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("VielfaltMenü KW %d: HTTP %d", week, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("VielfaltMenü KW %d: %w", week, err)
	}
	page := string(body)
	if !strings.Contains(page, "menuplan") && !strings.Contains(page, "order-menu") {
		return "", fmt.Errorf("VielfaltMenü KW %d: unerwartete Antwort (Login abgelaufen?)", week)
	}
	return page, nil
}
