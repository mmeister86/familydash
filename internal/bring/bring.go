// Package bring reads one shopping list from Bring! (https://getbring.com).
//
// Bring! has no public API. This uses the same endpoints as the Bring! apps,
// as reverse-engineered by the community bring-api library used by Home
// Assistant (https://github.com/miaucl/bring-api). It can break whenever
// Bring! changes things; failures only affect this panel.
package bring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL   = "https://api.getbring.com/rest/"
	DefaultLocaleURL = "https://web.getbring.com/locale/"
	// Public key shipped in the Bring! Android app (same as bring-api uses).
	apiKey = "cof4Nc6D8saplXjE3h3HXqHH8m7VU2i1Gs0g85Sp"
)

type Item struct {
	Name string `json:"name"`
	Spec string `json:"spec,omitempty"`
}

type List struct {
	Name      string    `json:"name"`
	Items     []Item    `json:"items"`
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

type Service struct {
	BaseURL   string
	LocaleURL string

	email, password string
	listName        string
	locale          string
	client          *http.Client

	// auth state (only touched from the refresh goroutine)
	uuid, token, defaultList string
	expires                  time.Time
	translations             map[string]string

	mu   sync.RWMutex
	last *List
	err  string
}

func NewService(email, password, listName, locale string) *Service {
	return &Service{
		BaseURL: DefaultBaseURL, LocaleURL: DefaultLocaleURL,
		email: email, password: password, listName: listName, locale: locale,
		client: &http.Client{Timeout: 15 * time.Second},
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
		slog.Warn("bring refresh failed", "err", err)
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
		return &List{Name: "Einkauf", Items: []Item{}, Error: s.err}
	}
	l := *s.last
	l.Error = s.err
	return &l
}

type errUnauthorized struct{}

func (errUnauthorized) Error() string { return "unauthorized" }

func (s *Service) fetch(ctx context.Context) (*List, error) {
	if s.token == "" || time.Now().After(s.expires) {
		if err := s.login(ctx); err != nil {
			return nil, err
		}
	}
	l, err := s.fetchList(ctx)
	if _, ok := err.(errUnauthorized); ok { // token revoked → one fresh login
		if err := s.login(ctx); err != nil {
			return nil, err
		}
		l, err = s.fetchList(ctx)
	}
	return l, err
}

func (s *Service) fetchList(ctx context.Context) (*List, error) {
	var lists struct {
		Lists []struct {
			ListUUID string `json:"listUuid"`
			Name     string `json:"name"`
		} `json:"lists"`
	}
	if err := s.get(ctx, "bringusers/"+s.uuid+"/lists", &lists); err != nil {
		return nil, err
	}
	var uuid, name string
	for _, l := range lists.Lists {
		if (s.listName != "" && strings.EqualFold(strings.TrimSpace(l.Name), s.listName)) ||
			(s.listName == "" && l.ListUUID == s.defaultList) {
			uuid, name = l.ListUUID, l.Name
			break
		}
	}
	if uuid == "" {
		if s.listName != "" {
			var names []string
			for _, l := range lists.Lists {
				names = append(names, l.Name)
			}
			return nil, fmt.Errorf("Bring!-Liste %q nicht gefunden (vorhanden: %s)", s.listName, strings.Join(names, ", "))
		}
		if len(lists.Lists) == 0 {
			return nil, fmt.Errorf("keine Bring!-Listen gefunden")
		}
		uuid, name = lists.Lists[0].ListUUID, lists.Lists[0].Name
	}

	var content struct {
		Items struct {
			Purchase []struct {
				ItemID        string `json:"itemId"`
				Specification string `json:"specification"`
			} `json:"purchase"`
		} `json:"items"`
	}
	if err := s.get(ctx, "v2/bringlists/"+uuid, &content); err != nil {
		return nil, err
	}
	l := &List{Name: name, Items: []Item{}, UpdatedAt: time.Now()}
	for _, p := range content.Items.Purchase {
		l.Items = append(l.Items, Item{Name: s.translate(p.ItemID), Spec: strings.TrimSpace(p.Specification)})
	}
	return l, nil
}

func (s *Service) login(ctx context.Context) error {
	form := url.Values{"email": {s.email}, "password": {s.password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"v2/bringauth", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.setHeaders(req, false)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("Bring!-Login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("Bring!-Login fehlgeschlagen – E-Mail/Passwort prüfen")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bring!-Login: HTTP %d", resp.StatusCode)
	}
	var auth struct {
		UUID          string `json:"uuid"`
		BringListUUID string `json:"bringListUUID"`
		AccessToken   string `json:"access_token"`
		ExpiresIn     int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return fmt.Errorf("Bring!-Login: %w", err)
	}
	if auth.AccessToken == "" || auth.UUID == "" {
		return fmt.Errorf("Bring!-Login: unerwartete Antwort")
	}
	s.uuid, s.token, s.defaultList = auth.UUID, auth.AccessToken, auth.BringListUUID
	life := time.Duration(auth.ExpiresIn) * time.Second
	if life <= 0 {
		life = time.Hour
	}
	s.expires = time.Now().Add(life - time.Minute)
	if s.translations == nil {
		s.loadTranslations(ctx)
	}
	return nil
}

func (s *Service) setHeaders(req *http.Request, auth bool) {
	req.Header.Set("X-BRING-API-KEY", apiKey)
	req.Header.Set("X-BRING-CLIENT", "android")
	req.Header.Set("X-BRING-APPLICATION", "bring")
	req.Header.Set("X-BRING-COUNTRY", "DE")
	req.Header.Set("Accept", "application/json")
	if auth {
		req.Header.Set("X-BRING-USER-UUID", s.uuid)
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
}

func (s *Service) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+path, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req, true)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("Bring!: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		s.token = ""
		return errUnauthorized{}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bring! %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

// Bring! stores catalog items under their Swiss-German key ("Rüebli").
// The locale file maps those keys to e.g. German names ("Karotten").
func (s *Service) loadTranslations(ctx context.Context) {
	s.translations = map[string]string{}
	if s.locale == "" || s.locale == "de-CH" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.LocaleURL+"articles."+s.locale+".json", nil)
	if err != nil {
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		slog.Warn("bring translations", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("bring translations", "status", resp.StatusCode)
		return
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&s.translations); err != nil {
		slog.Warn("bring translations", "err", err)
	}
}

func (s *Service) translate(id string) string {
	if t, ok := s.translations[id]; ok && t != "" {
		return t
	}
	return id
}
