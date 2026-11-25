package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const sample = `{
 "current":{"temperature_2m":14.3,"apparent_temperature":12.1,"weather_code":61,"wind_speed_10m":11.2,"is_day":1},
 "hourly":{"time":["2026-09-30T10:00","2026-09-30T11:00","2026-09-30T12:00"],
   "temperature_2m":[13,14,15],"precipitation_probability":[40,60,20],"weather_code":[3,61,2],"is_day":[1,1,1]},
 "daily":{"time":["2026-09-30","2026-10-01"],"weather_code":[61,0],"temperature_2m_max":[16.2,18],
   "temperature_2m_min":[8.1,6],"precipitation_probability_max":[70,5],
   "sunrise":["2026-09-30T07:03","2026-10-01T07:05"],"sunset":["2026-09-30T18:49","2026-10-01T18:47"]}}`

func TestFetchAndTransform(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("timezone") != "Europe/Berlin" {
			t.Errorf("timezone not passed: %s", r.URL.RawQuery)
		}
		w.Write([]byte(sample))
	}))
	defer srv.Close()

	loc, _ := time.LoadLocation("Europe/Berlin")
	s := NewService(50.81, 12.39, "Crimmitschau", loc)
	s.BaseURL = srv.URL
	s.Refresh(context.Background())

	w := s.Snapshot()
	if w == nil || w.Error != "" {
		t.Fatalf("no weather: %+v", w)
	}
	if w.Current.Label != "Leichter Regen" || w.Current.Icon != "rain" {
		t.Errorf("current: %+v", w.Current)
	}
	if len(w.Daily) != 2 || w.Daily[1].Icon != "sun" || w.Daily[0].Sunrise.Hour() != 7 {
		t.Errorf("daily: %+v", w.Daily)
	}
}

func TestKeepsLastGoodOnError(t *testing.T) {
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(sample))
	}))
	defer srv.Close()
	s := NewService(1, 1, "", time.UTC)
	s.BaseURL = srv.URL
	s.Refresh(context.Background())
	ok = false
	s.Refresh(context.Background())
	w := s.Snapshot()
	if w.Current.Temp != 14.3 || w.Error == "" {
		t.Fatalf("expected stale data + error, got %+v", w)
	}
}

// hourlyJSON builds an Open-Meteo style response with one entry per local
// hour for the given date strings (24 per day unless extra hours are listed).
func hourlyJSON(times []string) string {
	var ts, temp, prob, code, day []string
	for i, v := range times {
		ts = append(ts, `"`+v+`"`)
		temp = append(temp, strconv.Itoa(i%30))
		prob = append(prob, strconv.Itoa(i%100))
		code = append(code, "3")
		day = append(day, "1")
	}
	j := strings.Join
	return `{"current":{"temperature_2m":1,"apparent_temperature":1,"weather_code":3,"wind_speed_10m":1,"is_day":1},` +
		`"hourly":{"time":[` + j(ts, ",") + `],"temperature_2m":[` + j(temp, ",") + `],"precipitation_probability":[` + j(prob, ",") +
		`],"weather_code":[` + j(code, ",") + `],"is_day":[` + j(day, ",") + `]},"daily":{"time":[]}}`
}

func dayHours(date string, n int) []string {
	var out []string
	for h := 0; h < n; h++ {
		out = append(out, fmt.Sprintf("%sT%02d:00", date, h))
	}
	return out
}

func TestTodayCoversWholeDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := NewService(1, 1, "", loc)
	var r apiResponse
	times := append(dayHours("2026-09-30", 24), dayHours("2026-10-01", 24)...)
	if err := json.Unmarshal([]byte(hourlyJSON(times)), &r); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 13, 20, 0, 0, loc)
	w := s.transform(&r, now)
	if len(w.Today) != 24 {
		t.Fatalf("Today = %d entries, want 24", len(w.Today))
	}
	if h := w.Today[0].Time; h.Hour() != 0 || h.Day() != 30 {
		t.Errorf("Today[0] = %v, want 00:00 of 30th", h)
	}
	if h := w.Today[23].Time; h.Hour() != 23 || h.Day() != 30 {
		t.Errorf("Today[23] = %v, want 23:00 of 30th", h)
	}
	if w.Today[5].Temp != 5 || w.Today[5].PrecipProb != 5 || w.Today[5].Icon != "cloud" {
		t.Errorf("Today[5] not filled: %+v", w.Today[5])
	}
	if len(w.Hourly) != 12 || w.Hourly[0].Time.Hour() != 13 || w.Hourly[11].Time.Hour() != 0 {
		t.Errorf("Hourly changed: %d entries, first %v", len(w.Hourly), w.Hourly[0].Time)
	}
}

func TestTodayOnDSTEndDayHas25Hours(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	s := NewService(1, 1, "", loc)
	var r apiResponse
	// Last Sunday of October 2026 is the 25th; the 02:00 hour occurs twice.
	today := dayHours("2026-10-25", 24)
	today = append(today[:3], append([]string{"2026-10-25T02:00"}, today[3:]...)...)
	times := append(append(dayHours("2026-10-24", 24), today...), dayHours("2026-10-26", 24)...)
	if err := json.Unmarshal([]byte(hourlyJSON(times)), &r); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 13, 20, 0, 0, loc)
	w := s.transform(&r, now)
	if len(w.Today) != 25 {
		t.Fatalf("Today = %d entries, want 25", len(w.Today))
	}
}
