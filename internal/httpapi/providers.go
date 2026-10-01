package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
	"github.com/naniiluja/ccw/internal/upstream"
	"github.com/naniiluja/ccw/internal/zen"
)

// Cloud Code Assist provisions the account's project on the production host;
// the daily host that serves chat refuses these calls.
var antigravityProdURL = "https://cloudcode-pa.googleapis.com"

// antigravityMetadata identifies the client as the Antigravity IDE.
var antigravityMetadata = map[string]any{"ideType": 9, "platform": 2, "pluginType": 2}

// sigStore remembers the thought signature of each function call for an hour,
// so a follow-up turn can hand it back with the call.
type sigStore struct {
	mu sync.Mutex
	m  map[string]sigEntry
}

func newSigStore() sigStore {
	return sigStore{m: map[string]sigEntry{}}
}

type sigEntry struct {
	sig string
	at  time.Time
}

func (s *sigStore) Get(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[id]; ok && time.Since(e.at) < time.Hour {
		return e.sig
	}
	return ""
}

func (s *sigStore) Put(id, sig string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) > 10000 {
		for k, e := range s.m {
			if time.Since(e.at) > time.Hour {
				delete(s.m, k)
			}
		}
	}
	s.m[id] = sigEntry{sig: sig, at: time.Now()}
}

// antigravityProject returns the Cloud project of an account, asking Cloud
// Code Assist once and keeping the answer in the connection's metadata.
func (a *api) antigravityProject(ctx context.Context, conn store.Connection, token string) (string, error) {
	if p := conn.Meta["projectId"]; p != "" {
		return p, nil
	}
	body, _ := json.Marshal(map[string]any{"metadata": antigravityMetadata})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityProdURL+"/v1internal:loadCodeAssist", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", provider.AntigravityUserAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := upstream.Do(ctx, req, 2)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("loadCodeAssist: status %d", resp.StatusCode)
	}
	var d struct {
		Project json.RawMessage `json:"cloudaicompanionProject"`
	}
	json.Unmarshal(raw, &d)
	var id string
	if json.Unmarshal(d.Project, &id) != nil || id == "" {
		var o struct{ ID string }
		json.Unmarshal(d.Project, &o)
		id = o.ID
	}
	if id == "" {
		return "", fmt.Errorf("loadCodeAssist: no project for this account")
	}
	a.store.SetMeta(conn.ID, map[string]string{"projectId": id})
	return id, nil
}

// antigravityEnvelope wraps an inner Gemini request for Cloud Code Assist.
func (a *api) antigravityEnvelope(ctx context.Context, conn store.Connection, token, model string, inner []byte) ([]byte, error) {
	project, err := a.antigravityProject(ctx, conn, token)
	if err != nil {
		return nil, err
	}
	var req map[string]any
	d := json.NewDecoder(bytes.NewReader(inner))
	d.UseNumber()
	if err := d.Decode(&req); err != nil {
		return nil, err
	}
	// A stable session per account lets the upstream reuse its cache.
	h := sha256.Sum256([]byte("antigravity:" + conn.ID))
	req["sessionId"] = "-" + strconv.FormatUint(binary.BigEndian.Uint64(h[:8])&0x7fffffffffffffff, 10)
	contents, _ := req["contents"].([]any)
	step := 2*len(contents) - 1
	if step < 1 {
		step = 1
	}
	env := map[string]any{
		"project":     project,
		"model":       model,
		"userAgent":   "antigravity",
		"requestType": "agent",
		"requestId":   fmt.Sprintf("agent/%s/%d/%s/%d", uuid4(), time.Now().UnixMilli(), uuid4(), step),
		"request":     req,
	}
	return json.Marshal(env)
}

