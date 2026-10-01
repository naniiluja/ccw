package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/drift"
	"github.com/naniiluja/ccw/internal/filter"
	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/servertools"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
	"github.com/naniiluja/ccw/internal/upstream"
	"github.com/naniiluja/ccw/internal/websearch"
)

// ccw has one base URL, /v1. The caller names a model and ccw finds the
// accounts that serve it:
//
//   - "groq/llama-3.3-70b-versatile" names the provider. ccw strips the
//     prefix from the body's model and uses that provider's active accounts.
//   - "llama-3.3-70b-versatile" names only the model. ccw looks it up in the
//     model list of every provider and pools the accounts of all that list it.
//
// The pool is rotated per model so load spreads, and a busy account fails over
// to the next one. The rest of the path goes to the provider as sent, so
// /v1/chat/completions reaches <base>/chat/completions and /v1/messages reaches
// Anthropic's /messages.

// Model lists are cached so resolving a bare model does not cost an upstream
// call per request. A failed fetch is retried sooner than a good one expires.
const (
	catalogTTL     = 10 * time.Minute
	catalogFailTTL = time.Minute
)

// catalogFetchTimeout bounds one list fetch. The fetch outlives the caller that
// started it, so it carries its own bound.
var catalogFetchTimeout = upstream.NewLimit(2 * time.Minute)

type catalogEntry struct {
	ids []string
	ok  bool
	at  time.Time
	// groups holds the models folded from level variants (Antigravity).
	groups map[string]variantSet
	// raw is the provider's last list answer, as it came.
	raw []byte
	// info is what that answer says of each model's thinking.
	info map[string]ModelInfo
}

type catalog struct {
	mu sync.Mutex
	m  map[string]catalogEntry
	// inflight holds the fetch running for a provider, so concurrent misses
	// share one upstream list call.
	inflight map[string]*catalogFetch
}

// catalogFetch is one running list fetch. A waiter reads ids after done closes.
type catalogFetch struct {
	done chan struct{}
	ids  []string
}

// callerModelKey carries the model id the caller sent, before the provider
// prefix is removed, so a translated answer can name it back.
type callerModelKey struct{}

// v1 forwards a request to the accounts that serve the model named in its body.
func (a *api) v1(w http.ResponseWriter, r *http.Request) {
	req, refusal := parseV1Model(w, r)
	if refusal != nil {
		refusal.write(w)
		return
	}
	targets, refusal := a.selectTargets(r, &req)
	if refusal != nil {
		refusal.write(w)
		return
	}
	// The client's own request, before ccw changes anything, is what shows
	// a tool adding or dropping a field.
	if watched(targets[0].Provider) {
		a.drift.ObserveFrom(drift.Request, targets[0].Provider, clientOf(r), r.PathValue("path"), req.original, false)
	}
	if r.PathValue("path") == "messages/count_tokens" && !a.anyAnthropic(targets) {
		countTokensEstimate(w, req.body)
		return
	}
	targets, start := a.startFor(req.model, targets)
	a.failover(w, r.WithContext(context.WithValue(r.Context(), callerModelKey{}, req.model)), req.body, targets, start)
}

// v1Request is a /v1 call once its model is read: the body to send, the body
// as the client sent it, and the model it names, prefix included.
type v1Request struct {
	body, original []byte
	model          string
}

// v1Refusal is a /v1 call ccw refuses before any account is tried. api picks
// the caller's envelope with code and param over the plain error, and allow is
// the Allow header a wrong method gets.
type v1Refusal struct {
	status           int
	code, param, msg string
	api              bool
	allow            string
}

// write answers the caller with the refusal.
func (e *v1Refusal) write(w http.ResponseWriter) {
	if e.allow != "" {
		w.Header().Set("Allow", e.allow)
	}
	if e.api {
		writeAPIError(w, e.status, e.code, e.param, e.msg)
		return
	}
	writeError(w, e.status, e.msg)
}

