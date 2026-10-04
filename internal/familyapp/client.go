// Package familyapp connects the wall to the family app (React PWA + Convex,
// see docs/FAMILY_APP.md). Two directions, both started from here, so Unraid
// never has to be reachable from the internet:
//
//   - push: each child's week (timetable, homework, exams, appointments,
//     lunch) and the AI briefings go to POST /ingest/child and
//     POST /ingest/briefing (FAMILY_APP_INGEST_TOKEN)
//   - pull: the to-dos come from GET /todos (FAMILY_APP_DASHBOARD_TOKEN)
//
// Failures only ever log and retry; they never touch the wall display.
package familyapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the HTTP actions of the family app's Convex backend
// (the "site" URL, e.g. https://familybackend-http.matthias.lol).
type Client struct {
	BaseURL     string
	IngestToken string // write: /ingest/*
	ReadToken   string // read: /todos
	HTTP        *http.Client
}

func NewClient(baseURL, ingestToken, readToken string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		IngestToken: ingestToken,
		ReadToken:   readToken,
		HTTP:        &http.Client{Timeout: 20 * time.Second},
	}
}

// StatusError is a non-2xx answer of the backend.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("Familienapp: Token abgelehnt (HTTP %d)", e.Status)
	case http.StatusNotFound:
		return "Familienapp: Endpunkt nicht gefunden (HTTP 404) – FAMILY_APP_SITE_URL prüfen"
	}
	if e.Body != "" {
		return fmt.Sprintf("Familienapp: HTTP %d: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("Familienapp: HTTP %d", e.Status)
}

func (c *Client) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, c.IngestToken, nil)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, c.ReadToken, out)
}

// getCalendarFeed fetches the raw central calendar feed (GET
// /dashboard/calendars) using the calendar-only token. It returns the raw
// body for strict validation by ParseCalendarFeed.
func (c *Client) getCalendarFeed(ctx context.Context, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/dashboard/calendars", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Familienapp nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCalendarFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCalendarFeedBytes {
		return nil, fmt.Errorf("Familienapp: Kalenderantwort zu groß (> 4 MiB) – letzter Stand bleibt")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return nil, &StatusError{Status: resp.StatusCode, Body: msg}
	}
	return data, nil
}

func (c *Client) do(req *http.Request, token string, out any) error {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Familienapp nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return &StatusError{Status: resp.StatusCode, Body: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Familienapp: unerwartete Antwort: %w", err)
	}
	return nil
}
