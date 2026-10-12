package diabridge

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/naniiluja/ccw/internal/websearch"
)

// prepare turns a request into one Dia's gateway accepts: it runs the web
// search the client declared, then applies the tool-name and thinking rewrites,
// and returns the request with the mapping the reply has to be read back with.
//
// The search runs first because it recognises the client's own spelling of the
// tool ("WebSearch", "web_search"), while the compat pass renames tools and
// would no longer leave anything to recognise.
//
// Dia's gateway refuses Anthropic's hosted search tool with "Request contains
// an unrecognized tool", and the refusal ends the handler turn, so the search
// cannot be sent there at all. ccw already runs that search for every other
// provider whose model cannot search on its own, so it runs the same one here
// and hands the model the results, through the same service rather than a
// second implementation.
func (b *Bridge) prepare(ctx context.Context, body []byte) ([]byte, map[string]string) {
	body = b.runSearch(ctx, body)

	var parsed map[string]any
	if json.Unmarshal(body, &parsed) != nil {
		return body, map[string]string{}
	}
	renamed := applyCompat(parsed)
	out, err := json.Marshal(parsed)
	if err != nil {
		return body, map[string]string{}
	}
	return out, renamed
}

// runSearch runs the search a request asks for and folds the results into the
// request's system prompt. A request that declares none comes back as it went
// in. A request that declares one on a ccw with no search service still gets a
// rewritten request, carrying the same wording /v1 gives, so the model is told
// the search failed rather than left to invent sources.
func (b *Bridge) runSearch(ctx context.Context, body []byte) []byte {
	if !websearch.Hosted(body) {
		return body
	}
	query := websearch.Query(body)
	failure := ""
	var results []websearch.Result
	switch {
	case query == "":
		failure = "the request names nothing to search for"
	case b.search == nil:
		failure = "web search is not set up on this ccw: add an Antigravity account, or set CCW_SEARCH_PROVIDER and CCW_SEARCH_KEY"
	default:
		var err error
		if results, err = b.search.Search(ctx, query, 0); err != nil {
			// The query is the client's text, so only its length is logged.
			slog.Info("dia.search.fail", "query_len", len(query), "reason", err.Error())
			failure = err.Error()
		}
	}
	rewritten, err := websearch.Rewrite(body, query, results, failure)
	if err != nil {
		return body
	}
	return rewritten
}

// SetSearch gives the bridge the search service to run for clients whose model
// asks to search. ccw passes the one it already built, so Dia searches through
// the same provider every other route uses.
func (b *Bridge) SetSearch(s websearch.Searcher) { b.search = s }