// parseV1Model reads a /v1 body and the model it names, refusing a call that
// names none, names it twice, or is a Messages call without a valid max_tokens.
func parseV1Model(w http.ResponseWriter, r *http.Request) (v1Request, *v1Refusal) {
	// The OpenAI Responses API is not served to callers: only chat completions
	// and messages are. Responses stays an upstream shape for some providers.
	if strings.Trim(r.PathValue("path"), "/") == "responses" {
		return v1Request{}, &v1Refusal{status: http.StatusNotFound, msg: "Unknown request URL: " + r.Method + " " + r.URL.Path}
	}
	body, status, err := readBody(w, r)
	if err != nil {
		return v1Request{}, &v1Refusal{status: status, msg: err.Error()}
	}
	// Only the first "model" is read here, while the provider may read the
	// last one, so the id ccw checks would not be the id that runs.
	if topLevelCount(body, "model") > 1 {
		return v1Request{}, &v1Refusal{status: http.StatusBadRequest, msg: "the request names \"model\" more than once"}
	}
	req := v1Request{body: body, original: body}
	model, ok := bodyModel(body)
	if !ok || model == "" {
		switch {
		case clientShape(r.PathValue("path")) != "" && r.Method != http.MethodPost:
			return req, &v1Refusal{status: http.StatusMethodNotAllowed, allow: http.MethodPost, msg: "Method " + r.Method + " is not allowed on " + r.URL.Path + "; use POST"}
		case len(bytes.TrimSpace(body)) == 0 && r.Method != http.MethodPost:
			return req, &v1Refusal{status: http.StatusNotFound, msg: "Unknown request URL: " + r.Method + " " + r.URL.Path}
		default:
			return req, &v1Refusal{status: http.StatusBadRequest, api: true, param: "model", msg: "the request names no model; send \"model\": \"<provider>/<model>\" or a model id"}
		}
	}
	// Claude Code marks a model it has a 1M window for with a [1m] suffix, which
	// is a note to the tool and no part of the model's name.
	if base, found := strings.CutSuffix(model, "[1m]"); found && base != "" {
		if renamed, ok := setModel(body, base); ok {
			body, model = renamed, base
		}
	}
	req.body, req.model = body, model
	if r.PathValue("path") == "messages" {
		if msg := checkMaxTokens(body); msg != "" {
			return req, &v1Refusal{status: http.StatusBadRequest, api: true, param: "max_tokens", msg: msg}
		}
	}
	return req, nil
}

// selectTargets finds the accounts that may serve a request's model for this
// caller. A model that names its provider has the prefix stripped from the
// body, which selectTargets rewrites in req.
func (a *api) selectTargets(r *http.Request, req *v1Request) ([]store.Connection, *v1Refusal) {
	model := req.model
	prov, upstreamModel := a.splitModel(model)
	caller := principalOf(r)
	if !caller.allowsModel(prov, upstreamModel, model) {
		return nil, &v1Refusal{status: http.StatusForbidden, msg: "model is not permitted for this api key"}
	}
	var targets []store.Connection
	if prov != "" {
		// Check the off switch against the model and its variant base. Using
		// splitVariant (not the catalog) means a variant of a switched-off base
		// is still refused on a cold cache, with no model-list fetch per request.
		off := a.store.InactiveModels(prov)
		vbase, _ := splitVariant(upstreamModel)
		if (off[upstreamModel] || off[vbase]) && r.Context().Value(modelTestKey{}) == nil {
			return nil, &v1Refusal{status: http.StatusForbidden, msg: "this model is switched off in ccw"}
		}
		targets = a.activeConnections(prov)
		if upstreamModel != model {
			var ok bool
			if req.body, ok = setModel(req.body, upstreamModel); !ok {
				return nil, &v1Refusal{status: http.StatusBadRequest, msg: "cannot rewrite the model"}
			}
		}
	} else {
		for _, p := range a.providersServing(r.Context(), model) {
			if caller.allowsProviderModel(p, model) {
				targets = append(targets, a.activeConnections(p)...)
			}
		}
	}
	// A test pinned to one account reaches that account alone, switched on or not.
	if run, _ := r.Context().Value(modelTestKey{}).(*testRun); run != nil && run.pin != "" {
		targets = nil
		if list, err := a.store.ListConnections(); err == nil {
			for _, c := range list {
				if c.ID == run.pin && (prov == "" || c.Provider == prov) {
					targets = []store.Connection{c}
				}
			}
		}
	}
	if len(targets) == 0 {
		return nil, &v1Refusal{status: http.StatusNotFound, api: true, code: "model_not_found", param: "model", msg: "no active account serves this model"}
	}
	return targets, nil
}

