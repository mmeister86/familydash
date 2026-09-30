package besteschule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRoutesAndAuth(t *testing.T) {
	var journalQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/students":
			w.Write([]byte(`{"data":[{"id":11,"forename":"Lukas"}]}`))
		case "/api/time-tables/current":
			if r.URL.Query().Get("include") != "lessons.times" {
				t.Errorf("timetable include: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":{"lessons":[{"weekday":3,"nr":1,"subject":{"name":"Mathe"},"time":{"from":"07:30","to":"08:15"}}]}}`))
		case "/api/substitution-plans/days":
			w.Write([]byte(`{"data":[]}`))
		case "/api/journal/lessons":
			journalQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNotFound) // force fallback
		case "/api/journal/weeks":
			w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := NewService(srv.URL+"/api", "good", berlin, nil)
	raw, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(journalQuery, "filter%5Bstudent%5D=11") {
		t.Errorf("journal not filtered by student: %s", journalQuery)
	}
	if _, ok := raw.Journal["11"]; !ok {
		t.Error("journal/weeks fallback not stored")
	}

	bad := NewService(srv.URL+"/api", "wrong", berlin, nil)
	bad.Refresh(context.Background())
	if sc := bad.Snapshot(); sc == nil || !strings.Contains(sc.Error, "Token") {
		t.Errorf("auth error not surfaced: %+v", sc)
	}
}
