// Package briefing writes the AI card of the wall display: a short briefing
// for today in the morning scene and an outlook on tomorrow in the evening
// scene. Code works out the facts (times, dates, what's cancelled, which
// bins go out), the model only picks and phrases them. Without a model, or
// when it fails, the same facts are turned into plain rule-based lines.
package briefing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"familydash/internal/scene"
)

// Item is one line of the card.
type Item struct {
	Section string `json:"section"` // "tonight" (evening: still to do tonight) or "day"
	Icon    string `json:"icon"`    // one of Icons, the frontend maps it to an emoji
	Who     string `json:"who,omitempty"`
	Color   string `json:"color,omitempty"` // the person's/calendar's color, set by code
	Text    string `json:"text"`
}

// Briefing is what /api/dashboard reports.
type Briefing struct {
	Kind      Kind      `json:"kind"`
	Date      string    `json:"date"` // the day it is about, YYYY-MM-DD
	Headline  string    `json:"headline,omitempty"`
	Items     []Item    `json:"items"`
	AI        bool      `json:"ai"` // false = rule-based fallback
	CreatedAt time.Time `json:"createdAt"`
	Error     string    `json:"error,omitempty"` // last model error (the fallback is shown meanwhile)
}

// Icons the model may choose from.
var Icons = []string{"schule", "frei", "ausfall", "test", "hausaufgabe", "termin", "fahrt", "essen", "brotbox",
	"muell", "regen", "kalt", "warm", "schnee", "sport", "todo", "geburtstag", "hinweis"}

const maxItems = 5

const systemPrompt = `Du schreibst die KI-Karte für ein Familien-Wanddisplay im Flur (Hochformat, ohne Touch).
Die Familie liest im Vorbeigehen: Eltern und Kinder.

Regeln:
- Verwende ausschließlich die Fakten aus dem JSON. Erfinde nichts dazu.
- Uhrzeiten und Tage genau so übernehmen, wie sie in den Fakten stehen. Nichts umrechnen.
- Weniger ist mehr: 2 bis 5 Einträge, das Wichtigste zuerst. Ein ruhiger Tag bekommt nur 1–2 Einträge.
- Wichtig ist, was etwas verändert: Ausfall/Vertretung (späterer Beginn, früheres Ende), Arbeiten/Tests,
  Termine mit Abholen oder Fahren, Müll, kein bestelltes Mittagessen, Wetter nur wenn es etwas ändert
  (Regen, Kälte, Frost, Schnee, Hitze), offene To-dos. Normale Stundenpläne nicht nacherzählen.
- "zeitgleich" nur erwähnen, wenn daraus plausibel ein Problem entsteht (z. B. wer fährt/holt ab).
- Jeder Eintrag: ein kurzer Satz, höchstens 70 Zeichen, ohne Emojis, ohne Namen am Satzanfang,
  wenn "who" schon die Person nennt. Ton: warm, knapp, alltagstauglich, keine Floskeln.
- "who": Vorname des Kindes, Name des Kalenders, oder "Familie".
- "headline": höchstens 45 Zeichen, fasst den Tag zusammen (z. B. "Ruhiger Freitag, ab Mittag Regen").
- Morgen-Briefing: alle Einträge mit section "day".
- Abend-Vorausschau: section "tonight" für das, was heute Abend noch vorbereitet werden sollte
  (Müll rausstellen, Sportbeutel packen, für eine Arbeit lernen, Brotbox wenn kein Essen bestellt,
  Kleidung fürs Wetter bereitlegen, Termine heute Abend); section "day" für das, was morgen ansteht.
- Am Wochenende gibt es keine Schule – dann geht es um Termine, Wetter und To-dos.`

func schema() map[string]any {
	str := func() map[string]any { return map[string]any{"type": "STRING"} }
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"headline": str(),
			"items": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"section": map[string]any{"type": "STRING", "enum": []string{"tonight", "day"}},
						"icon":    map[string]any{"type": "STRING", "enum": Icons},
						"who":     str(),
						"text":    str(),
					},
					"required":         []string{"section", "icon", "text"},
					"propertyOrdering": []string{"section", "icon", "who", "text"},
				},
			},
		},
		"required":         []string{"headline", "items"},
		"propertyOrdering": []string{"headline", "items"},
	}
}

// Service keeps the current briefing fresh. It wakes up every minute, looks
// at the scene that starts within Lead (morning → today, evening → tomorrow),
// and asks the model again only when the facts changed – at most every MinGap.
type Service struct {
	Model  *Gemini // nil = rule-based only
	Sched  *scene.Schedule
	Loc    *time.Location
	Lead   time.Duration // prepare this long before the scene starts
	MinGap time.Duration // at most one model call per briefing within this time
	Gather func(now time.Time) Data

	started time.Time
	mu      sync.RWMutex
	cur     *Briefing
	key     string // kind + date of cur
	hash    string // facts behind the last model answer for key
	lastAI  time.Time
	backoff time.Time
}

