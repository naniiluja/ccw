package zen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limit is a model's token limits from models.dev; 0 means unknown.
type Limit struct{ Context, Input, Output int64 }

// Catalogue is what models.dev, the catalogue the OpenCode agent itself reads,
// says about the OpenCode models: which reason, which cost nothing, their
// limits, and which speak the Responses API (provider.npm is @ai-sdk/openai, the
// field the agent picks its endpoint from).
type Catalogue struct {
	Reasoning  map[string]bool
	PricedFree map[string]bool
	Limits     map[string]Limit
	Responses  map[string]bool
}

const responsesNPM = "@ai-sdk/openai"

// ParseCatalogue reads a models.dev document. It reports false when the
// document names no OpenCode model that says whether it reasons: an answer of
// that shape is not the catalogue, and must not replace one that worked.
func ParseCatalogue(raw []byte) (Catalogue, bool) {
	var doc struct {
		OpenCode struct {
			Models map[string]struct {
				Reasoning *bool `json:"reasoning"`
				Cost      *struct {
					Input  *float64 `json:"input"`
					Output *float64 `json:"output"`
				} `json:"cost"`
				Limit struct {
					Context float64 `json:"context"`
					Input   float64 `json:"input"`
					Output  float64 `json:"output"`
				} `json:"limit"`
				Provider struct {
					NPM string `json:"npm"`
				} `json:"provider"`
			} `json:"models"`
		} `json:"opencode"`
	}
	c := Catalogue{Reasoning: map[string]bool{}, PricedFree: map[string]bool{}, Limits: map[string]Limit{}, Responses: map[string]bool{}}
	if json.Unmarshal(raw, &doc) != nil {
		return c, false
	}
	for id, m := range doc.OpenCode.Models {
		if m.Reasoning != nil {
			c.Reasoning[id] = *m.Reasoning
		}
		if m.Cost != nil && m.Cost.Input != nil && m.Cost.Output != nil && *m.Cost.Input == 0 && *m.Cost.Output == 0 {
			c.PricedFree[id] = true
		}
		if l := (Limit{int64(m.Limit.Context), int64(m.Limit.Input), int64(m.Limit.Output)}); l != (Limit{}) {
			c.Limits[id] = l
		}
		if m.Provider.NPM == responsesNPM {
			c.Responses[id] = true
		}
	}
	return c, len(c.Reasoning) > 0
}

// IsFree reports whether the anonymous key can call a model: it is named -free,
// or models.dev prices it at zero. It says nothing about whether ccw can
// reach the model; Hidden keeps the ones that speak no chat shape out of the list.
func IsFree(id string, c *Catalogue) bool {
	return strings.HasSuffix(id, "-free") || (c != nil && c.PricedFree[id])
}

// List returns the models an upstream list offers to a caller of the anonymous
// key, and the list in the shape ccw reads token limits and thinking from
// (OpenRouter's: supported_parameters, context_length, top_provider). A model
// the catalogue does not name gets neither field.
func List(upstream []string, c *Catalogue) (ids []string, doc []byte) {
	if c == nil {
		c = &Catalogue{}
	}
	data := []map[string]any{}
	for _, id := range upstream {
		if !IsFree(id, c) || Hidden(id) {
			continue
		}
		ids = append(ids, id)
		item := map[string]any{"id": id, "object": "model", "owned_by": "opencode"}
		if r, ok := c.Reasoning[id]; ok {
			if r {
				item["supported_parameters"] = []string{"reasoning", "include_reasoning"}
			} else {
				item["supported_parameters"] = []string{}
			}
		}
		if l := c.Limits[id]; l.Context > 0 || l.Output > 0 {
			if l.Context > 0 {
				item["context_length"] = l.Context
			}
			top := map[string]any{}
			if l.Context > 0 {
				top["context_length"] = l.Context
			}
			if l.Output > 0 {
				top["max_completion_tokens"] = l.Output
			}
			item["top_provider"] = top
		}
		data = append(data, item)
	}
	doc, _ = json.Marshal(map[string]any{"object": "list", "data": data})
	return ids, doc
}

// Source fetches the catalogue and keeps it for TTL. A failed read keeps the last
// answer, so a models.dev outage never empties the model list.
type Source struct {
	URL    string
	TTL    time.Duration
	Client *http.Client

	mu   sync.Mutex
	cat  Catalogue
	have bool
	at   time.Time
}

// Get returns the catalogue, refreshing it when it is older than TTL. It is nil
// until one read has worked, or when URL is empty.
func (s *Source) Get(ctx context.Context) *Catalogue {
	if s == nil || s.URL == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.have && time.Since(s.at) < s.TTL {
		return &s.cat
	}
	if !s.have && !s.at.IsZero() && time.Since(s.at) < time.Minute {
		return nil // a failed first read is not retried on every call
	}
	s.at = time.Now()
	if c, ok := s.fetch(ctx); ok {
		s.cat, s.have = c, true
	}
	if !s.have {
		return nil
	}
	return &s.cat
}

func (s *Source) fetch(ctx context.Context) (Catalogue, bool) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Catalogue{}, false
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Catalogue{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalogue{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Catalogue{}, false
	}
	return ParseCatalogue(raw)
}
