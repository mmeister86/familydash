package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
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
