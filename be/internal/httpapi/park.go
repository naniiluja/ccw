package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// Parking an account whose provider has run out of room for a model.
//
// A 429 means this account cannot answer this model right now. Without a record
// of that, every later request tries it again, pays the round trip, and gets the
// same refusal: one account with a spent quota slows every request behind it and
// still answers none. A parked account is skipped until the moment the provider
// itself named, and is then tried again as if nothing had happened.
//
// Two sources say when that moment is, and either alone is enough. A provider that
// reports its quota gives a window ccw already cached: when the model's window is
// spent, the window's own reset time is the moment. A provider that reports no
// quota says it in the answer instead, in Retry-After. OpenCode Zen is the second
// kind: it has no quota endpoint, and its free tier answers 429 with a Retry-After
// that counts down to the top of the next UTC day.
//
// Neither is trusted forever. A reset time in the past is not a park, and a
// missing or unparsable one parks nothing, so an account is never put aside for
// good over a header ccw did not understand.

// parkMax is how long one Retry-After can hold an account. A provider that asks
// for longer than this is either wrong or has dropped the account rather than its
// quota, and an account parked for a day would look like a broken gateway.
const parkMax = 12 * time.Hour

// park is when one account is held out of rotation, and until when. The key is an
// account and a model: an account can answer one model and be spent for another.
type park struct {
	mu   sync.Mutex
	held map[string]time.Time
}

func newPark() park { return park{held: map[string]time.Time{}} }

func parkKey(connID, model string) string { return connID + "\x00" + model }

// until answers when the account's park for the model runs out, or the zero time
// when it is not parked: past that moment the entry is dropped, so the map holds
// no account forever.
func (p *park) until(connID, model string) time.Time {
	key := parkKey(connID, model)
	p.mu.Lock()
	defer p.mu.Unlock()
	at, ok := p.held[key]
	if !ok {
		return time.Time{}
	}
	if !at.After(time.Now()) {
		delete(p.held, key)
		return time.Time{}
	}
	return at
}

// set parks an account until a moment. A moment in the past parks nothing.
func (p *park) set(connID, model string, at time.Time) {
	if !at.After(time.Now()) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held == nil {
		p.held = map[string]time.Time{}
	}
	p.held[parkKey(connID, model)] = at
}

// forget drops one park, for a model whose refusal turned out to be something else.
func (p *park) forget(connID, model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.held, parkKey(connID, model))
}

// forgetAll drops every park an account holds, for a 401: the account's credential
// is the problem, not its quota, so no model of it may stay out of rotation over a
// 429 another model once earned.
func (p *park) forgetAll(connID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.held {
		if id, _, ok := strings.Cut(k, "\x00"); ok && id == connID {
			delete(p.held, k)
		}
	}
}

// parkFromQuota reads the quota ccw already cached for the account, with no read
// of the provider: a request must not wait on a quota endpoint to learn that an
// account it was going to skip is spent. Zero when the quota is not spent, or when
// no window says it is.
func (a *api) parkFromQuota(connID, model string) time.Time {
	a.quota.mu.Lock()
	q, ok := a.quota.m[connID]
	a.quota.mu.Unlock()
	if !ok {
		return time.Time{}
	}
	if left := quotaLeft(q, model); left < 0 || left > 0 {
		return time.Time{}
	}
	var until time.Time
	for _, w := range q.Windows {
		if w.UsedPct < 100 || w.ResetAt == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, w.ResetAt); err == nil && t.After(until) {
			until = t
		}
	}
	if !until.After(time.Now()) {
		return time.Time{}
	}
	return until
}

// parkFromHeader reads Retry-After, in the two forms RFC 9110 allows: a number of
// seconds, or an HTTP date. Anything else, and anything past parkMax, parks
// nothing.
func parkFromHeader(h http.Header) time.Time {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return time.Time{}
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return time.Time{}
		}
		d := time.Duration(secs) * time.Second
		if d > parkMax {
			d = parkMax
		}
		return time.Now().Add(d)
	}
	at, err := http.ParseTime(raw)
	if err != nil {
		return time.Time{}
	}
	// A date that has passed names no moment to wait for, so it parks nothing.
	if !at.After(time.Now()) {
		return time.Time{}
	}
	return at
}

// parkAccount holds an account out of rotation for a model after its provider
// answered 429. The header is read first: it is what the provider just said about
// this request, where the cached quota was read before the 429 arrived and may be
// older than it.
func (a *api) parkAccount(conn store.Connection, model string, resp *http.Response) {
	if resp.StatusCode != http.StatusTooManyRequests {
		return
	}
	if model == "" {
		return
	}
	until := parkFromHeader(resp.Header)
	if until.IsZero() {
		until = a.parkFromQuota(conn.ID, model)
	}
	if until.IsZero() {
		return
	}
	a.parks.set(conn.ID, model, until)
	a.logFor(context.Background()).Info("proxy.account.park",
		"connection", conn.ID, "provider", conn.Provider, "model", model, "until", until.UTC().Format(time.RFC3339))
}

// spentUntil is when the account is held out for the model, from a Retry-After
// the provider just sent or from a quota ccw already cached. The zero time means
// it is not parked.
func (a *api) spentUntil(connID, model string) time.Time {
	if model == "" {
		return time.Time{}
	}
	if at := a.parks.until(connID, model); !at.IsZero() {
		return at
	}
	return a.parkFromQuota(connID, model)
}

// skipSpent drops the attempts whose account is parked for the model until its
// reset. When every account is parked they are all kept, so the caller gets the
// provider's own answer instead of ccw's guess that nothing could answer: a
// gateway that answers "no usable account" for a provider that would have said
// why is worse than one that relays the truth.
func (a *api) skipSpent(attempts []attempt, model string) []attempt {
	kept := make([]attempt, 0, len(attempts))
	for _, at := range attempts {
		m := model
		if at.model != "" {
			m = at.model
		}
		if a.spentUntil(at.conn.ID, m).IsZero() {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		return attempts
	}
	return kept
}
