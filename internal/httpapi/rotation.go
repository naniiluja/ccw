package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
)

// Rotation is how a provider's accounts take turns:
//   - round-robin: each account serves Sticky requests in a row, then the next
//     one takes over (Sticky 1 turns on every request);
//   - fallback: the first account in Order serves every request, and the next
//     one only when it fails or is busy.
//
// Order is the accounts' priority, first first; accounts it does not name
// follow in the order they were added.
type Rotation struct {
	Mode   string   `json:"mode"`
	Sticky int      `json:"sticky"`
	Order  []string `json:"order"`
}

const (
	RotateRoundRobin = "round-robin"
	RotateFallback   = "fallback"
)

// rrCursor is a rotation's place: the account index and how many requests it
// has served in a row.
type rrCursor struct{ i, used int }

func rotationKey(prov string) string { return "rotation:" + prov }

func (a *api) rotation(prov string) Rotation {
	r := Rotation{Mode: RotateRoundRobin, Sticky: 1}
	if v, _ := a.store.GetSetting(rotationKey(prov)); v != "" {
		json.Unmarshal([]byte(v), &r)
	}
	if r.Mode != RotateFallback {
		r.Mode = RotateRoundRobin
	}
	if r.Sticky < 1 {
		r.Sticky = 1
	}
	return r
}

// ordered sorts a provider's connections by the rotation's priority.
func (r Rotation) ordered(conns []store.Connection) []store.Connection {
	pos := map[string]int{}
	for i, id := range r.Order {
		pos[id] = i
	}
	out := append([]store.Connection(nil), conns...)
	sort.SliceStable(out, func(i, j int) bool {
		pi, iok := pos[out[i].ID]
		pj, jok := pos[out[j].ID]
		switch {
		case iok && jok:
			return pi < pj
		case iok != jok:
			return iok
		}
		return false
	})
	return out
}

// startFor picks the first account a request tries and advances the
// rotation. One provider's accounts follow its rotation; a pool across
// providers (a bare model several providers list) turns on every request,
// per model.
func (a *api) startFor(model string, targets []store.Connection) ([]store.Connection, int) {
	prov := targets[0].Provider
	for _, c := range targets[1:] {
		if c.Provider != prov {
			return withStandbyLast(targets, func(n int) int { return a.advance("m:"+model, n, 1) })
		}
	}
	rot := a.rotation(prov)
	targets = rot.ordered(targets)
	if rot.Mode == RotateFallback {
		return withStandbyLast(targets, func(int) int { return 0 })
	}
	return withStandbyLast(targets, func(n int) int { return a.advance("p:"+prov, n, rot.Sticky) })
}

// withStandbyLast lets the normal accounts take turns from the index pick
// returns, and puts the standby accounts after all of them, so a standby is
// tried only when every normal account failed. The list it returns starts at
// index 0.
func withStandbyLast(targets []store.Connection, pick func(n int) int) ([]store.Connection, int) {
	var normal, standby []store.Connection
	for _, c := range targets {
		if c.Standby {
			standby = append(standby, c)
		} else {
			normal = append(normal, c)
		}
	}
	if len(standby) == 0 {
		return targets, pick(len(targets))
	}
	out := make([]store.Connection, 0, len(targets))
	if len(normal) > 0 {
		i := pick(len(normal))
		out = append(append(out, normal[i:]...), normal[:i]...)
	}
	return append(out, standby...), 0
}

// advance returns the cursor's account for this request and moves it on
// once that account has served sticky requests in a row.
func (a *api) advance(key string, n, sticky int) int {
	a.rrMu.Lock()
	defer a.rrMu.Unlock()
	c := a.rrNext[key]
	c.i %= n
	i := c.i
	c.used++
	if c.used >= sticky {
		c.i, c.used = (c.i+1)%n, 0
	}
	a.rrNext[key] = c
	return i
}

// getRotation serves a provider's rotation and whose turn it is:
// {"rotation":{…},"next":"<connection id>","used":n}.
func (a *api) getRotation(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	writeJSON(w, a.rotationState(prov))
}

func (a *api) rotationState(prov string) map[string]any {
	rot := a.rotation(prov)
	conns := rot.ordered(a.activeConnections(prov))
	out := map[string]any{"rotation": rot}
	if len(conns) == 0 {
		return out
	}
	if rot.Mode == RotateFallback {
		conns, _ = withStandbyLast(conns, func(int) int { return 0 })
		out["next"] = conns[0].ID
		return out
	}
	a.rrMu.Lock()
	c := a.rrNext["p:"+prov]
	a.rrMu.Unlock()
	conns, i := withStandbyLast(conns, func(n int) int { return c.i % n })
	out["next"], out["used"] = conns[i].ID, c.used
	return out
}

// setRotation stores {"mode","sticky","order"}; fields left out keep their
// value.
func (a *api) setRotation(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	var b struct {
		Mode   *string   `json:"mode"`
		Sticky *int      `json:"sticky"`
		Order  *[]string `json:"order"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	rot := a.rotation(prov)
	if b.Mode != nil {
		if *b.Mode != RotateRoundRobin && *b.Mode != RotateFallback {
			writeError(w, http.StatusBadRequest, "mode is round-robin or fallback")
			return
		}
		rot.Mode = *b.Mode
	}
	if b.Sticky != nil {
		if *b.Sticky < 1 || *b.Sticky > 1000 {
			writeError(w, http.StatusBadRequest, "sticky is 1 to 1000")
			return
		}
		rot.Sticky = *b.Sticky
	}
	if b.Order != nil {
		rot.Order = *b.Order
	}
	raw, _ := json.Marshal(rot)
	if err := a.store.SetSetting(rotationKey(prov), string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot save")
		return
	}
	// A new rotation starts from its first account.
	a.rrMu.Lock()
	delete(a.rrNext, "p:"+prov)
	a.rrMu.Unlock()
	writeJSON(w, a.rotationState(prov))
}

// attempt is one try of a request: an account, and the upstream model when
// ccw picks it ("" keeps the request's own).
type attempt struct {
	conn  store.Connection
	model string
}

// attemptsFor orders the tries of a request: every account in rotation
// order; then, for a level variant that every account refused as busy, every
// account again on the model's default variant (a -high is refused for
// capacity far more often than the -tiered one).
func (a *api) attemptsFor(ctx context.Context, targets []store.Connection, start int, body []byte) []attempt {
	model, _ := bodyModel(body)
	out := make([]attempt, 0, len(targets))
	var fallback []attempt
	for i := range targets {
		c := targets[(start+i)%len(targets)]
		p, ok := a.providerFor(c)
		if !ok || p.API != translate.Antigravity {
			out = append(out, attempt{conn: c})
			continue
		}
		a.catalogIDs(ctx, c.Provider) // the variants come with the list
		picked := a.resolveVariant(c.Provider, model, body)
		out = append(out, attempt{conn: c, model: picked})
		if g, ok := a.variants(c.Provider)[a.variantBase(c.Provider, picked)]; ok {
			if def := g.ids[g.def]; def != "" && def != picked {
				fallback = append(fallback, attempt{conn: c, model: def})
			}
		}
	}
	return append(out, fallback...)
}
