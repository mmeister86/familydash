package briefing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Gemini calls generateContent with a plain API key (GEMINI_API_KEY).
//
// The same key format exists for two services: the Gemini Developer API
// (AI Studio, generativelanguage.googleapis.com) and Vertex AI in express mode
// (aiplatform.googleapis.com). Backend "auto" tries the Developer API first and
// switches to Vertex when the key is rejected there – and remembers what worked.
type Gemini struct {
	Key      string
	Model    string // gemini-3.8-flash
	Thinking string // low | medium | high, "" = model default
	Backend  string // auto | gemini | vertex

	GeminiURL string // https://generativelanguage.googleapis.com
	VertexURL string // https://aiplatform.googleapis.com

	client *http.Client
	mu     sync.Mutex
	works  string // backend that answered last time (auto mode)
}

func NewGemini(key, model, thinking, backend string) *Gemini {
	if backend == "" {
		backend = "auto"
	}
	return &Gemini{
		Key: key, Model: model, Thinking: thinking, Backend: strings.ToLower(backend),
		GeminiURL: "https://generativelanguage.googleapis.com",
		VertexURL: "https://aiplatform.googleapis.com",
		client:    &http.Client{Timeout: 90 * time.Second},
	}
}

// apiError is a non-200 answer; authRejected marks "wrong service for this key".
type apiError struct {
	backend string
	status  int
	msg     string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.backend, e.status, e.msg)
}

func (e *apiError) authRejected() bool {
	if e.status == http.StatusUnauthorized || e.status == http.StatusForbidden {
		return true
	}
	m := strings.ToLower(e.msg)
	return e.status == http.StatusBadRequest && (strings.Contains(m, "api key") || strings.Contains(m, "api_key"))
}

// Usage is the token count of one call (for the log).
type Usage struct {
	Prompt   int `json:"promptTokenCount"`
	Output   int `json:"candidatesTokenCount"`
	Thoughts int `json:"thoughtsTokenCount"`
}

// GenerateJSON sends system + user text and returns the model's JSON answer
// (constrained by schema, an OpenAPI-style schema both backends understand).
func (g *Gemini) GenerateJSON(ctx context.Context, system, user string, schema any) ([]byte, Usage, error) {
	gen := map[string]any{
		"responseMimeType": "application/json",
		"responseSchema":   schema,
		"maxOutputTokens":  4096,
	}
	if g.Thinking != "" {
		gen["thinkingConfig"] = map[string]any{"thinkingLevel": strings.ToUpper(g.Thinking)}
	}
	body, err := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": system}}},
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": user}}}},
		"generationConfig":  gen,
	})
	if err != nil {
		return nil, Usage{}, err
	}

	order := []string{g.Backend}
	if g.Backend == "auto" {
		g.mu.Lock()
		order = []string{"gemini", "vertex"}
		if g.works == "vertex" {
			order = []string{"vertex", "gemini"}
		}
		g.mu.Unlock()
	}
	var lastErr error
	for _, b := range order {
		out, usage, err := g.post(ctx, b, body)
		if err == nil {
			g.mu.Lock()
			g.works = b
			g.mu.Unlock()
			return out, usage, nil
		}
		lastErr = err
		var ae *apiError
		if g.Backend != "auto" || !errors.As(err, &ae) || !ae.authRejected() {
			break // only a rejected key is a reason to try the other service
		}
	}
	return nil, Usage{}, lastErr
}

func (g *Gemini) post(ctx context.Context, backend string, body []byte) ([]byte, Usage, error) {
	var url string
	switch backend {
	case "vertex":
		url = strings.TrimRight(g.VertexURL, "/") + "/v1/publishers/google/models/" + g.Model + ":generateContent"
	case "gemini":
		url = strings.TrimRight(g.GeminiURL, "/") + "/v1beta/models/" + g.Model + ":generateContent"
	default:
		return nil, Usage{}, fmt.Errorf("GEMINI_BACKEND %q: want auto, gemini or vertex", backend)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.Key) // header, never ?key= – errors would print the URL
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("%s: %w", backend, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("%s: %w", backend, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, Usage{}, &apiError{backend: backend, status: resp.StatusCode, msg: msg}
	}

	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		Usage Usage `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, Usage{}, fmt.Errorf("%s: decode: %w", backend, err)
	}
	if r.PromptFeedback.BlockReason != "" {
		return nil, r.Usage, fmt.Errorf("%s: blocked: %s", backend, r.PromptFeedback.BlockReason)
	}
	if len(r.Candidates) == 0 {
		return nil, r.Usage, fmt.Errorf("%s: no answer", backend)
	}
	var sb strings.Builder
	for _, p := range r.Candidates[0].Content.Parts {
		if !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return nil, r.Usage, fmt.Errorf("%s: empty answer (finish: %s)", backend, r.Candidates[0].FinishReason)
	}
	return []byte(text), r.Usage, nil
}