// splitModel reads a "<provider>/<model>" prefix. It reports the provider only
// when the prefix is a registered provider, because model ids carry slashes of
// their own (meta-llama/llama-3.3-70b-instruct at openrouter).
func (a *api) splitModel(model string) (prov, rest string) {
	i := strings.IndexByte(model, '/')
	if i <= 0 {
		return "", model
	}
	if !a.knownProvider(model[:i]) {
		return "", model
	}
	return model[:i], model[i+1:]
}

// knownProvider reports whether id is a registered provider or the id of a
// custom provider that a stored connection defines.
func (a *api) knownProvider(id string) bool {
	if _, ok := provider.Lookup(id); ok {
		return true
	}
	list, _ := a.store.ListConnections()
	for _, c := range list {
		if c.Provider == id && c.BaseURL != "" {
			return true
		}
	}
	return false
}

// providerFor returns how to reach one connection: its registered provider,
// with the connection's base URL when it sets one, or a generic
// OpenAI-compatible upstream for a custom provider. It reports false when the
// connection cannot be reached (unknown provider, or a URL still missing its
// account id).
func (a *api) providerFor(c store.Connection) (provider.Provider, bool) {
	p, ok := provider.Lookup(c.Provider)
	switch {
	case ok && c.BaseURL != "":
		p.BaseURL = c.BaseURL
	case !ok && c.BaseURL != "":
		p, ok = provider.GenericAPI(c.Provider, c.BaseURL, c.Meta["api"]), true
	}
	if !ok || p.BaseURL == "" || strings.Contains(p.BaseURL, "{accountId}") {
		return provider.Provider{}, false
	}
	return p, true
}

// providersServing returns, in id order, the providers with an active account
// whose model list contains model. Lists are fetched in parallel when stale.
func (a *api) providersServing(ctx context.Context, model string) []string {
	provs := a.activeProviders()
	lists := make([][]string, len(provs))
	var wg sync.WaitGroup
	for i, p := range provs {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			lists[i] = a.providerModels(ctx, p)
		}(i, p)
	}
	wg.Wait()
	out := []string{}
	for i, p := range provs {
		if slices.Contains(lists[i], a.variantBase(p, model)) {
			out = append(out, p)
		}
	}
	return out
}

// models answers GET /v1/models with every provider's models, each named
// "<provider>/<model>" so the id a client picks routes back to one provider.
func (a *api) models(w http.ResponseWriter, r *http.Request) {
	data := a.modelEntries(r.Context())
	first, last := "", ""
	if len(data) > 0 {
		first, last = data[0].ID, data[len(data)-1].ID
	}
	writeJSON(w, map[string]any{"object": "list", "data": data, "has_more": false, "first_id": first, "last_id": last})
}

// model serves one entry of the list, by "<provider>/<model>" or by a bare id
// that only one provider lists.
func (a *api) model(w http.ResponseWriter, r *http.Request) {
	want := r.PathValue("model")
	var bare []modelEntry
	for _, e := range a.modelEntries(r.Context()) {
		if e.ID == want {
			writeJSON(w, e)
			return
		}
		if strings.HasSuffix(e.ID, "/"+want) {
			bare = append(bare, e)
		}
	}
	if len(bare) == 1 {
		writeJSON(w, bare[0])
		return
	}
	writeAPIError(w, http.StatusNotFound, "model_not_found", "model", "model "+strconv.Quote(want)+" is not served")
}

