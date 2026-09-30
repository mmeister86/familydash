package bring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeBring(t *testing.T, logins *int, revokeOnce *bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/v2/bringauth", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Header.Get("X-BRING-API-KEY") == "" {
			t.Error("missing api key header")
		}
		if r.Form.Get("password") != "geheim" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		*logins++
		json.NewEncoder(w).Encode(map[string]any{"uuid": "u1", "bringListUUID": "L1", "access_token": "tok", "expires_in": 3600})
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if *revokeOnce {
			*revokeOnce = false
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-BRING-USER-UUID") != "u1" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /rest/bringusers/u1/lists", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			w.Write([]byte(`{"lists":[{"listUuid":"L1","name":"Zuhause"},{"listUuid":"L2","name":"Baumarkt"}]}`))
		}
	})
	mux.HandleFunc("GET /rest/v2/bringlists/L1", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			w.Write([]byte(`{"uuid":"L1","items":{"purchase":[{"uuid":"a","itemId":"Poulet","specification":"500 g"},{"uuid":"b","itemId":"Milch","specification":""}],"recently":[{"itemId":"Brot"}]}}`))
		}
	})
	mux.HandleFunc("GET /rest/v2/bringlists/L2", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			w.Write([]byte(`{"items":{"purchase":[{"itemId":"Schrauben","specification":"M4"}],"recently":[]}}`))
		}
	})
	mux.HandleFunc("GET /locale/articles.de-DE.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Poulet":"Hähnchen","Milch":"Milch"}`))
	})
	return httptest.NewServer(mux)
}

func newTest(srv *httptest.Server, list, password string) *Service {
	s := NewService("a@b.de", password, list, "de-DE")
	s.BaseURL = srv.URL + "/rest/"
	s.LocaleURL = srv.URL + "/locale/"
	return s
}

func TestDefaultListWithTranslation(t *testing.T) {
	logins, revoke := 0, false
	srv := fakeBring(t, &logins, &revoke)
	defer srv.Close()

	s := newTest(srv, "", "geheim")
	s.Refresh(context.Background())
	l := s.Snapshot()
	if l == nil || l.Error != "" || l.Name != "Zuhause" || len(l.Items) != 2 {
		t.Fatalf("list: %+v", l)
	}
	if l.Items[0] != (Item{Name: "Hähnchen", Spec: "500 g"}) {
		t.Errorf("translation/spec: %+v", l.Items[0])
	}
}

func TestNamedListAndRelogin(t *testing.T) {
	logins, revoke := 0, false
	srv := fakeBring(t, &logins, &revoke)
	defer srv.Close()

	s := newTest(srv, "baumarkt", "geheim")
	s.Refresh(context.Background())
	revoke = true // server revokes the token → client must log in again transparently
	s.Refresh(context.Background())
	l := s.Snapshot()
	if l.Error != "" || l.Name != "Baumarkt" || l.Items[0].Name != "Schrauben" {
		t.Fatalf("list: %+v", l)
	}
	if logins != 2 {
		t.Errorf("expected re-login, logins=%d", logins)
	}
}

func TestErrors(t *testing.T) {
	logins, revoke := 0, false
	srv := fakeBring(t, &logins, &revoke)
	defer srv.Close()

	bad := newTest(srv, "", "falsch")
	bad.Refresh(context.Background())
	if l := bad.Snapshot(); l == nil || !strings.Contains(l.Error, "Passwort") {
		t.Errorf("wrong password: %+v", l)
	}

	missing := newTest(srv, "Drogerie", "geheim")
	missing.Refresh(context.Background())
	if l := missing.Snapshot(); !strings.Contains(l.Error, "Zuhause, Baumarkt") {
		t.Errorf("missing list should name existing lists: %+v", l)
	}
}