func uuid4() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// antigravityModels lists an account's models with fetchAvailableModels.
func (a *api) antigravityModels(ctx context.Context, conn store.Connection) ([]string, []byte, bool) {
	token, err := a.secretFor(ctx, conn.ID)
	if err != nil {
		return nil, nil, false
	}
	project, _ := a.antigravityProject(ctx, conn, token)
	p, _ := a.providerFor(conn)
	base := p.BaseURL
	if over, ok := a.baseOverride[conn.Provider]; ok {
		base = over
	}
	body, _ := json.Marshal(map[string]any{"project": project})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1internal:fetchAvailableModels", bytes.NewReader(body))
	if err != nil {
		return nil, nil, false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", provider.AntigravityUserAgent)
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", provider.AntigravityVersion)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := upstream.Do(ctx, req, 2)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}
	var d struct {
		Models map[string]struct {
			IsInternal bool `json:"isInternal"`
		} `json:"models"`
		// Deprecated models stay listed but refuse calls (400); the value
		// names the replacement, which is listed on its own.
		Deprecated map[string]json.RawMessage `json:"deprecatedModelIds"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil || json.Unmarshal(raw, &d) != nil {
		return nil, nil, false
	}
	ids := []string{}
	for id, m := range d.Models {
		if _, gone := d.Deprecated[id]; !m.IsInternal && !gone {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, raw, true
}

// copilotTokenURL trades a GitHub OAuth token for a Copilot token. It is a
// variable so a test can point it at a fake.
var copilotTokenURL = "https://api.github.com/copilot_internal/v2/token"

type copilotToken struct {
	token string
	exp   time.Time
}

// copilotCache holds the Copilot token of each connection until it is close
// to expiry, so the exchange runs about once every half hour, not per request.
type copilotCache struct {
	mu sync.Mutex
	m  map[string]copilotToken
}

func newCopilotCache() copilotCache {
	return copilotCache{m: map[string]copilotToken{}}
}

// exchanged returns the bearer to send for a provider whose stored credential
// must be exchanged first, or the credential itself when it needs none.
func (a *api) exchanged(ctx context.Context, p provider.Provider, connID, secret string) (string, error) {
	if p.Exchange != "copilot" {
		return secret, nil
	}
	a.copilot.mu.Lock()
	t, ok := a.copilot.m[connID]
	a.copilot.mu.Unlock()
	if ok && time.Until(t.exp) > 5*time.Minute {
		return t.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+secret)
	req.Header.Set("User-Agent", "GitHubCopilotChat/"+provider.CopilotChatVersion)
	req.Header.Set("Editor-Version", "vscode/"+provider.CopilotVSCodeVersion)
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/"+provider.CopilotChatVersion)
	req.Header.Set("X-Github-Api-Version", provider.CopilotAPIVersion)
	req.Header.Set("Accept", "application/json")
	resp, err := upstream.Do(ctx, req, 2)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copilot token exchange: status %d", resp.StatusCode)
	}
	var d struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &d); err != nil || d.Token == "" {
		return "", fmt.Errorf("copilot token exchange: no token in answer")
	}
	t = copilotToken{token: d.Token, exp: time.Unix(d.ExpiresAt, 0)}
	a.copilot.mu.Lock()
	a.copilot.m[connID] = t
	a.copilot.mu.Unlock()
	return t.token, nil
}

// dropExchanged forgets a connection's exchanged token after the upstream
// refused it, so the next call exchanges again.
func (a *api) dropExchanged(connID string) {
	a.copilot.mu.Lock()
	delete(a.copilot.m, connID)
	a.copilot.mu.Unlock()
}

// copilotTokenLimit matches the models that take max_completion_tokens and
// reject max_tokens on Copilot.
var copilotTokenLimit = regexp.MustCompile(`(?i)gpt-5|o[134]-`)

// adjustForProvider applies a provider's own request rules that a blacklist
// cannot express (a rename, not a removal).
func adjustForProvider(p provider.Provider, body []byte) []byte {
	if p.Exchange != "copilot" {
		return body
	}
	model, _ := bodyModel(body)
	if !copilotTokenLimit.MatchString(model) {
		return body
	}
	start, end, err := topLevelValue(body, "max_tokens")
	if err != nil {
		return body
	}
	if _, _, err := topLevelValue(body, "max_completion_tokens"); err == nil {
		return body
	}
	// Rename the key in place: find the quoted name just before the value.
	k := lastIndexBefore(body, start, `"max_tokens"`)
	if k < 0 {
		return body
	}
	_ = end
	out := append([]byte{}, body[:k]...)
	out = append(out, `"max_completion_tokens"`...)
	return append(out, body[k+len(`"max_tokens"`):]...)
}

func lastIndexBefore(b []byte, before int, s string) int {
	for i := before - len(s); i >= 0; i-- {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}

// secret0 re-reads a connection's stored credential for a second exchange.
func secret0(a *api, r *http.Request, connID string) string {
	s, _ := a.secretFor(r.Context(), connID)
	return s
}

// The OpenCode Zen free tier (the "opencode" provider) reads the request it gets
// and only answers what looks like an OpenCode agent. This file is where a call
// to it is shaped; the parts that do not depend on HTTP are in internal/zen.

const modelsDevURL = "https://models.dev/api.json"

// zenState holds what the Zen provider keeps between calls.
type zenState struct {
	// pool maps a caller's conversation to an upstream session, so the upstream's
	// prompt cache stays warm for as long as the conversation talks.
	pool *zen.Pool
	// cat is models.dev: which models reason, cost nothing, and speak Responses.
	cat *zen.Source
}

func newZenState() zenState {
	return zenState{
		pool: zen.NewPool(10*time.Minute, 256),
		cat:  &zen.Source{URL: modelsDevURL, TTL: 24 * time.Hour},
	}
}

// zenSessionKey carries the upstream session of the call in progress from where
// the request is built to where its headers are set.
type zenSessionKey struct{}

type zenSession struct{ id, key string }

func zenSessionOf(r *http.Request) *zenSession {
	s, _ := r.Context().Value(zenSessionKey{}).(*zenSession)
	return s
}

// withZenSession gives a call a place to note its upstream session.
func withZenSession(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), zenSessionKey{}, &zenSession{}))
}

// noteZenSession records the session a call will use.
func noteZenSession(r *http.Request, s zen.Session) {
	if h := zenSessionOf(r); h != nil {
		h.id, h.key = s.ID, s.Key
	}
}

// releaseZenSession gives back the session of a call that never reached the
// upstream: nothing was cached there, so the next call mints a fresh id.
func (a *api) releaseZenSession(r *http.Request) {
	if h := zenSessionOf(r); h != nil && h.key != "" {
		a.zen.pool.Release(h.key)
		h.id, h.key = "", ""
	}
}

// zenRequest builds what one call sends to the free tier. client is the shape the
// caller speaks; body is the caller's own request, so the caller's session id is
// read from it before a translation drops the fields it lives in. It returns the
// body, the upstream path and the shape the answer comes back in.
func (a *api) zenRequest(r *http.Request, body []byte, client, model string) (send []byte, path, via string, err error) {
	id, ep, pin := zen.Route(model, func() *zen.Catalogue { return a.zen.cat.Get(r.Context()) })
	if ep == zen.SystemOne {
		return nil, "", "", fmt.Errorf("%s answers System One calls only: POST /v1/systemone", model)
	}
	hub, err := toProvider(body, client, translate.OpenAI)
	if err != nil {
		return nil, "", "", err
	}
	if id != model {
		var ok bool
		if hub, ok = setModel(hub, id); !ok {
			return nil, "", "", fmt.Errorf("cannot rewrite the model")
		}
	}
	var payload map[string]any
	json.Unmarshal(body, &payload)
	caller, source := zen.Caller(r.Header, payload)
	sess := a.zen.pool.For(caller, source, body)
	noteZenSession(r, sess)
	a.logFor(r.Context()).Info("zen.session.use", "model", id, "session", sess.ID, "uses", sess.Uses, "caller", zenWho(sess))

	if ep == zen.Responses {
		send, err = zen.PrepareResponses(hub, sess.ID, pin)
		return send, "responses", translate.ZenResponses, err
	}
	send, err = zen.PrepareChat(hub, pin)
	return send, "chat/completions", translate.Zen, err
}

// zenSystemOne builds a System One call. It carries no conversation, so its
// session is keyed on the body.
func (a *api) zenSystemOne(r *http.Request, body []byte, model string) ([]byte, error) {
	id, ep, _ := zen.Route(model, nil)
	if ep != zen.SystemOne {
		id = model // a name with no System One profile goes upstream as it came
	}
	noteZenSession(r, a.zen.pool.For("", "", body))
	return zen.PrepareSystemOne(body, id)
}

func zenWho(s zen.Session) string {
	if s.Caller == "" {
		return "-"
	}
	return s.Caller + "(" + s.Source + ")"
}

// zenList narrows what the upstream lists to the models the anonymous key can
// call, with their limits and thinking. When the upstream cannot answer, the
// models ccw knows stand in for them.
func (a *api) zenList(ctx context.Context, ids []string, ok bool) ([]string, []byte, bool) {
	if ok && len(ids) > 0 {
		if kept, doc := zen.List(ids, a.zen.cat.Get(ctx)); len(kept) > 0 {
			return kept, doc, true
		}
	}
	return zen.ProfileIDs(), nil, false
}

// relayZenSystemOne answers a System One call with the fields of its shape.
func (a *api) relayZenSystemOne(w http.ResponseWriter, resp *http.Response, connID string) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTranslatedBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot read the upstream answer")
		return
	}
	a.rate.capture(connID, resp.Header)
	if kept, ok := zen.SystemOneAnswer(body); ok {
		body = kept
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
	a.recordUsage(a.respLog(resp), connID, keyIDOf(resp), body, "")
}

// zenSessions shows the held sessions, so reuse can be watched: which caller
// conversation holds which upstream id, where its id was read from, and when the
// mapping would be dropped. It is for an admin: it names the callers' own ids.
func (a *api) zenSessions(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ttl := a.zen.pool.TTL()
	live := []map[string]any{}
	for _, s := range a.zen.pool.Live() {
		live = append(live, map[string]any{
			"id": s.ID, "caller": s.Caller, "source": s.Source, "uses": s.Uses,
			"created": s.Created, "lastUsed": s.LastUsed,
			"expiresInSeconds": max(0, int((ttl - now.Sub(s.LastUsed)).Seconds())),
		})
	}
	writeJSON(w, map[string]any{"count": len(live), "ttlSeconds": int(ttl.Seconds()), "maxSessions": a.zen.pool.Max(), "sessions": live})
}

// systemOneModel reports whether a model answers System One calls: any model of
// a TypeSafe provider, and the Zen models profiled for it (jev). A model of the
// Zen provider that answers chat is not one, though it shares the provider.
func (a *api) systemOneModel(model string) bool {
	prov, rest := a.splitModel(model)
	p, ok := provider.Lookup(prov)
	if prov == "" || !ok {
		return false
	}
	switch p.API {
	case "typesafe":
		return true
	case translate.Zen:
		pr, known := zen.Resolve(rest)
		return known && pr.Endpoint == zen.SystemOne
	}
	return false
}

// chatModel reports whether a model is one ccw serves for chat: it names a
// provider and is not a System One model.
func (a *api) chatModel(model string) bool {
	prov, _ := a.splitModel(model)
	_, ok := provider.Lookup(prov)
	return prov != "" && ok && !a.systemOneModel(model)
}
