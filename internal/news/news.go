// Package news reads a few RSS feeds (by default Google News: local searches
// plus Germany's top stories) and reduces them to headlines for the wall:
// title, source and time – no links, the display has no touch.
package news

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Feed is one RSS source. Feeds with the same Group end up in one section.
type Feed struct {
	Group string
	URL   string
}

type Item struct {
	Title     string    `json:"title"`
	Source    string    `json:"source,omitempty"`
	Published time.Time `json:"published"`
}

type Group struct {
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

type Snapshot struct {
	Groups    []Group   `json:"groups"`
	UpdatedAt time.Time `json:"updatedAt"`
	Error     string    `json:"error,omitempty"`
}

const googleBase = "https://news.google.com/rss"

// GoogleTop is Google News' top stories for Germany.
func GoogleTop() string { return googleBase + "?hl=de&gl=DE&ceid=DE:de" }

// GoogleSearch is a Google News search feed limited to the last `days` days –
// without the limit, small towns mostly bring up years-old articles.
func GoogleSearch(term string, days int) string {
	term = strings.TrimSpace(term)
	if strings.ContainsRune(term, ' ') && !strings.HasPrefix(term, `"`) {
		term = `"` + term + `"`
	}
	q := url.Values{"q": {fmt.Sprintf("%s when:%dd", term, days)}, "hl": {"de"}, "gl": {"DE"}, "ceid": {"DE:de"}}
	return googleBase + "/search?" + q.Encode()
}

type Service struct {
	Feeds    []Feed
	MaxAge   time.Duration // older headlines are dropped
	PerGroup int
	client   *http.Client

	mu    sync.RWMutex
	cache map[string][]Item // last good items per feed URL
	last  *Snapshot
}

func NewService(feeds []Feed, maxAge time.Duration, perGroup int) *Service {
	return &Service{Feeds: feeds, MaxAge: maxAge, PerGroup: perGroup,
		client: &http.Client{Timeout: 15 * time.Second}, cache: map[string][]Item{}}
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

// Refresh fetches every feed. A feed that fails keeps its last good items,
// so one hiccup at Google doesn't empty the card.
func (s *Service) Refresh(ctx context.Context) {
	var errs []string
	fresh := map[string][]Item{}
	for _, f := range s.Feeds {
		items, err := s.fetch(ctx, f.URL)
		if err != nil {
			slog.Warn("news feed failed", "group", f.Group, "err", err)
			errs = append(errs, fmt.Sprintf("%s: %v", f.Group, err))
			continue
		}
		fresh[f.URL] = items
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for u, items := range fresh {
		s.cache[u] = items
	}
	snap := Build(s.Feeds, s.cache, time.Now(), s.MaxAge, s.PerGroup)
	if len(errs) > 0 {
		snap.Error = strings.Join(errs, "; ")
	}
	s.last = &snap
}

func (s *Service) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

func (s *Service) fetch(ctx context.Context, u string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "familydash/1 (+https://github.com/mmeister86/familydash)")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return Parse(io.LimitReader(res.Body, 4<<20))
}

type rss struct {
	Items []struct {
		Title   string `xml:"title"`
		PubDate string `xml:"pubDate"`
		Source  string `xml:"source"`
	} `xml:"channel>item"`
}

// Parse reads an RSS 2.0 document.
func Parse(r io.Reader) ([]Item, error) {
	var doc rss
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("RSS: %w", err)
	}
	out := make([]Item, 0, len(doc.Items))
	for _, it := range doc.Items {
		title, source := cleanTitle(strings.TrimSpace(it.Title), strings.TrimSpace(it.Source))
		if title == "" {
			continue
		}
		out = append(out, Item{Title: title, Source: source, Published: parseDate(it.PubDate)})
	}
	return out, nil
}

// Google News appends " - Source" to every headline; it's shown separately.
func cleanTitle(title, source string) (string, string) {
	if source != "" {
		title = strings.TrimSpace(strings.TrimSuffix(title, " - "+source))
		return title, source
	}
	if i := strings.LastIndex(title, " - "); i > 0 && len(title)-i < 45 {
		return strings.TrimSpace(title[:i]), strings.TrimSpace(title[i+3:])
	}
	return title, ""
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Build groups the cached items in feed order, newest first, drops old and
// duplicate headlines (a Crimmitschau story usually shows up in the district
// feed too – the first group keeps it) and caps each group.
func Build(feeds []Feed, cache map[string][]Item, now time.Time, maxAge time.Duration, perGroup int) Snapshot {
	snap := Snapshot{Groups: []Group{}, UpdatedAt: now}
	seen := map[string]bool{}
	idx := map[string]int{}
	for _, f := range feeds {
		i, ok := idx[f.Group]
		if !ok {
			i = len(snap.Groups)
			idx[f.Group] = i
			snap.Groups = append(snap.Groups, Group{Name: f.Group, Items: []Item{}})
		}
		items := append([]Item(nil), cache[f.URL]...)
		sort.SliceStable(items, func(a, b int) bool { return items[a].Published.After(items[b].Published) })
		for _, it := range items {
			if maxAge > 0 && !it.Published.IsZero() && now.Sub(it.Published) > maxAge {
				continue
			}
			key := normalize(it.Title)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			snap.Groups[i].Items = append(snap.Groups[i].Items, it)
		}
	}
	for i := range snap.Groups {
		g := &snap.Groups[i]
		sort.SliceStable(g.Items, func(a, b int) bool { return g.Items[a].Published.After(g.Items[b].Published) })
		if perGroup > 0 && len(g.Items) > perGroup {
			g.Items = g.Items[:perGroup]
		}
	}
	return snap
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
