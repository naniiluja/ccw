package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"

	"github.com/naniiluja/ccw/internal/servertools"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
)

// droppedTools remembers which tools ccw has said it drops, so the log names
// each once and not on every request.
var droppedTools sync.Map

func logDropped(names []string) {
	for _, n := range names {
		if _, seen := droppedTools.LoadOrStore(n, true); !seen {
			log.Printf("tool %s is dropped from the request: ccw does not emulate it, and the provider has none", n)
		}
	}
}

// serverTools answers a Messages request that declares tools Anthropic runs on
// its own servers (the MCP connector, tool search, web fetch), for a provider
// that is not Anthropic. ccw runs them: it calls the model without a stream,
// runs the calls the model makes to those tools, and calls the model again with
// the results, until the model answers or asks the client for one of its own
// tools. The client gets the blocks Anthropic's API would have sent.
func (a *api) serverTools(w http.ResponseWriter, r *http.Request, body []byte, targets []store.Connection, start int, search *translate.WebSearch) {
	plan, err := servertools.Prepare(r.Context(), body)
	if err != nil {
		var re *servertools.RequestError
		if errors.As(err, &re) {
			writeError(w, http.StatusBadRequest, re.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if search != nil {
		// A web search that ran before the first call: its blocks open the
		// answer, and the results stay in the request the model is sent.
		call, result := search.Blocks()
		plan.Preface(call, result, search.Requests())
	}
	req, err := plan.Request()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for {
		rec := &recorder{header: http.Header{}, code: http.StatusOK}
		a.failover(rec, r, req, targets, start)
		if rec.code != http.StatusOK {
			rec.copyTo(w)
			return
		}
		next, final, err := plan.Advance(r.Context(), rec.body.Bytes())
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		if next == nil {
			if m := rec.header.Get("X-Ccw-Model"); m != "" {
				w.Header().Set("X-Ccw-Model", m)
			}
			writeFinal(w, final, plan.Stream)
			return
		}
		req = next
	}
}

func writeFinal(w http.ResponseWriter, msg map[string]any, stream bool) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write(servertools.MessageSSE(msg))
		return
	}
	b, _ := json.Marshal(msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

// recorder is a ResponseWriter that keeps the answer, for the calls ccw makes
// to a model on its own account.
type recorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
	wrote  bool
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) Flush()              {}
func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.code, r.wrote = code, true
	}
}
func (r *recorder) Write(p []byte) (int, error) {
	r.wrote = true
	return r.body.Write(p)
}

// copyTo sends a recorded answer on as it came.
func (r *recorder) copyTo(w http.ResponseWriter) {
	for k, v := range r.header {
		w.Header()[k] = v
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(r.code)
	w.Write(r.body.Bytes())
}
