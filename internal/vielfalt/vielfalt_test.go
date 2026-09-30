package vielfalt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Shaped like the real portal markup (one menu tile each).
const tileOrdered = `<div class="order-menu-node-shopping-cart-quantity">
  <button id="menu_quantity_2026-10-06_7_338000270" class="menuplan-checkbox bg-ok" tabindex="4"
    data-quantity-ordered="1" data-quantity-in-shopping-cart data-order-status="2" data-price="5,51€"
    data-date="06.10.2026" data-name="Milchreis mit Erdbeerso&szlig;e &amp; Zucker / Zimt (V)"
    onclick="IbsOrderMenu.clickMenuCheckbox(2, '1', 'ibs4_2019_delegate' , true , '7' );"
    aria-label="Für den 06.10.2026 wurde das Milchreis bestellt."><img src="x.png"></button></div>`

const tileOpen = `<button id="menu_quantity_2026-10-06_7_338000271" class="menuplan-checkbox" tabindex="4"
    data-quantity-ordered data-quantity-in-shopping-cart data-order-status="0" data-price="5,51€"
    data-date="06.10.2026" data-name="Grüner Erbseneintopf mit Kartoffeln, Sellerie und Möhre (V) | Bio-Vollkornbrot"
    onclick="IbsOrderMenu.clickMenuCheckbox(0, '', 'ibs4_2019_delegate' , true , '7' );">x</button>`

func tile(date, name string, ordered bool) string {
	q := ""
	if ordered {
		q = "1"
	}
	return fmt.Sprintf(`<button class="menuplan-checkbox" data-quantity-ordered="%s" data-date="%s" data-name="%s"></button>`, q, date, name)
}

func TestParseWeek(t *testing.T) {
	page := `<div class="order-menu-node">` + tileOrdered + tileOpen +
		tile("07.10.2026", "Wildlachspfanne", false) + // offered, nothing ordered
		`<button class="other" data-date="08.10.2026" data-quantity-ordered="1" data-name="kein Menü">` +
		`<div class="order-menu-node-disabled"> </div>`
	days := ParseWeek(page)
	if len(days) != 2 {
		t.Fatalf("want 2 days, got %+v", days)
	}
	if days[0].Date != "2026-10-06" || len(days[0].Ordered) != 1 {
		t.Fatalf("day 1: %+v", days[0])
	}
	if got := days[0].Ordered[0].Name; got != "Milchreis mit Erdbeersoße & Zucker / Zimt (V)" {
		t.Errorf("name %q", got)
	}
	if days[1].Date != "2026-10-07" || len(days[1].Ordered) != 0 {
		t.Errorf("day 2: %+v", days[1])
	}
}

func TestSplitDish(t *testing.T) {
	d := SplitDish("Grüner Erbseneintopf mit Kartoffeln (V)  |  Bio-Vollkornbrot | Obst")
	if d.Name != "Grüner Erbseneintopf mit Kartoffeln (V)" || d.Side != "Bio-Vollkornbrot · Obst" {
		t.Errorf("%+v", d)
	}
}

func TestFetch(t *testing.T) {
	var weeks []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "laravel_session", Value: "abc"})
	})
	mux.HandleFunc("POST /frontend/login", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("username") != "123" || r.Form.Get("password") != "geheim" {
			w.Write([]byte(`{"token":""}`))
			return
		}
		if c, err := r.Cookie("laravel_session"); err != nil || c.Value != "abc" {
			t.Error("session cookie not sent")
		}
		w.Write([]byte(`{"token":"TOK","iframe":"ibs4_2019_delegate","adresse":{"Name1":"Meister Lukas"}}`))
	})
	mux.HandleFunc("GET /ibs4_2019_delegate/Mealplan/Weekplan", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Auth") != "TOK" || r.Header.Get("Authorization") != "Bearer TOK" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		weeks = append(weeks, r.URL.Query().Get("year")+"-"+r.URL.Query().Get("week"))
		switch r.URL.Query().Get("week") {
		case "40":
			w.Write([]byte(tile("29.09.2026", "Gestern", true) + tile("02.10.2026", "Makkaroni | Schoko", true)))
		case "41":
			w.Write([]byte(tileOrdered + tileOpen + tile("07.10.2026", "Wildlachspfanne", false)))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	loc, _ := time.LoadLocation("Europe/Berlin")
	s := NewService(nil, loc)
	s.LoginURL, s.IBSURL = srv.URL, srv.URL
	now := time.Date(2026, 10, 2, 17, 0, 0, 0, loc) // Friday, KW 40

	c, err := s.Fetch(context.Background(), Account{User: "123", Password: "geheim"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(weeks, ",") != "2026-40,2026-41" {
		t.Errorf("weeks %v", weeks)
	}
	if c.Name != "Meister Lukas" {
		t.Errorf("name %q", c.Name)
	}
	var got []string
	for _, d := range c.Days {
		got = append(got, fmt.Sprintf("%s:%d", d.Date, len(d.Ordered)))
	}
	if strings.Join(got, " ") != "2026-10-02:1 2026-10-06:1 2026-10-07:0" {
		t.Errorf("days %v", got)
	}
	if c.Days[0].Ordered[0].Side != "Schoko" {
		t.Errorf("side %+v", c.Days[0].Ordered[0])
	}

	if _, err := s.Fetch(context.Background(), Account{User: "123", Password: "falsch"}, now); err == nil ||
		!strings.Contains(err.Error(), "Kundennummer/Passwort") {
		t.Errorf("wrong password: %v", err)
	}
}