// modelEntry serves both vocabularies: OpenAI reads object, created and
// owned_by, Anthropic reads type, display_name and created_at.
type modelEntry struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	// Token limits, when the provider's list gives them.
	ContextLength   int64 `json:"context_length,omitempty"`
	MaxInputTokens  int64 `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

// modelEntries lists every model an active account serves. A provider list
// gives no creation date, so created is when ccw read the list.
func (a *api) modelEntries(ctx context.Context) []modelEntry {
	provs := a.activeProviders()
	lists := make([][]string, len(provs))
	var wg sync.WaitGroup
	for i, p := range provs {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			lists[i] = a.providerModels(ctx, p)
		}(i, p)
	}
	wg.Wait()
	data := []modelEntry{}
	for i, p := range provs {
		a.cat.mu.Lock()
		infos, at := a.cat.m[p].info, a.cat.m[p].at
		a.cat.mu.Unlock()
		if at.IsZero() {
			at = time.Now()
		}
		for _, id := range lists[i] {
			in := infos[id]
			data = append(data, modelEntry{ID: p + "/" + id, Object: "model", Created: at.Unix(), OwnedBy: p,
				Type: "model", DisplayName: p + "/" + id, CreatedAt: at.UTC().Format(time.RFC3339),
				ContextLength: in.Context, MaxInputTokens: in.Input, MaxOutputTokens: in.Output})
		}
	}
	return data
}

// activeProviders returns the registered providers that have an active account.
func (a *api) activeProviders() []string {
	list, err := a.store.ListConnections()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, c := range list {
		if _, ok := a.providerFor(c); ok && c.IsActive {
			seen[c.Provider] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// providerModels returns the models a provider serves: what it lists, from
// the cache when fresh, minus the ones the operator switched off.
func (a *api) providerModels(ctx context.Context, prov string) []string {
	ids := a.catalogIDs(ctx, prov)
	off := a.store.InactiveModels(prov)
	if len(off) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !off[id] {
			out = append(out, id)
		}
	}
	return out
}

// catalogIDs returns everything a provider lists, from the cache while it is
// fresh and from one shared fetch when it is not.
func (a *api) catalogIDs(ctx context.Context, prov string) []string {
	a.cat.mu.Lock()
	e, hit := a.cat.m[prov]
	a.cat.mu.Unlock()
	ttl := catalogTTL
	if !e.ok {
		ttl = catalogFailTTL
	}
	if hit && time.Since(e.at) < ttl {
		return e.ids
	}
	return a.fetchCatalog(ctx, prov, e.ids)
}

// fetchCatalog runs one list fetch per provider: a caller that arrives while a
// fetch runs waits for that one instead of starting a second. The fetch is
// detached from the caller, so it still fills the cache when the caller goes
// away, and a caller that gives up gets the list it already had.
func (a *api) fetchCatalog(ctx context.Context, prov string, stale []string) []string {
	a.cat.mu.Lock()
	f, running := a.cat.inflight[prov]
	if !running {
		f = &catalogFetch{done: make(chan struct{})}
		a.cat.inflight[prov] = f
		go a.runCatalogFetch(ctx, prov, f)
	}
	a.cat.mu.Unlock()
	select {
	case <-f.done:
		return f.ids
	case <-ctx.Done():
		return stale
	}
}

// runCatalogFetch fills one entry and wakes its waiters. A fetch that brings
// nothing keeps the ids already cached: an empty list would drop every model of
// a provider that is briefly unreachable.
func (a *api) runCatalogFetch(ctx context.Context, prov string, f *catalogFetch) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), catalogFetchTimeout.Get())
	defer cancel()
	e := a.fetchCatalogEntry(ctx, prov)
	a.cat.mu.Lock()
	defer a.cat.mu.Unlock()
	if prev, had := a.cat.m[prov]; had && len(e.ids) == 0 && len(prev.ids) > 0 {
		prev.ok, prev.at = false, time.Now()
		e = prev
	}
	a.cat.m[prov] = e
	delete(a.cat.inflight, prov)
	f.ids = e.ids
	close(f.done)
}

// fetchCatalogEntry reads a provider's list and records it in the database: a
// live list drops the models that are gone, a fallback list does not. New
// models start by the provider's policy; under AutoTest they are tested in the
// background.
func (a *api) fetchCatalogEntry(ctx context.Context, prov string) catalogEntry {
	ids, ok := []string(nil), false
	var groups map[string]variantSet
	var raw []byte
	if conns := a.activeConnections(prov); len(conns) > 0 {
		if p, _ := a.providerFor(conns[0]); p.API == translate.Antigravity {
			if ids, raw, ok = a.antigravityModels(ctx, conns[0]); ok {
				ids, groups = groupVariants(ids)
			}
		} else if p.NoModelList {
			// Declared without a list: the declared models are the list.
			ids, ok = append([]string(nil), p.Models...), true
		} else {
			ids, raw, ok = a.fetchModelIDs(ctx, conns[0])
			if p.API == translate.Zen {
				ids, raw, ok = a.zenList(ctx, ids, ok)
			}
		}
	}
	live := ok && len(ids) > 0
	if p, known := provider.Lookup(prov); known && len(p.Models) > 0 && len(ids) == 0 {
		ids, ok = p.Models, true
	}
	if len(ids) > 0 {
		pol := a.modelPolicy(prov)
		added, err := a.store.SyncModels(prov, ids, live, pol.startsOn)
		if err != nil {
			log.Printf("models %s: %v", prov, err)
		} else if pol.AutoTest && len(added) > 0 && a.auto.start(prov) {
			go a.autoTestHeld(prov, added, false)
		}
	}
	return catalogEntry{ids: ids, ok: ok, at: time.Now(), groups: groups, raw: raw,
		info: foldInfos(modelInfos(raw), groups)}
}

// refreshCatalog fetches a provider's list again, however fresh the cache is.
// The cached entry stays until a new one replaces it, so a failed refresh still
// serves the list that works.
func (a *api) refreshCatalog(ctx context.Context, prov string) []string {
	a.cat.mu.Lock()
	ids := a.cat.m[prov].ids
	a.cat.mu.Unlock()
	return a.fetchCatalog(ctx, prov, ids)
}

// fetchModelIDs reads one account's model list and returns its ids.
func (a *api) fetchModelIDs(ctx context.Context, conn store.Connection) ([]string, []byte, bool) {
	resp, err := a.getModels(ctx, conn)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, false
	}
	var d struct {
		Data []struct {
			ID, Name     string
			Capabilities struct {
				Type string `json:"type"`
			} `json:"capabilities"`
			Policy *struct {
				State string `json:"state"`
			} `json:"policy"`
		} `json:"data"`
		Models []json.RawMessage `json:"models"`
		// Cloudflare's model search.
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	// Some lists are a bare array of models (Together).
	if t := bytes.TrimSpace(raw); len(t) > 0 && t[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(t, &arr) != nil {
			return nil, nil, false
		}
		d.Models = arr
	} else if err := json.Unmarshal(raw, &d); err != nil {
		return nil, nil, false
	}
	ids := []string{}
	for _, m := range d.Data {
		// Copilot lists embedding models and models the plan has not enabled.
		if (m.Capabilities.Type != "" && m.Capabilities.Type != "chat") || (m.Policy != nil && m.Policy.State != "enabled") {
			continue
		}
		if m.ID != "" {
			ids = append(ids, m.ID)
		} else if m.Name != "" {
			ids = append(ids, m.Name)
		}
	}
	for _, m := range d.Result {
		if m.Name != "" {
			ids = append(ids, m.Name)
		}
	}
	for _, raw := range d.Models {
		var s string
		var o struct{ ID, Slug, Name string }
		if json.Unmarshal(raw, &s) == nil && s != "" {
			ids = append(ids, s)
		} else if json.Unmarshal(raw, &o) == nil && (o.ID != "" || o.Slug != "" || o.Name != "") {
			ids = append(ids, firstNonEmpty(o.Slug, o.ID, o.Name))
		}
	}
	// Google's OpenAI-shaped list names models "models/<id>"; calls take <id>.
	for i, id := range ids {
		ids[i] = strings.TrimPrefix(id, "models/")
	}
	sort.Strings(ids)
	return ids, raw, true
}

// getModels sends GET <base>/models for one account.
func (a *api) getModels(ctx context.Context, conn store.Connection) (*http.Response, error) {
	p, ok := a.providerFor(conn)
	if !ok {
		return nil, errors.New("provider not reachable")
	}
	secret, err := a.secretFor(ctx, conn.ID)
	if err != nil {
		return nil, err
	}
	if secret, err = a.exchanged(ctx, p, conn.ID, secret); err != nil {
		return nil, err
	}
	base := p.BaseURL
	if over, ok := a.baseOverride[conn.Provider]; ok {
		base = over
	}
	url := base + "/models"
	if p.ModelsPath != "" {
		url = strings.TrimSuffix(base, "/v1") + p.ModelsPath
	}
	if p.ModelsURL != "" {
		url = strings.ReplaceAll(p.ModelsURL, "{accountId}", conn.Meta["accountId"])
	}
	if p.ModelsQuery != "" {
		url += "?" + p.ModelsQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range p.Defaults {
		req.Header.Set(k, v)
	}
	for k, v := range p.Identity {
		req.Header.Set(k, v)
	}
	if p.AuthHeader != "" {
		req.Header.Set(p.AuthHeader, p.AuthPrefix+secret)
	}
	req.Header.Set("Accept-Encoding", "identity")
	return upstream.Do(ctx, req, 2)
}

// bodyModel returns the top-level "model" string of a JSON body.
func bodyModel(body []byte) (string, bool) {
	start, end, err := topLevelValue(body, "model")
	if err != nil {
		return "", false
	}
	var s string
	if json.Unmarshal(body[start:end], &s) != nil {
		return "", false
	}
	return s, true
}

// failover tries the accounts in order from start, wrapping around, and relays
// the first answer that is not a busy status. The last account's answer is
// relayed whatever it is, so the caller sees the real upstream error. An OAuth
// account that answers 401 is refreshed and retried once, which recovers a token
// the provider revoked before its recorded expiry.
func (a *api) failover(w http.ResponseWriter, r *http.Request, body []byte, targets []store.Connection, start int) {
	fc, served := a.newFailoverCall(w, r, body, targets, start)
	if served {
		return
	}
	tried := 0
	attempts := a.attemptsFor(fc.r.Context(), targets, start, fc.body)
	for i, at := range attempts {
		acc, ok := a.resolveAttempt(fc, at)
		if !ok {
			continue
		}
		call, err := a.buildUpstreamCall(fc, acc)
		if err != nil {
			var skip errSkipAccount
			if errors.As(err, &skip) {
				log.Printf("connection %s: %v", acc.conn.ID, skip.err)
				continue
			}
			writeError(w, http.StatusBadRequest, "cannot translate the request: "+err.Error())
			return
		}
		// A same-shape Chat Completions stream is asked for its usage so ccw
		// can count it; the editor in relayAnswer takes it out again.
		injected := false
		if call.to == "" && fc.client == translate.OpenAI {
			if want, _ := shapeFor(acc.p, acc.model); want == translate.OpenAI {
				call.send, injected = withUsage(call.send)
			}
		}
		call.send = filterFor(a, acc.p, acc.conn.Provider, call.send)
		tried++
		resp, err := a.sendWithRecovery(fc, acc, &call)
		if err != nil {
			continue
		}
		// Fail over on a busy status only while another account remains.
		if retryableStatus(resp.StatusCode) {
			log.Printf("connection %s: %s answered %d%s", acc.conn.ID, acc.conn.Provider, resp.StatusCode, modelNote(at.model))
		}
		if retryableStatus(resp.StatusCode) && i < len(attempts)-1 {
			resp.Body.Close()
			continue
		}
		a.relayAnswer(fc, acc, call, resp, injected)
		return
	}
	if tried == 0 {
		writeError(w, http.StatusInternalServerError, "no usable account")
		return
	}
	writeError(w, http.StatusBadGateway, "all accounts failed")
}

// failoverCall is what stays the same across the accounts one request tries.
type failoverCall struct {
	w http.ResponseWriter
	// r carries the Zen session, so every account of the request shares it.
	r      *http.Request
	body   []byte
	client string
	stream bool
	reply  translate.Reply
	// translated keeps the request in each provider shape already built, so
	// accounts of one shape share one translation.
	translated map[string][]byte
}

// accountTry is one account ready to be called: its provider, the bearer to
// send, and the model it is asked for.
type accountTry struct {
	conn   store.Connection
	p      provider.Provider
	secret string
	// model is the model the account is asked for; variant is the level
	// variant ccw chose for it, or "".
	model   string
	variant string
}

// upstreamCall is the request built for one account: the path and body sent,
// and the shapes the answer is translated between ("" when it passes through).
type upstreamCall struct {
	path, to, via string
	send          []byte
}

// newFailoverCall reads what every account of a request shares and fills in
// the client tools and the hosted search a non-Anthropic provider lacks. It
// reports true when the server tools answered the request themselves.
func (a *api) newFailoverCall(w http.ResponseWriter, r *http.Request, body []byte, targets []store.Connection, start int) (*failoverCall, bool) {
	fc := &failoverCall{w: w, client: clientShape(r.PathValue("path")), stream: translate.Stream(body), translated: map[string][]byte{}}
	if fc.client == translate.Anthropic {
		callerModel, _ := r.Context().Value(callerModelKey{}).(string)
		fc.reply = translate.ReplyFor(body, callerModel)
	}
	// A client tool declared by type alone (bash, the editor, memory) gets the
	// schema Anthropic keeps on its side; any other model needs it.
	anthropic := fc.client == translate.Anthropic && r.PathValue("path") == "messages" && !a.anyAnthropic(targets)
	if anthropic {
		var dropped []string
		body, dropped = servertools.FillClientTools(body)
		logDropped(dropped)
	}
	// A client that asks its model to search declares Anthropic's hosted tool,
	// which no other provider has: ccw runs the search and answers with it.
	if anthropic && websearch.Hosted(body) {
		body, fc.reply.Search = a.hostedSearch(r.Context(), body)
	}
	if anthropic && servertools.Wants(body) {
		// The tools Anthropic runs on its servers (the MCP connector, tool
		// search, web fetch) are run here.
		a.serverTools(w, r, body, targets, start, fc.reply.Search)
		return nil, true
	}
	fc.body, fc.r = body, withZenSession(r)
	return fc, false
}

// resolveAttempt finds an attempt's provider and the bearer to send. It
// reports false for an account that cannot be called, which is skipped.
func (a *api) resolveAttempt(fc *failoverCall, at attempt) (accountTry, bool) {
	ctx := fc.r.Context()
	acc := accountTry{conn: at.conn, variant: at.model}
	var ok bool
	if acc.p, ok = a.providerFor(at.conn); !ok {
		return acc, false
	}
	secret, err := a.secretFor(ctx, at.conn.ID)
	if err != nil {
		return acc, false
	}
	if run, _ := ctx.Value(modelTestKey{}).(*testRun); run != nil {
		run.used = at.conn.ID
	}
	if acc.secret, err = a.exchanged(ctx, acc.p, at.conn.ID, secret); err != nil {
		log.Printf("connection %s: %v", at.conn.ID, err)
		return acc, false
	}
	acc.model, _ = bodyModel(fc.body)
	if at.model != "" {
		acc.model = at.model
	}
	return acc, true
}

// buildUpstreamCall builds the request for one account. A caller of one shape
// reaching a provider of the other gets its request translated, and the answer
// translated back; a caller that already speaks the provider's shape is passed
// through untouched. An errSkipAccount fails this account only. On any other
// error outside Zen the call returned is the untranslated request, which the
// Copilot /responses retry relies on.
func (a *api) buildUpstreamCall(fc *failoverCall, acc accountTry) (upstreamCall, error) {
	r, body, client, model := fc.r, fc.body, fc.client, acc.model
	want, wantPath := shapeFor(acc.p, model)
	c := upstreamCall{path: r.PathValue("path"), send: body}
	if acc.p.API == translate.Zen {
		// The free tier reads the whole request and answers each model on
		// one endpoint, so its call is built here, per model.
		if client == "" && c.path == "systemone" {
			var err error
			c.send, err = a.zenSystemOne(r, body, model)
			return c, err
		}
		if client != "" {
			var err error
			c.send, c.path, c.via, err = a.zenRequest(r, body, client, model)
			c.to = client
			return c, err
		}
	}
	if client == "" || !translatable(want) {
		return c, nil
	}
	if want == client {
		// Same shape: bytes pass through, at the provider's own path.
		if wantPath != "" {
			c.path = wantPath
		}
		// Anthropic verifies a history that the other providers accept.
		if client == translate.Anthropic && r.PathValue("path") == "messages" {
			if healed, ok := translate.HealAnthropic(body); ok {
				c.send = healed
			}
		}
		return c, nil
	}
	if want == translate.Antigravity {
		// The envelope names the account's project, so it is built
		// per account from the chat form of the request.
		hub, err := toProvider(body, client, translate.OpenAI)
		if err != nil {
			return c, err
		}
		inner, err := translate.OpenAIToGemini(hub, &a.sigs)
		if err != nil {
			return c, err
		}
		env, err := a.antigravityEnvelope(r.Context(), acc.conn, acc.secret, model, inner)
		if err != nil {
			return c, errSkipAccount{err}
		}
		return upstreamCall{path: wantPath, send: env, to: client, via: want}, nil
	}
	tb, ok := fc.translated[want]
	if !ok {
		var err error
		if tb, err = toProvider(body, client, want); err != nil {
			return c, err
		}
		fc.translated[want] = tb
	}
	return upstreamCall{path: wantPath, send: tb, to: client, via: want}, nil
}

// filterFor applies a provider's own request rules, then the blacklist, which
// runs last on the exact bytes the provider will receive.
func filterFor(a *api, p provider.Provider, prov string, body []byte) []byte {
	body = adjustForProvider(p, body)
	body, _ = filter.Apply(body, a.rulesFor(prov))
	return body
}

// isResponsesOnly reports Copilot's refusal of a model on /chat/completions.
func isResponsesOnly(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "not accessible via the /chat/completions endpoint") ||
		strings.Contains(s, "The requested model is not supported")
}

// send builds and sends one upstream request for a connection.
func (a *api) send(r *http.Request, p provider.Provider, providerID, path, secret string, body []byte) (*http.Response, error) {
	out, err := a.newOutbound(r, p, providerID, path, secret, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// The answer is relayed to the caller, so the wait for the headers and the
	// gap between two reads are bounded, never the whole call: a stream that
	// keeps sending must reach the caller whole.
	return upstream.DoStream(r.Context(), out, 1)
}

// activeConnections returns the active connections of one provider.
func (a *api) activeConnections(prov string) []store.Connection {
	list, err := a.store.ListConnections()
	if err != nil {
		return nil
	}
	out := []store.Connection{}
	for _, c := range list {
		if c.Provider == prov && c.IsActive {
			out = append(out, c)
		}
	}
	return out
}

// retryableStatus reports a status that means "this account is busy, try another".
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusInternalServerError ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusConflict
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// errSkipAccount is a failure of one account (not of the request), after
// which the next account is tried.
type errSkipAccount struct{ err error }

func (e errSkipAccount) Error() string { return e.err.Error() }

func modelNote(m string) string {
	if m == "" {
		return ""
	}
	return " on " + m
}

// checkMaxTokens refuses a Messages request whose max_tokens is missing or not
// a positive integer, as the Messages API does; it returns the reason. A body
// that is not JSON is left to the translation, which names that fault.
func checkMaxTokens(body []byte) string {
	var in map[string]json.RawMessage
	if json.Unmarshal(body, &in) != nil {
		return ""
	}
	raw, ok := in["max_tokens"]
	if !ok {
		return "max_tokens: Field required"
	}
	if n, err := strconv.ParseInt(string(raw), 10, 64); err != nil || n < 1 {
		return "max_tokens: Input should be a positive integer"
	}
	return ""
}
