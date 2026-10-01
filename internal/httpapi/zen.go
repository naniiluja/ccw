package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/translate"
	"github.com/naniiluja/ccw/internal/zen"
)

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
	log.Printf("zen: model=%s session=%s uses=%d caller=%s", id, sess.ID, sess.Uses, zenWho(sess))

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
	a.recordUsage(connID, keyIDOf(resp), body, "")
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