const (
	warmup       = 90 * time.Second // let the sources load once before the first call
	retryBackoff = 10 * time.Minute
)

func (s *Service) Run(ctx context.Context) {
	s.started = time.Now()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.tick(ctx, now)
		}
	}
}

// KindAt is the briefing due at now: the one of the scene running Lead from now.
func (s *Service) KindAt(now time.Time) (Kind, bool) {
	switch s.Sched.At(now.Add(s.Lead), s.Loc).Name {
	case scene.Morning:
		return Morning, true
	case scene.Evening:
		return Evening, true
	}
	return "", false
}

func (s *Service) tick(ctx context.Context, now time.Time) {
	if now.Sub(s.started) < warmup {
		return
	}
	kind, ok := s.KindAt(now)
	if !ok {
		return
	}
	// prepared Lead ahead: the target day follows the scene's start, not now
	target := Target(kind, now.Add(s.Lead), s.Loc)
	data := s.Gather(now)
	facts := BuildFacts(kind, now, target, s.Loc, data)
	key := string(kind) + "@" + target.Format(ymdLayout)
	hash := factsHash(facts)
	colors := Colors(data)

	s.mu.Lock()
	if key != s.key {
		s.key, s.hash, s.cur = key, "", nil
	}
	if hash == s.hash {
		s.mu.Unlock()
		return
	}
	fallback := Fallback(kind, target, facts, colors)
	if s.Model == nil {
		s.cur, s.hash = fallback, hash
		s.mu.Unlock()
		return
	}
	haveAI := s.cur != nil && s.cur.AI
	if now.Before(s.backoff) || (haveAI && now.Sub(s.lastAI) < s.MinGap) {
		if !haveAI {
			s.cur = fallback // keep the fallback current until the model answers
		}
		s.mu.Unlock()
		return
	}
	s.lastAI = now
	s.mu.Unlock()

	b, err := s.ask(ctx, kind, target, facts, colors)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != key {
		return // the day moved on while we waited
	}
	if err != nil {
		slog.Warn("briefing: model call failed", "kind", kind, "err", err)
		s.backoff = now.Add(retryBackoff)
		if s.cur == nil || !s.cur.AI {
			fallback.Error = err.Error()
			s.cur = fallback
		} else {
			s.cur.Error = err.Error()
		}
		return
	}
	s.cur, s.hash = b, hash
}

// Snapshot returns the current briefing (nil until the first one is ready).
func (s *Service) Snapshot() *Briefing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cur == nil {
		return nil
	}
	b := *s.cur
	b.Items = append([]Item(nil), s.cur.Items...)
	return &b
}

// Generate builds one briefing right away (for -briefing-preview). With no
// model it returns the rule-based version.
func (s *Service) Generate(ctx context.Context, kind Kind, now time.Time) (*Briefing, Facts, error) {
	target := Target(kind, now, s.Loc)
	data := s.Gather(now)
	facts := BuildFacts(kind, now, target, s.Loc, data)
	colors := Colors(data)
	if s.Model == nil {
		return Fallback(kind, target, facts, colors), facts, nil
	}
	b, err := s.ask(ctx, kind, target, facts, colors)
	return b, facts, err
}

func (s *Service) ask(ctx context.Context, kind Kind, target time.Time, f Facts, colors map[string]string) (*Briefing, error) {
	js, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	start := time.Now()
	raw, usage, err := s.Model.GenerateJSON(ctx, systemPrompt, "Fakten:\n"+string(js), schema())
	if err != nil {
		return nil, err
	}
	var out struct {
		Headline string `json:"headline"`
		Items    []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("answer is no valid JSON: %w", err)
	}
	b := &Briefing{Kind: kind, Date: target.Format(ymdLayout), Headline: strings.TrimSpace(out.Headline),
		Items: normalize(kind, out.Items, colors), AI: true, CreatedAt: time.Now()}
	slog.Info("briefing: generated", "kind", kind, "date", b.Date, "items", len(b.Items), "dur", time.Since(start).Round(time.Millisecond),
		"tokensIn", usage.Prompt, "tokensOut", usage.Output, "tokensThinking", usage.Thoughts)
	return b, nil
}

func normalize(kind Kind, in []Item, colors map[string]string) []Item {
	var tonight, day []Item
	for _, it := range in {
		it.Text = strings.TrimSpace(it.Text)
		if it.Text == "" {
			continue
		}
		if !contains(Icons, it.Icon) {
			it.Icon = "hinweis"
		}
		it.Who = strings.TrimSpace(it.Who)
		it.Color = colorOf(it.Who, colors)
		if kind == Evening && it.Section == "tonight" {
			tonight = append(tonight, it)
		} else {
			it.Section = "day"
			day = append(day, it)
		}
	}
	out := append(tonight, day...)
	if len(out) > maxItems {
		out = out[:maxItems]
	}
	return out
}

