package vielfalt

import (
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseWeek reads the meal plan HTML of one week. Every offered menu is a
// <button class="menuplan-checkbox …"> with data attributes, e.g.
//
//	<button class="menuplan-checkbox bg-ok" data-quantity-ordered="1"
//	        data-order-status="2" data-date="06.10.2026"
//	        data-name="Milchreis mit Erdbeersoße & Zucker / Zimt (V)">
//
// A menu counts as ordered when data-quantity-ordered ≥ 1. Ticked but unsaved
// menus only sit in the shopping cart (data-quantity-in-shopping-cart) and
// are ignored. Days without any menu (weekend, "Bestellpause") are left out.
func ParseWeek(page string) []Day {
	days := map[string]*Day{}
	for _, attrs := range buttons(page) {
		if !hasClass(attrs["class"], "menuplan-checkbox") {
			continue
		}
		date, err := time.Parse("02.01.2006", strings.TrimSpace(attrs["data-date"]))
		if err != nil {
			continue
		}
		key := date.Format("2006-01-02")
		d := days[key]
		if d == nil {
			d = &Day{Date: key, Ordered: []Dish{}}
			days[key] = d
		}
		if n, _ := strconv.Atoi(strings.TrimSpace(attrs["data-quantity-ordered"])); n > 0 {
			d.Ordered = append(d.Ordered, SplitDish(attrs["data-name"]))
		}
	}
	out := make([]Day, 0, len(days))
	for _, d := range days {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// SplitDish separates the main dish from the sides:
// "Grüner Erbseneintopf mit Kartoffeln (V) | Bio-Vollkornbrot"
// → {"Grüner Erbseneintopf mit Kartoffeln (V)", "Bio-Vollkornbrot"}.
func SplitDish(name string) Dish {
	parts := strings.Split(strings.Join(strings.Fields(name), " "), "|")
	d := Dish{Name: strings.TrimSpace(parts[0])}
	var sides []string
	for _, p := range parts[1:] {
		if p = strings.TrimSpace(p); p != "" {
			sides = append(sides, p)
		}
	}
	d.Side = strings.Join(sides, " · ")
	return d
}

func hasClass(class, want string) bool {
	for _, c := range strings.Fields(class) {
		if c == want {
			return true
		}
	}
	return false
}

// buttons returns the attributes of every <button> start tag. A tiny
// quote-aware scanner is enough here and keeps the binary dependency-free.
func buttons(page string) []map[string]string {
	var out []map[string]string
	lower := strings.ToLower(page)
	for i := 0; ; {
		j := strings.Index(lower[i:], "<button")
		if j < 0 {
			return out
		}
		pos := i + j + len("<button")
		attrs, end := attributes(page, pos)
		out = append(out, attrs)
		i = end
	}
}

// attributes parses name="value" pairs from pos up to the closing '>' of
// the tag and returns them with the index after the tag.
func attributes(s string, pos int) (map[string]string, int) {
	attrs := map[string]string{}
	i := pos
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == '>' {
			return attrs, i + 1
		}
		if s[i] == '/' {
			i++
			continue
		}
		start := i
		for i < len(s) && !isSpace(s[i]) && s[i] != '=' && s[i] != '>' {
			i++
		}
		name := strings.ToLower(s[start:i])
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			attrs[name] = ""
			continue
		}
		i++ // '='
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		var val string
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			q := s[i]
			i++
			vs := i
			for i < len(s) && s[i] != q {
				i++
			}
			val = s[vs:i]
			i++ // closing quote
		} else {
			vs := i
			for i < len(s) && !isSpace(s[i]) && s[i] != '>' {
				i++
			}
			val = s[vs:i]
		}
		attrs[name] = html.UnescapeString(val)
	}
	return attrs, len(s)
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' || b == '\f' }
