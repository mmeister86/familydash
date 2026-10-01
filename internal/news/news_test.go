package news

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Crimmitschau - Google News</title>
<item><title>Stadtrat beschließt neuen Spielplatz - Freie Presse</title>
  <pubDate>Thu, 01 Oct 2026 07:30:00 GMT</pubDate><source url="https://www.freiepresse.de">Freie Presse</source></item>
<item><title>Alte Meldung - Blick</title>
  <pubDate>Mon, 21 Sep 2026 07:30:00 GMT</pubDate><source url="https://www.blick.de">Blick</source></item>
<item><title>Ohne Quelle – Bindestrich im Titel - MDR</title>
  <pubDate>Thu, 01 Oct 2026 09:00:00 GMT</pubDate></item>
</channel></rss>`

func TestParse(t *testing.T) {
	items, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0].Title != "Stadtrat beschließt neuen Spielplatz" || items[0].Source != "Freie Presse" {
		t.Errorf("item 0: %+v", items[0])
	}
	if items[0].Published.UTC().Hour() != 7 {
		t.Errorf("date: %v", items[0].Published)
	}
	if items[2].Title != "Ohne Quelle – Bindestrich im Titel" || items[2].Source != "MDR" {
		t.Errorf("item 2: %+v", items[2])
	}
}

func TestBuildDedupAgeAndCap(t *testing.T) {
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return now.Add(-time.Duration(n) * time.Hour) }
	feeds := []Feed{{"Region", "a"}, {"Region", "b"}, {"Deutschland", "c"}}
	cache := map[string][]Item{
		"a": {{Title: "Spielplatz eröffnet", Published: h(5)}},
		"b": {{Title: "Spielplatz eröffnet!", Published: h(4)}, {Title: "Kreistag tagt", Published: h(1)}, {Title: "uralt", Published: h(100)}},
		"c": {{Title: "Eins", Published: h(1)}, {Title: "Zwei", Published: h(2)}, {Title: "Drei", Published: h(3)}},
	}
	s := Build(feeds, cache, now, 48*time.Hour, 2)
	if len(s.Groups) != 2 || s.Groups[0].Name != "Region" || s.Groups[1].Name != "Deutschland" {
		t.Fatalf("groups: %+v", s.Groups)
	}
	r := s.Groups[0].Items
	if len(r) != 2 || r[0].Title != "Kreistag tagt" || r[1].Title != "Spielplatz eröffnet" {
		t.Errorf("region: %+v", r)
	}
	if d := s.Groups[1].Items; len(d) != 2 || d[0].Title != "Eins" {
		t.Errorf("deutschland: %+v", d)
	}
}

func TestGoogleSearchQuotesPhrases(t *testing.T) {
	u, _ := url.Parse(GoogleSearch("Landkreis Zwickau", 2))
	if q := u.Query().Get("q"); q != `"Landkreis Zwickau" when:2d` {
		t.Errorf("q = %q", q)
	}
}

func TestRefreshKeepsLastGoodOnError(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "nope", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(sample))
	}))
	defer srv.Close()
	s := NewService([]Feed{{"Region", srv.URL}}, 0, 10)
	s.Refresh(t.Context())
	if n := len(s.Snapshot().Groups[0].Items); n != 3 {
		t.Fatalf("first refresh: %d items", n)
	}
	fail = true
	s.Refresh(t.Context())
	snap := s.Snapshot()
	if len(snap.Groups[0].Items) != 3 || snap.Error == "" {
		t.Errorf("after error: %d items, error %q", len(snap.Groups[0].Items), snap.Error)
	}
}
