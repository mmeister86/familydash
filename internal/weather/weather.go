// Package weather talks to Open-Meteo (free, no API key, DWD ICON model for
// Germany) and reduces the answer to what a wall display needs.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const DefaultBaseURL = "https://api.open-meteo.com/v1/forecast"

type Current struct {
	Temp  float64 `json:"temp"`
	Feels float64 `json:"feels"`
	Wind  float64 `json:"wind"` // km/h
	Code  int     `json:"code"`
	Label string  `json:"label"`
	Icon  string  `json:"icon"`
	IsDay bool    `json:"isDay"`
}

type Hour struct {
	Time       time.Time `json:"time"`
	Temp       float64   `json:"temp"`
	PrecipProb int       `json:"precipProb"`
	Icon       string    `json:"icon"`
}

type Day struct {
	Date       string    `json:"date"` // YYYY-MM-DD
	Max        float64   `json:"max"`
	Min        float64   `json:"min"`
	PrecipProb int       `json:"precipProb"`
	Code       int       `json:"code"`
	Label      string    `json:"label"`
	Icon       string    `json:"icon"`
	Sunrise    time.Time `json:"sunrise"`
	Sunset     time.Time `json:"sunset"`
}

type Weather struct {
	Location  string    `json:"location,omitempty"`
	Current   Current   `json:"current"`
	Hourly    []Hour    `json:"hourly"`
	Daily     []Day     `json:"daily"`
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

type Service struct {
	BaseURL  string
	lat, lon float64
	name     string
	loc      *time.Location
	client   *http.Client

	mu   sync.RWMutex
	last *Weather
	err  string
}

func NewService(lat, lon float64, name string, loc *time.Location) *Service {
	return &Service{BaseURL: DefaultBaseURL, lat: lat, lon: lon, name: name, loc: loc,
		client: &http.Client{Timeout: 15 * time.Second}}
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
	w, err := s.fetch(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		slog.Warn("weather refresh failed", "err", err)
		s.err = err.Error()
		return
	}
	s.last, s.err = w, ""
}

// Snapshot returns the last good forecast (nil if none yet) plus the last error.
func (s *Service) Snapshot() *Weather {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.last == nil {
		if s.err == "" {
			return nil
		}
		return &Weather{Error: s.err}
	}
	w := *s.last
	w.Error = s.err
	return &w
}

type apiResponse struct {
	Current struct {
		Temp  float64 `json:"temperature_2m"`
		Feels float64 `json:"apparent_temperature"`
		Code  int     `json:"weather_code"`
		Wind  float64 `json:"wind_speed_10m"`
		IsDay int     `json:"is_day"`
	} `json:"current"`
	Hourly struct {
		Time       []string  `json:"time"`
		Temp       []float64 `json:"temperature_2m"`
		PrecipProb []int     `json:"precipitation_probability"`
		Code       []int     `json:"weather_code"`
		IsDay      []int     `json:"is_day"`
	} `json:"hourly"`
	Daily struct {
		Time       []string  `json:"time"`
		Code       []int     `json:"weather_code"`
		Max        []float64 `json:"temperature_2m_max"`
		Min        []float64 `json:"temperature_2m_min"`
		PrecipProb []int     `json:"precipitation_probability_max"`
		Sunrise    []string  `json:"sunrise"`
		Sunset     []string  `json:"sunset"`
	} `json:"daily"`
}

func (s *Service) fetch(ctx context.Context) (*Weather, error) {
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(s.lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(s.lon, 'f', 4, 64))
	q.Set("current", "temperature_2m,apparent_temperature,weather_code,wind_speed_10m,is_day")
	q.Set("hourly", "temperature_2m,precipitation_probability,weather_code,is_day")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,sunrise,sunset")
	q.Set("timezone", s.loc.String())
	q.Set("forecast_days", "6")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo: HTTP %d", resp.StatusCode)
	}
	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	return s.transform(&r, time.Now()), nil
}

func (s *Service) parseLocal(v string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02T15:04", v, s.loc)
	return t
}

func (s *Service) transform(r *apiResponse, now time.Time) *Weather {
	label, icon := Describe(r.Current.Code, r.Current.IsDay == 1)
	w := &Weather{
		Location: s.name,
		Current: Current{Temp: r.Current.Temp, Feels: r.Current.Feels, Wind: r.Current.Wind,
			Code: r.Current.Code, Label: label, Icon: icon, IsDay: r.Current.IsDay == 1},
		UpdatedAt: now,
	}
	// next 12 hours, starting with the current hour
	cutoff := now.Truncate(time.Hour)
	for i, ts := range r.Hourly.Time {
		t := s.parseLocal(ts)
		if t.Before(cutoff) || len(w.Hourly) >= 12 {
			continue
		}
		isDay := i < len(r.Hourly.IsDay) && r.Hourly.IsDay[i] == 1
		_, ic := Describe(at(r.Hourly.Code, i), isDay)
		w.Hourly = append(w.Hourly, Hour{Time: t, Temp: atF(r.Hourly.Temp, i), PrecipProb: at(r.Hourly.PrecipProb, i), Icon: ic})
	}
	for i, date := range r.Daily.Time {
		lbl, ic := Describe(at(r.Daily.Code, i), true)
		day := Day{Date: date, Max: atF(r.Daily.Max, i), Min: atF(r.Daily.Min, i),
			PrecipProb: at(r.Daily.PrecipProb, i), Code: at(r.Daily.Code, i), Label: lbl, Icon: ic}
		if i < len(r.Daily.Sunrise) {
			day.Sunrise = s.parseLocal(r.Daily.Sunrise[i])
		}
		if i < len(r.Daily.Sunset) {
			day.Sunset = s.parseLocal(r.Daily.Sunset[i])
		}
		w.Daily = append(w.Daily, day)
	}
	return w
}

func at(xs []int, i int) int {
	if i < len(xs) {
		return xs[i]
	}
	return 0
}

func atF(xs []float64, i int) float64 {
	if i < len(xs) {
		return xs[i]
	}
	return 0
}

// Describe maps a WMO weather code to a German label and an icon key the
// frontend knows (sun, moon, partly, partly-night, cloud, fog, drizzle, rain,
// sleet, snow, thunder).
func Describe(code int, isDay bool) (label, icon string) {
	switch code {
	case 0:
		if isDay {
			return "Sonnig", "sun"
		}
		return "Klar", "moon"
	case 1:
		if isDay {
			return "Überwiegend sonnig", "partly"
		}
		return "Überwiegend klar", "partly-night"
	case 2:
		if isDay {
			return "Teilweise bewölkt", "partly"
		}
		return "Teilweise bewölkt", "partly-night"
	case 3:
		return "Bedeckt", "cloud"
	case 45, 48:
		return "Nebel", "fog"
	case 51, 53, 55:
		return "Nieselregen", "drizzle"
	case 56, 57, 66, 67:
		return "Gefrierender Regen", "sleet"
	case 61:
		return "Leichter Regen", "rain"
	case 63:
		return "Regen", "rain"
	case 65:
		return "Starker Regen", "rain"
	case 71, 73, 75, 77:
		return "Schnee", "snow"
	case 80, 81:
		return "Regenschauer", "rain"
	case 82:
		return "Heftige Schauer", "rain"
	case 85, 86:
		return "Schneeschauer", "snow"
	case 95:
		return "Gewitter", "thunder"
	case 96, 99:
		return "Gewitter mit Hagel", "thunder"
	}
	return "Unbekannt", "cloud"
}
