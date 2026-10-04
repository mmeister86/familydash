package familyapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"familydash/internal/briefing"
)

// BriefingPayload is the body of POST /ingest/briefing. The app upserts by
// kind + date. Text is ready-to-render Markdown; headline and items carry
// the same content structured, for a nicer card.
type BriefingPayload struct {
	Kind        string         `json:"kind"` // morning | evening
	Date        string         `json:"date"` // the day it's about, YYYY-MM-DD
	Text        string         `json:"text"`
	Headline    string         `json:"headline,omitempty"`
	Items       []BriefingItem `json:"items"`
	AI          bool           `json:"ai"` // false = rule-based fallback
	GeneratedAt int64          `json:"generatedAt"`
}

type BriefingItem struct {
	Section string `json:"section"` // tonight | day
	Icon    string `json:"icon"`
	Who     string `json:"who,omitempty"`
	Color   string `json:"color,omitempty"`
	Text    string `json:"text"`
}

func NewBriefingPayload(b *briefing.Briefing) BriefingPayload {
	p := BriefingPayload{Kind: string(b.Kind), Date: b.Date, Headline: b.Headline, AI: b.AI,
		GeneratedAt: b.CreatedAt.UnixMilli(), Items: make([]BriefingItem, 0, len(b.Items))}
	for _, it := range b.Items {
		p.Items = append(p.Items, BriefingItem{Section: it.Section, Icon: it.Icon, Who: it.Who, Color: it.Color, Text: it.Text})
	}
	p.Text = briefingMarkdown(b)
	return p
}

func briefingMarkdown(b *briefing.Briefing) string {
	var parts []string
	if b.Headline != "" {
		parts = append(parts, "**"+b.Headline+"**")
	}
	var block []string
	section := ""
	flush := func() {
		if len(block) > 0 {
			parts = append(parts, strings.Join(block, "\n"))
			block = nil
		}
	}
	for _, it := range b.Items {
		if b.Kind == briefing.Evening && it.Section != section {
			flush()
			section = it.Section
			if section == "tonight" {
				block = append(block, "Heute Abend:")
			} else {
				block = append(block, "Morgen:")
			}
		}
		line := "- "
		if it.Who != "" {
			line += "**" + it.Who + ":** "
		}
		block = append(block, line+it.Text)
	}
	flush()
	return strings.Join(parts, "\n\n")
}

// Pusher sends the children's weeks and the briefings to the app. It builds
// the payloads every tick and only posts what changed – plus a heartbeat for
// the children, so the app can tell fresh data from a dead dashboard.
type Pusher struct {
	Client    *Client
	Loc       *time.Location
	Heartbeat time.Duration    // re-send unchanged child data this often
	ExtraCals map[string][]int // FAMILY_APP_<SLUG>_CALENDARS → calendar indexes
	Gather    func(now time.Time) Inputs
	Briefings func() []*briefing.Briefing // optional: current cards of every kind

	mu    sync.Mutex
	sent  map[string]sentState
	errAt map[string]time.Time
}

type sentState struct {
	hash string
	at   time.Time
}

const (
	pushWarmup  = 2 * time.Minute // let the sources load once – never push half-empty data
	pushBackoff = 5 * time.Minute // after a failed post
)

func (p *Pusher) Run(ctx context.Context, every time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(pushWarmup):
	}
	p.Push(ctx, time.Now())
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.Push(ctx, now)
		}
	}
}

// Push sends everything that is due once. Incomplete central projections —
// a required calendar source without first valid data — are held back so
// the last good snapshot in the app stays intact; only complete data is
// ever transmitted.
func (p *Pusher) Push(ctx context.Context, now time.Time) {
	for _, c := range BuildChildren(p.Gather(now), now, p.Loc, p.ExtraCals) {
		if c.Incomplete {
			slog.Info("familyapp: holding back incomplete child projection", "child", c.ChildSlug)
			continue
		}
		p.send(ctx, "child:"+c.ChildSlug, "/ingest/child", c, now, p.Heartbeat)
	}
	if p.Briefings == nil {
		return
	}
	for _, b := range p.Briefings() {
		if b == nil || b.Date == "" {
			continue
		}
		p.send(ctx, "briefing:"+string(b.Kind)+":"+b.Date, "/ingest/briefing", NewBriefingPayload(b), now, 0)
	}
}

func (p *Pusher) send(ctx context.Context, key, path string, payload any, now time.Time, heartbeat time.Duration) {
	js, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("familyapp: encode", "key", key, "err", err)
		return
	}
	sum := sha256.Sum256(js)
	hash := hex.EncodeToString(sum[:8])

	p.mu.Lock()
	if p.sent == nil {
		p.sent, p.errAt = map[string]sentState{}, map[string]time.Time{}
	}
	last, failed := p.sent[key], p.errAt[key]
	p.mu.Unlock()
	if last.hash == hash && (heartbeat <= 0 || now.Sub(last.at) < heartbeat) {
		return
	}
	if !failed.IsZero() && now.Sub(failed) < pushBackoff {
		return
	}

	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = p.Client.post(cctx, path, payload)

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if failed.IsZero() {
			slog.Warn("familyapp: push failed", "key", key, "err", err)
		}
		p.errAt[key] = now
		return
	}
	if !failed.IsZero() {
		slog.Info("familyapp: push works again", "key", key)
	}
	delete(p.errAt, key)
	p.sent[key] = sentState{hash: hash, at: now}
	// forget briefings of days long gone
	for k, s := range p.sent {
		if strings.HasPrefix(k, "briefing:") && now.Sub(s.at) > 72*time.Hour {
			delete(p.sent, k)
		}
	}
}
