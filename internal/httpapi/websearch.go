package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
	"github.com/naniiluja/ccw/internal/websearch"
)

// webState holds the search service ccw calls for a client that asks its model
// to search. It is built from the environment on first use, so an ccw with no
// search configured costs nothing.
type webState struct {
	once sync.Once
	s    websearch.Searcher
	// unavailable is why there is no searcher.
	unavailable string
}

// searcher returns the search service to use, or why there is none. A service set
// in the environment wins. With none set, an active Antigravity account searches
// through Google, so a setup that already has one needs nothing more.
func (a *api) searcher() (websearch.Searcher, string) {
	a.web.once.Do(func() {
		if a.web.s != nil { // set by a test
			return
		}
		provider := strings.ToLower(strings.TrimSpace(os.Getenv("CCW_SEARCH_PROVIDER")))
		switch provider {
		case "":
			a.web.unavailable = "web search is not set up on this ccw: add an Antigravity account, or set CCW_SEARCH_PROVIDER and CCW_SEARCH_KEY"
		case "antigravity":
			a.web.s = antigravitySearch{a: a}
		default:
			s, err := websearch.New(*websearch.FromEnv())
			if err != nil {
				a.web.unavailable = err.Error()
				return
			}
			a.web.s = s
		}
	})
	if a.web.s == nil && a.web.unavailable != "" && os.Getenv("CCW_SEARCH_PROVIDER") == "" && len(a.activeConnections("antigravity")) > 0 {
		return antigravitySearch{a: a}, ""
	}
	return a.web.s, a.web.unavailable
}

// hostedSearch answers a request that declares Anthropic's hosted web search
// tool, for a provider that has none: it runs the search, and returns the request
// rewritten for the model with the results in it, and the search for the answer to
// carry. A request it cannot rewrite is returned as it came, with no search.
func (a *api) hostedSearch(ctx context.Context, body []byte) ([]byte, *translate.WebSearch) {
	query := websearch.Query(body)
	var results []websearch.Result
	failure := ""
	switch s, why := a.searcher(); {
	case query == "":
		failure = "the request names nothing to search for"
	case s == nil:
		failure = why
	default:
		var err error
		if results, err = s.Search(ctx, query, 0); err != nil {
			failure = err.Error()
		}
	}
	if failure != "" {
		log.Printf("web search %q: %s", query, failure)
	}
	rewritten, err := websearch.Rewrite(body, query, results, failure)
	if err != nil {
		return body, nil
	}
	found := make([]translate.WebResult, len(results))
	for i, r := range results {
		found[i] = translate.WebResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet, Age: r.Age}
	}
	return rewritten, &translate.WebSearch{Query: query, Results: found, Failure: failure}
}

// antigravitySearch searches through Google: it asks a Gemini model behind an
// Antigravity account a question with Google Search grounding, and reads the
// sources the answer was grounded in. It costs a call of a fast model, and needs
// no key beyond the account ccw already holds.
type antigravitySearch struct{ a *api }

// searchModel answers the search. Not every model takes the grounding tool: the
// Gemini 3 ids answered 404 on this endpoint, and this one answered.
func searchModel() string {
	if m := strings.TrimSpace(os.Getenv("CCW_SEARCH_MODEL")); m != "" {
		return m
	}
	return "gemini-2.5-flash"
}

func (s antigravitySearch) Search(ctx context.Context, query string, count int) ([]websearch.Result, error) {
	conns := s.a.activeConnections("antigravity")
	if len(conns) == 0 {
		return nil, fmt.Errorf("no active Antigravity account")
	}
	inner, _ := json.Marshal(map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Search the web for: " + query +
			"\nReport what the sources say as short factual statements, with names, dates and numbers."}}}},
		"tools": []any{map[string]any{"googleSearch": map[string]any{}}},
	})
	var lastErr error
	for _, c := range conns {
		res, err := s.searchWith(ctx, c, inner, count)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s antigravitySearch) searchWith(ctx context.Context, c store.Connection, inner []byte, count int) ([]websearch.Result, error) {
	p, ok := s.a.providerFor(c)
	if !ok {
		return nil, fmt.Errorf("antigravity account %s is not reachable", c.ID)
	}
	secret, err := s.a.secretFor(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	env, err := s.a.antigravityEnvelope(ctx, c, secret, searchModel(), inner)
	if err != nil {
		return nil, err
	}
	_, path := shapeFor(p, searchModel())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.a.send(req, p, c.Provider, path, secret, env)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("antigravity answered %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return readGrounding(resp.Body, count, followRedirects(ctx))
}

// readGrounding reads the stream of a grounded answer into results: each source
// the answer cites, with the statements it was cited for.
func readGrounding(body io.Reader, count int, resolve func(string) string) ([]websearch.Result, error) {
	type chunk struct {
		Web struct{ URI, Title string } `json:"web"`
	}
	var chunks []chunk
	var supports []struct {
		Segment struct{ Text string } `json:"segment"`
		Indices []int                 `json:"groundingChunkIndices"`
	}
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "data:")
		if line == sc.Text() {
			continue
		}
		var ev struct {
			Response struct {
				Candidates []struct {
					Grounding struct {
						Chunks   []chunk `json:"groundingChunks"`
						Supports []struct {
							Segment struct{ Text string } `json:"segment"`
							Indices []int                 `json:"groundingChunkIndices"`
						} `json:"groundingSupports"`
					} `json:"groundingMetadata"`
				} `json:"candidates"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		for _, cand := range ev.Response.Candidates {
			if len(cand.Grounding.Chunks) > 0 {
				chunks, supports = cand.Grounding.Chunks, cand.Grounding.Supports
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(chunks) > count && count > 0 {
		chunks = chunks[:count]
	}
	res := make([]websearch.Result, len(chunks))
	var wg sync.WaitGroup
	for i, c := range chunks {
		var said []string
		for _, sp := range supports {
			for _, idx := range sp.Indices {
				if idx == i && sp.Segment.Text != "" {
					said = append(said, sp.Segment.Text)
				}
			}
		}
		res[i] = websearch.Result{Title: c.Web.Title, URL: c.Web.URI, Snippet: strings.Join(said, " ")}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res[i].URL = resolve(res[i].URL)
		}(i)
	}
	wg.Wait()
	return res, nil
}

// followRedirects turns Google's grounding link into the page it stands for, so
// the client lists the real address. A link that does not resolve stays as it is.
func followRedirects(ctx context.Context) func(string) string {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return func(u string) string {
		if !strings.Contains(u, "grounding-api-redirect") {
			return u
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			return u
		}
		resp, err := client.Do(req)
		if err != nil {
			return u
		}
		resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "" && resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return loc
		}
		return u
	}
}
