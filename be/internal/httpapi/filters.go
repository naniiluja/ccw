package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/naniiluja/ccw/internal/filter"
	"github.com/naniiluja/ccw/internal/store"
)

// filterCache holds the compiled rules per provider ("*" for all). It is
// rebuilt after every change made through the dashboard.
type filterCache struct {
	mu    sync.RWMutex
	ready bool
	by    map[string][]filter.Rule
}

// rulesFor returns the enabled rules that apply to a provider.
func (a *api) rulesFor(prov string) []filter.Rule {
	a.filters.mu.RLock()
	if a.filters.ready {
		out := append(append([]filter.Rule{}, a.filters.by["*"]...), a.filters.by[prov]...)
		a.filters.mu.RUnlock()
		return out
	}
	a.filters.mu.RUnlock()
	a.reloadFilters()
	return a.rulesFor(prov)
}

// reloadFilters compiles the stored filters. A rule that no longer compiles is
// skipped and logged rather than blocking every request.
func (a *api) reloadFilters() {
	list, err := a.store.ListFilters()
	by := map[string][]filter.Rule{}
	if err != nil {
		a.logger().Error("filters.load.fail", "err", err)
	}
	for _, f := range list {
		if !f.Enabled {
			continue
		}
		// A body-wiping rule stored before the guard existed must not apply now.
		if why := unsafeFilter(f.Kind, f.Pattern); why != "" {
			a.logger().Warn("filters.skip.unsafe", "filter", f.ID, "kind", f.Kind, "pattern", f.Pattern, "reason", why)
			continue
		}
		r, err := filter.Compile(f.Kind, f.Pattern)
		if err != nil {
			a.logger().Warn("filters.skip.invalid", "filter", f.ID, "err", err)
			continue
		}
		by[f.Provider] = append(by[f.Provider], r)
	}
	a.filters.mu.Lock()
	a.filters.by, a.filters.ready = by, true
	a.filters.mu.Unlock()
}

// listFilters serves the filters as JSON.
func (a *api) listFilters(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListFilters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read filters")
		return
	}
	writeJSON(w, map[string]any{"filters": list})
}

// saveFilter creates (no id) or replaces a filter from a JSON body.
func (a *api) saveFilter(w http.ResponseWriter, r *http.Request) {
	var f store.Filter
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&f); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if _, err := filter.Compile(f.Kind, f.Pattern); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if why := unsafeFilter(f.Kind, f.Pattern); why != "" {
		writeError(w, http.StatusBadRequest, why)
		return
	}
	saved, err := a.putFilter(f)
	if errors.Is(err, store.ErrFilterNotFound) {
		writeError(w, http.StatusNotFound, "unknown filter")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot save filter")
		return
	}
	writeJSON(w, saved)
}

// deleteFilter removes one filter.
func (a *api) deleteFilter(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteFilter(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "unknown filter")
		return
	}
	a.reloadFilters()
	writeJSON(w, map[string]any{"ok": true})
}

// filterInPlace reports an enabled rule already stored for that provider. A
// review asks before it installs: a second row doubles the alert and grows the
// table on every pass.
func (a *api) filterInPlace(provider, kind, pattern string) bool {
	list, err := a.store.ListFilters()
	if err != nil {
		return false
	}
	for _, f := range list {
		if f.Enabled && f.Provider == provider && f.Kind == kind && f.Pattern == pattern {
			return true
		}
	}
	return false
}

// apiProviders lists the providers with their account counts for machines.
func (a *api) apiProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"providers": a.providerSummary()})
}

// One guard decides what a filter rule may never remove. The dashboard, MCP,
// the drift review and the error review all ask it, so a rule that empties
// every request cannot enter through a second door. The reviews act on a
// model's word, and that model reads a client's own request text.

// essentialLeaf are the last names of a path that carry the request itself, at
// any depth: messages, messages.*.content, contents.*.parts.*.text. A deeper
// path below one of them (messages.*.content.*.cache_control) is fine.
var essentialLeaf = map[string]bool{
	"content": true, "contents": true, "parts": true, "text": true, "role": true,
	"messages": true, "input": true, "prompt": true, "instructions": true, "system": true,
	"systemInstruction": true, "generationConfig": true, "model": true, "tools": true,
	"request": true, "stream": true,
}

// essentialPaths name what a request needs under a container, where the leaf
// alone says nothing: a tool without its name or its schema cannot be called.
var essentialPaths = [][]string{
	{"tools", "*", "name"},
	{"tools", "*", "function"},
	{"tools", "*", "function", "name"},
	{"tools", "*", "function", "parameters"},
	{"tools", "*", "input_schema"},
	{"tools", "*", "parameters"},
}

// systemLines are ordinary system-prompt lines. An expression that matches one
// of them removes the prompt instead of one marker line, so it is refused.
var systemLines = []string{
	"",
	"x",
	"You are a helpful assistant.",
	strings.Repeat("Answer in short sentences and keep the user's format. ", 4)[:200],
}

// essentialField reports a field rule that leaves a request the provider
// cannot answer: every item of a container, a content-bearing name, or a
// tool's identity.
func essentialField(pattern string) bool {
	segs := strings.Split(strings.TrimSpace(pattern), ".")
	if last := segs[len(segs)-1]; last == "*" || essentialLeaf[last] {
		return true
	}
	if len(segs) > 1 && segs[0] == "request" {
		segs = segs[1:] // Antigravity wraps the Gemini request under "request"
	}
	for _, e := range essentialPaths {
		if coversPath(segs, e) {
			return true
		}
	}
	return false
}

// coversPath reports whether p is e or an ancestor of it. "*" on either side
// matches one segment.
func coversPath(p, e []string) bool {
	if len(p) > len(e) {
		return false
	}
	for i, s := range p {
		if s != e[i] && s != "*" && e[i] != "*" {
			return false
		}
	}
	return true
}

// catchAllSystem reports a system expression that removes the prompt: it does
// not compile, or it matches an ordinary line.
func catchAllSystem(pattern string) bool {
	re, err := regexp.Compile("(?m)" + strings.TrimSpace(pattern))
	if err != nil {
		return true
	}
	for _, line := range systemLines {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// unsafeFilter says why a rule may not be installed, or "" when it is safe.
func unsafeFilter(kind, pattern string) string {
	switch kind {
	case filter.Field:
		if essentialField(pattern) {
			return "this pattern would strip an essential field from every request; narrow it"
		}
	case filter.System:
		if catchAllSystem(pattern) {
			return "this expression matches ordinary system lines; it would remove the whole system prompt"
		}
	}
	return ""
}
