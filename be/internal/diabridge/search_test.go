package diabridge

import (
	"context"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/websearch"
)

// fakeSearch stands in for the search service ccw shares with every provider,
// so the test proves the bridge runs a search rather than reaching one itself.
type fakeSearch struct {
	calls []string
}

func (f *fakeSearch) Search(_ context.Context, query string, _ int) ([]websearch.Result, error) {
	f.calls = append(f.calls, query)
	return []websearch.Result{{Title: "Bitcoin price", URL: "https://example.test/btc", Snippet: "about 82,600 USD"}}, nil
}

func TestBridgeRunsTheSearchItsOwnClientDeclared(t *testing.T) {
	b := New()
	search := &fakeSearch{}
	b.search = search
	body := []byte(`{"model":"claude-opus-5-5","max_tokens":100,"stream":true,` +
		`"tools":[{"name":"WebSearch","description":"Search the web"}],` +
		`"messages":[{"role":"user","content":"Perform a web search for the query: bitcoin price"}]}`)

	sent, _ := b.prepare(context.Background(), body)

	if len(search.calls) != 1 || !strings.Contains(search.calls[0], "bitcoin price") {
		t.Fatalf("bridge did not run the search its client declared, calls: %v", search.calls)
	}
	if bytesContain(sent, []byte("WebSearch")) {
		t.Error("the search tool was sent to the gateway, which refuses it")
	}
	if !bytesContain(sent, []byte("example.test/btc")) {
		t.Error("the results were not handed to the model")
	}
}

func TestBridgeSendsARequestWithoutSearchUntouched(t *testing.T) {
	b := New()
	search := &fakeSearch{}
	b.search = search
	body := []byte(`{"model":"claude-opus-5-5","max_tokens":100,` +
		`"tools":[{"name":"Read","description":"read a file"}],` +
		`"messages":[{"role":"user","content":"read the file"}]}`)

	sent, renamed := b.prepare(context.Background(), body)

	if len(search.calls) != 0 {
		t.Errorf("a request that declared no search still ran one: %v", search.calls)
	}
	if !bytesContain(sent, []byte(`"name":"Read"`)) {
		t.Error("the tools a request did declare were dropped")
	}
	if _, ok := renamed["Read"]; !ok {
		t.Error("a tool the gateway accepts lost its mapping")
	}
}

func TestBridgeKeepsTheRequestWhenTheSearchFails(t *testing.T) {
	b := New()
	b.search = failingSearch{}
	body := []byte(`{"model":"claude-opus-5-5","max_tokens":100,` +
		`"tools":[{"name":"WebSearch","description":"Search the web"}],` +
		`"messages":[{"role":"user","content":"search for the weather"}]}`)

	sent, _ := b.prepare(context.Background(), body)

	if !bytesContain(sent, []byte("search failed")) {
		t.Error("the model was not told the search failed, so it would invent sources")
	}
}

// failingSearch stands for a ccw with no search service set up at all.
type failingSearch struct{}

func (failingSearch) Search(_ context.Context, _ string, _ int) ([]websearch.Result, error) {
	return nil, errNoSearcher
}

var errNoSearcher = &searchError{"no search service is configured"}

// searchError is the plain error the bridge hands back; websearch.Searcher is a
// one-method interface, so the bridge has no provider of its own to name here.
type searchError struct{ msg string }

func (e *searchError) Error() string { return e.msg }

func bytesContain(hay, needle []byte) bool {
	return strings.Contains(string(hay), string(needle))
}
