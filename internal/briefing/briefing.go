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
- To-dos: "wer" ist die zuständige Person (fehlt = ganze Familie) und gehört dann in "who".
  Überfälliges zuerst. Abends: "heute noch offen" in section "tonight", "morgen" in section "day".
- "bestaetigungen_offen" > 0: kurz daran erinnern, dass Eltern abgehakte Aufgaben bestätigen sollen.
- "punkte": höchstens gelegentlich und nur als Ansporn erwähnen, Kinder nie miteinander vergleichen.
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

// Service keeps the briefings fresh. It wakes up every minute, looks at the
// scene that starts within Lead (morning → today, evening → tomorrow), and
// asks the model again only when the facts changed – at most every MinGap.
// Morning and evening each keep their own card, so ?scene=… on a laptop can
// look at the other one without disturbing the wall.
type Service struct {
	Model  *Gemini // nil = rule-based only
	Sched  *scene.Schedule
	Loc    *time.Location
	Lead   time.Duration // prepare this long before the scene starts
	MinGap time.Duration // at most one model call per briefing within this time
	Gather func(now time.Time) Data

	started time.Time
	mu      sync.Mutex
	slots   map[Kind]*slot
}

type slot struct {
	key     string // kind + target date of cur
	cur     *Briefing
	hash    string // facts behind cur (model answer or, without a model, the rules)
	lastAI  time.Time
	backoff time.Time
	busy    bool // a model call is running
}

const (
	warmup       = 90 * time.Second // let the sources load once before the first call
	retryBackoff = 10 * time.Minute
	previewWait  = 20 * time.Second // ?scene=…: wait this long for a first answer
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
	return kindOf(s.Sched.At(now.Add(s.Lead), s.Loc).Name)
}

func kindOf(sceneName string) (Kind, bool) {
	switch sceneName {
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
	s.refresh(ctx, kind, Target(kind, now.Add(s.Lead), s.Loc), now)
}

func (s *Service) slot(kind Kind) *slot {
	if s.slots == nil {
		s.slots = map[Kind]*slot{}
	}
	sl := s.slots[kind]
	if sl == nil {
		sl = &slot{}
		s.slots[kind] = sl
	}
	return sl
}

// refresh brings the card of one kind up to date for target.
func (s *Service) refresh(ctx context.Context, kind Kind, target, now time.Time) {
	data := s.Gather(now)
	facts := BuildFacts(kind, now, target, s.Loc, data)
	key := string(kind) + "@" + target.Format(ymdLayout)
	hash := factsHash(facts)
	colors := Colors(data)

	s.mu.Lock()
	sl := s.slot(kind)
	if key != sl.key {
		*sl = slot{key: key}
	}
	if hash == sl.hash || sl.busy {
		s.mu.Unlock()
		return
	}
	fallback := Fallback(kind, target, facts, colors)
	if s.Model == nil {
		sl.cur, sl.hash = fallback, hash
		s.mu.Unlock()
		return
	}
	haveAI := sl.cur != nil && sl.cur.AI
	if !haveAI {
		sl.cur = fallback // show the rule-based card until the model answers
	}
	if now.Before(sl.backoff) || (haveAI && now.Sub(sl.lastAI) < s.MinGap) {
		s.mu.Unlock()
		return
	}
	sl.busy, sl.lastAI = true, now
	s.mu.Unlock()

	b, err := s.ask(ctx, kind, target, facts, colors)

	s.mu.Lock()
	defer s.mu.Unlock()
	sl.busy = false
	if sl.key != key {
		return // the day moved on while we waited
	}
	if err != nil {
		slog.Warn("briefing: model call failed", "kind", kind, "err", err)
		sl.backoff = now.Add(retryBackoff)
		if sl.cur != nil {
			sl.cur.Error = err.Error()
		}
		return
	}
	sl.cur, sl.hash = b, hash
}

// Snapshot returns the card for the wall: the running scene's, or the one
// being prepared for the next scene. nil = nothing (yet).
func (s *Service) Snapshot() *Briefing { return s.snapshotAt(time.Now()) }

func (s *Service) snapshotAt(now time.Time) *Briefing {
	kind, ok := kindOf(s.Sched.At(now, s.Loc).Name)
	if !ok {
		if kind, ok = s.KindAt(now); !ok {
			return nil
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.slot(kind).cur)
}

// For returns the card of one kind on request (/api/dashboard?scene=evening),
// whatever the time: today's morning briefing or the outlook on tomorrow.
// The first request waits up to previewWait for the model; later ones answer
// at once and update in the background.
func (s *Service) For(kind Kind, now time.Time) *Briefing {
	target := Target(kind, now, s.Loc)
	key := string(kind) + "@" + target.Format(ymdLayout)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.refresh(context.Background(), kind, target, now)
	}()
	s.mu.Lock()
	ready := s.slot(kind).key == key && s.slot(kind).cur != nil && s.slot(kind).cur.AI
	s.mu.Unlock()
	if !ready {
		select {
		case <-done:
		case <-time.After(previewWait):
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sl := s.slot(kind); sl.key == key {
		return copyOf(sl.cur)
	}
	return nil
}

// Cards returns the current card of every kind (morning and evening), for
// the family app. Empty slots are left out.
func (s *Service) Cards() []*Briefing {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Briefing
	for _, k := range []Kind{Morning, Evening} {
		if sl := s.slots[k]; sl != nil && sl.cur != nil {
			out = append(out, copyOf(sl.cur))
		}
	}
	return out
}

func copyOf(b *Briefing) *Briefing {
	if b == nil {
		return nil
	}
	c := *b
	c.Items = append([]Item(nil), b.Items...)
	return &c
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
	for i, t := range f.Todos {
		if i == 2 {
			break
		}
		section := "day"
		if kind == Evening && (t.Faellig == "heute noch offen" || strings.HasPrefix(t.Faellig, "überfällig")) {
			section = "tonight"
		}
		who := t.Wer
		if who == "" {
			who = "Familie"
		}
		add(section, "todo", who, t.Titel)
	}
	return &Briefing{Kind: kind, Date: target.Format(ymdLayout), Items: normalize(kind, items, colors), CreatedAt: time.Now()}
}