// Colors maps the names the card may mention to their dashboard colors:
// every calendar, and each child (its school calendar, else its lunch account).
func Colors(d Data) map[string]string {
	m := map[string]string{}
	for _, c := range d.Calendar.Calendars {
		m[c.Name] = c.Color
	}
	kid := func(name, calName string) {
		for _, c := range d.Calendar.Calendars {
			if c.Panel == "school" && ((calName != "" && strings.EqualFold(c.Name, calName)) || SameKid(c.Name, name)) {
				m[name] = c.Color
				return
			}
		}
		if d.Meals != nil {
			for _, ch := range d.Meals.Children {
				if SameKid(ch.Name, name) && ch.Color != "" {
					m[name] = ch.Color
					return
				}
			}
		}
	}
	if d.School != nil {
		for _, st := range d.School.Students {
			kid(st.Name, "")
		}
	}
	for _, c := range d.Timetables {
		kid(c.Name, c.Calendar)
	}
	return m
}

func colorOf(who string, colors map[string]string) string {
	if who == "" || strings.EqualFold(who, "Familie") {
		return ""
	}
	for name, c := range colors {
		if strings.EqualFold(name, who) {
			return c
		}
	}
	for name, c := range colors {
		if SameKid(name, who) {
			return c
		}
	}
	return ""
}

func factsHash(f Facts) string {
	f.Jetzt = "" // changes every minute, says nothing new
	js, _ := json.Marshal(f)
	sum := sha256.Sum256(js)
	return hex.EncodeToString(sum[:8])
}

// Fallback turns the facts into plain lines without a model.
func Fallback(kind Kind, target time.Time, f Facts, colors map[string]string) *Briefing {
	var items []Item
	add := func(section, icon, who, text string) {
		items = append(items, Item{Section: section, Icon: icon, Who: who, Text: text})
	}
	prep := "day" // things to get ready
	if kind == Evening {
		prep = "tonight"
	}
	when, take := "heute", "mitnehmen"
	if kind == Evening {
		when, take = "morgen", "bereitlegen"
	}

	if kind == Evening {
		for _, m := range f.Muell {
			name, _, _ := strings.Cut(m, ":")
			add("tonight", "muell", "Familie", name+" heute Abend rausstellen")
		}
	}
	for _, k := range f.Kinder {
		for _, a := range k.Arbeiten {
			if strings.HasSuffix(a, "– "+when) {
				add(prep, "test", k.Name, strings.TrimSpace(strings.TrimSuffix(a, "– "+when))+" "+when)
			}
		}
		if strings.HasPrefix(k.Essen, "nichts bestellt") {
			add(prep, "brotbox", k.Name, "Kein Mittagessen bestellt – Brotbox einpacken")
		}
		if k.Schule != nil && k.Schule.Sport && kind == Evening {
			add("tonight", "sport", k.Name, "Sportsachen packen")
		}
	}
	if w := f.Wetter; w != nil {
		switch {
		case w.Schnee:
			add(prep, "schnee", "Familie", "Schnee möglich – Winterstiefel "+take)
		case w.RegenMax >= 50 && w.RegenAb != "":
			add(prep, "regen", "Familie", fmt.Sprintf("Regen ab %s – Regenjacke %s", w.RegenAb, take))
		case w.Frost:
			add(prep, "kalt", "Familie", fmt.Sprintf("Frost, nur %d °C – Mütze und Handschuhe", w.Min))
		case w.Max >= 27:
			add(prep, "warm", "Familie", fmt.Sprintf("Bis %d °C – Sonnencreme und Trinkflasche", w.Max))
		}
	}
	for _, k := range f.Kinder {
		if sd := k.Schule; sd != nil {
			if len(sd.Ausfall) > 0 {
				txt := "Entfällt: " + strings.Join(sd.Ausfall, ", ")
				if sd.Beginn != "" {
					txt += " – Beginn " + sd.Beginn
				}
				add("day", "ausfall", k.Name, txt)
			}
			if len(sd.Vertretung) > 0 {
				add("day", "schule", k.Name, "Vertretung: "+strings.Join(sd.Vertretung, ", "))
			}
		}
	}
	for i, a := range f.Termine {
		if i == 3 {
			break
		}
		add("day", "termin", a.Kalender, a.Zeit+" "+a.Titel)
	}
	return &Briefing{Kind: kind, Date: target.Format(ymdLayout), Items: normalize(kind, items, colors), CreatedAt: time.Now()}
}
