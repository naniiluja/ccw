package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/upstream"
)

// QuotaWindow is one limit of an account: a rolling window, a monthly pool,
// a model's share. UsedPct is 0–100, or -1 when the provider gives no ratio.
type QuotaWindow struct {
	Name      string  `json:"name"`
	UsedPct   float64 `json:"usedPct"`
	Used      string  `json:"used,omitempty"`
	Limit     string  `json:"limit,omitempty"`
	ResetAt   string  `json:"resetAt,omitempty"`
	Unlimited bool    `json:"unlimited,omitempty"`
}

// AccountQuota is what ccw knows of one account's quota.
type AccountQuota struct {
	ConnectionID string        `json:"connectionId"`
	Provider     string        `json:"provider"`
	Label        string        `json:"label"`
	Plan         string        `json:"plan,omitempty"`
	Source       string        `json:"source"` // "api": read from the provider; "headers": from the last answer
	Windows      []QuotaWindow `json:"windows"`
	Resets       []QuotaReset  `json:"resets"`
	ResetsError  string        `json:"resetsError,omitempty"`
	Error        string        `json:"error,omitempty"`
	FetchedAt    string        `json:"fetchedAt"`

	// Block-present flags for Claude fresh read validation in claimReset.
	HasJuniperTide bool `json:"-"`
	HasCedarEmber  bool `json:"-"`
}

const quotaTTL = time.Minute

type quotaFlight struct {
	seq  uint64
	done chan struct{}
	res  AccountQuota
}

type quotaCache struct {
	mu        sync.Mutex
	seq       atomic.Uint64
	m         map[string]AccountQuota
	highWater map[string]uint64
	flights   map[string]*quotaFlight
}

func newQuotaCache() quotaCache {
	return quotaCache{m: map[string]AccountQuota{}, highWater: map[string]uint64{}, flights: map[string]*quotaFlight{}}
}

// quotaFetchers read an account's quota from the provider's own endpoint.
var quotaFetchers = map[string]func(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error){}

// quotaFor returns an account's quota: from the provider when it has a quota
// endpoint, otherwise from the rate-limit headers of its last answer.
//
// Without refresh, a cached read younger than quotaTTL answers, and callers
// that miss together share one read (a flight). With refresh, the caller reads
// on its own. Either way a read is stored only when its sequence number is
// above the last one stored, so an older read never overwrites a newer one.
func (a *api) quotaFor(ctx context.Context, c store.Connection, refresh bool) AccountQuota {
	// The cache check, the flight join and the flight start share one lock
	// hold, so two callers that miss together cannot both start a read.
	a.quota.mu.Lock()
	a.quota.ensureMaps()
	if q, ok := a.quota.freshEntry(c.ID, refresh); ok {
		a.quota.mu.Unlock()
		return q
	}
	if f := a.quota.joinableFlight(c.ID, refresh); f != nil {
		a.quota.mu.Unlock()
		return a.awaitQuotaFlight(ctx, c, f)
	}
	seq := a.quota.seq.Add(1)
	var myFlight *quotaFlight
	if !refresh {
		myFlight = &quotaFlight{seq: seq, done: make(chan struct{})}
		a.quota.flights[c.ID] = myFlight
	}
	a.quota.mu.Unlock()

	if refresh {
		res := a.fetchQuota(ctx, c)
		a.storeQuota(c.ID, seq, res, nil)
		return res
	}
	go a.leadQuotaFlight(ctx, c, seq, myFlight)
	return a.awaitQuotaFlight(ctx, c, myFlight)
}

// ensureMaps makes the maps of a zero quotaCache. The caller holds q.mu.
func (q *quotaCache) ensureMaps() {
	if q.m == nil {
		q.m = map[string]AccountQuota{}
	}
	if q.highWater == nil {
		q.highWater = map[string]uint64{}
	}
	if q.flights == nil {
		q.flights = map[string]*quotaFlight{}
	}
}

// freshEntry returns the cached read of a connection when it is younger than
// quotaTTL and refresh is off. The caller holds q.mu.
func (q *quotaCache) freshEntry(connID string, refresh bool) (AccountQuota, bool) {
	cached, hit := q.m[connID]
	if !hit || refresh {
		return AccountQuota{}, false
	}
	if t, err := time.Parse(time.RFC3339, cached.FetchedAt); err == nil && time.Since(t) < quotaTTL {
		return cached, true
	}
	return AccountQuota{}, false
}

// joinableFlight returns the read in flight for a connection when it started
// after the last stored read and refresh is off. The caller holds q.mu.
func (q *quotaCache) joinableFlight(connID string, refresh bool) *quotaFlight {
	if refresh {
		return nil
	}
	if f := q.flights[connID]; f != nil && f.seq > q.highWater[connID] {
		return f
	}
	return nil
}

// awaitQuotaFlight waits for a flight, or for ctx. A finished flight answers
// with the cache, which a newer read may have replaced, else its own result.
func (a *api) awaitQuotaFlight(ctx context.Context, c store.Connection, f *quotaFlight) AccountQuota {
	select {
	case <-f.done:
		a.quota.mu.Lock()
		cached, ok := a.quota.m[c.ID]
		a.quota.mu.Unlock()
		if ok {
			return cached
		}
		return f.res
	case <-ctx.Done():
		return AccountQuota{
			ConnectionID: c.ID, Provider: c.Provider, Label: c.Label,
			Windows: []QuotaWindow{}, Resets: []QuotaReset{},
			Error:     ctx.Err().Error(),
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}
}

// leadQuotaFlight runs the read of a flight, stores it and ends the flight.
func (a *api) leadQuotaFlight(ctx context.Context, c store.Connection, seq uint64, f *quotaFlight) {
	a.storeQuota(c.ID, seq, a.fetchQuota(ctx, c), f)
}

// storeQuota stores a read under the sequence rule. With a flight, it also
// hands the read to the flight's waiters and ends the flight.
func (a *api) storeQuota(connID string, seq uint64, res AccountQuota, f *quotaFlight) {
	a.quota.mu.Lock()
	defer a.quota.mu.Unlock()
	if seq > a.quota.highWater[connID] {
		a.quota.highWater[connID] = seq
		a.quota.m[connID] = res
	}
	if f == nil {
		return
	}
	f.res = res
	close(f.done)
	if a.quota.flights[connID] == f {
		delete(a.quota.flights, connID)
	}
}

// fetchQuota reads a connection's quota from its provider's endpoint, and
// falls back to the rate-limit headers of its last answer when that gives no
// window. Windows and Resets are never nil.
func (a *api) fetchQuota(ctx context.Context, c store.Connection) AccountQuota {
	res := AccountQuota{
		ConnectionID: c.ID, Provider: c.Provider, Label: c.Label,
		Windows: []QuotaWindow{}, Resets: []QuotaReset{},
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if fetch, ok := quotaFetchers[c.Provider]; ok {
		token, err := a.secretFor(fctx, c.ID)
		if err == nil {
			var got AccountQuota
			if got, err = fetch(a, fctx, c, token); err == nil {
				got.ConnectionID, got.Provider, got.Label, got.Source, got.FetchedAt = c.ID, c.Provider, c.Label, "api", res.FetchedAt
				res = got
			}
		}
		if err != nil {
			res.Error = err.Error()
		}
	}
	if len(res.Windows) == 0 {
		if snap, ok := a.rate.get(c.ID); ok {
			res.Windows = windowsFromHeaders(snap.Headers)
			res.Source = "headers"
			if len(res.Windows) > 0 {
				res.Error = ""
			}
		}
	}
	if res.Windows == nil {
		res.Windows = []QuotaWindow{}
	}
	if res.Resets == nil {
		res.Resets = []QuotaReset{}
	}
	return res
}

// freshQuota calls the fetcher directly, no header fallback, no flight,
// stores through the sequence rule, fills the same fields as quotaFor,
// 20-second timeout on context.WithoutCancel.
func (a *api) freshQuota(ctx context.Context, c store.Connection) (AccountQuota, error) {
	fetch, ok := quotaFetchers[c.Provider]
	if !ok {
		return AccountQuota{}, fmt.Errorf("no quota fetcher for provider %q", c.Provider)
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	token, err := a.secretFor(fctx, c.ID)
	if err != nil {
		return AccountQuota{}, err
	}
	seq := a.quota.seq.Add(1)
	got, err := fetch(a, fctx, c, token)
	if err != nil {
		return AccountQuota{}, err
	}
	got.ConnectionID = c.ID
	got.Provider = c.Provider
	got.Label = c.Label
	got.Source = "api"
	got.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	if got.Windows == nil {
		got.Windows = []QuotaWindow{}
	}
	if got.Resets == nil {
		got.Resets = []QuotaReset{}
	}
	a.quota.mu.Lock()
	if a.quota.m == nil {
		a.quota.m = map[string]AccountQuota{}
	}
	if a.quota.highWater == nil {
		a.quota.highWater = map[string]uint64{}
	}
	if seq > a.quota.highWater[c.ID] {
		a.quota.highWater[c.ID] = seq
		a.quota.m[c.ID] = got
	}
	a.quota.mu.Unlock()
	return got, nil
}

// invalidateQuota clears an account's quota cache entry and raises the high-water mark.
func (a *api) invalidateQuota(connID string) {
	a.quota.mu.Lock()
	if a.quota.m != nil {
		delete(a.quota.m, connID)
	}
	if a.quota.highWater == nil {
		a.quota.highWater = map[string]uint64{}
	}
	a.quota.highWater[connID] = a.quota.seq.Add(1)
	a.quota.mu.Unlock()
}

// quotaList serves every active account's quota, read in parallel.
// Query: provider, connection, refresh=1.
func (a *api) quotaList(allowRefresh bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conns, err := a.store.ListConnections()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cannot read connections")
			return
		}
		wantProvider := r.URL.Query().Get("provider")
		wantConn := r.URL.Query().Get("connection")
		refresh := false
		if allowRefresh && r.URL.Query().Get("refresh") == "1" {
			sfs := r.Header.Get("Sec-Fetch-Site")
			if sfs == "" || sfs == "same-origin" {
				refresh = true
			}
		}
		var list []store.Connection
		for _, c := range conns {
			if _, ok := a.providerFor(c); ok && c.IsActive &&
				(wantProvider == "" || c.Provider == wantProvider) &&
				(wantConn == "" || c.ID == wantConn) {
				list = append(list, c)
			}
		}
		out := make([]AccountQuota, len(list))
		var wg sync.WaitGroup
		for i, c := range list {
			wg.Add(1)
			go func(i int, c store.Connection) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
				defer cancel()
				out[i] = a.quotaFor(ctx, c, refresh)
			}(i, c)
		}
		wg.Wait()
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Provider != out[j].Provider {
				return out[i].Provider < out[j].Provider
			}
			return out[i].Label < out[j].Label
		})
		writeJSON(w, map[string]any{"accounts": out})
	}
}

// windowsFromHeaders reads the rate-limit headers providers send with each
// answer: OpenAI-style x-ratelimit-{limit,remaining,reset}-{requests,tokens},
// Codex's x-codex-{primary,secondary}-*, Copilot's x-quota-snapshot-*.
// It never returns nil: the JSON of one account with null windows breaks the
// whole Quota page.
func windowsFromHeaders(h map[string]string) []QuotaWindow {
	out := []QuotaWindow{}
	// Codex: primary (short) and secondary (weekly) windows.
	for _, w := range []string{"primary", "secondary"} {
		used, ok := h["x-codex-"+w+"-used-percent"]
		if !ok {
			continue
		}
		pct, _ := strconv.ParseFloat(used, 64)
		qw := QuotaWindow{Name: w, UsedPct: pct}
		if mins, err := strconv.Atoi(h["x-codex-"+w+"-window-minutes"]); err == nil && mins > 0 {
			qw.Name = windowName(mins)
		}
		if at, err := strconv.ParseInt(h["x-codex-"+w+"-reset-at"], 10, 64); err == nil && at > 0 {
			qw.ResetAt = time.Unix(at, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, qw)
	}
	// OpenAI-style request and token limits.
	for _, kind := range []string{"requests", "tokens"} {
		limit, remaining := h["x-ratelimit-limit-"+kind], h["x-ratelimit-remaining-"+kind]
		if limit == "" || remaining == "" {
			continue
		}
		l, _ := strconv.ParseFloat(limit, 64)
		rm, _ := strconv.ParseFloat(remaining, 64)
		qw := QuotaWindow{Name: kind, UsedPct: -1, Limit: limit, Used: strconv.FormatFloat(l-rm, 'f', -1, 64)}
		if l > 0 {
			qw.UsedPct = 100 * (l - rm) / l
		}
		if d := h["x-ratelimit-reset-"+kind]; d != "" {
			if dur, err := time.ParseDuration(d); err == nil {
				qw.ResetAt = time.Now().Add(dur).UTC().Format(time.RFC3339)
			}
		}
		out = append(out, qw)
	}
	// Copilot: x-quota-snapshot-<name>: ent=…&rem=…&ov=…&rst=… style values.
	for k, v := range h {
		name, ok := strings.CutPrefix(k, "x-quota-snapshot-")
		if !ok {
			continue
		}
		vals := map[string]string{}
		for _, part := range strings.Split(v, "&") {
			if kk, vv, ok := strings.Cut(part, "="); ok {
				vals[kk] = vv
			}
		}
		qw := QuotaWindow{Name: name, UsedPct: -1}
		if vals["ent"] == "-1" {
			qw.Unlimited = true
		}
		// ent is the entitlement (the window's total), rem the remaining count,
		// so used percent is (ent-rem)/ent. rem is a count, not a percent: on a
		// 500-unit window with 450 left, 100-rem would read -350.
		if rem, err := strconv.ParseFloat(vals["rem"], 64); err == nil && !qw.Unlimited {
			qw.Limit = vals["ent"]
			if ent, err := strconv.ParseFloat(vals["ent"], 64); err == nil && ent > 0 {
				qw.UsedPct = 100 * (ent - rem) / ent
				qw.Used = strconv.FormatFloat(ent-rem, 'f', -1, 64)
			}
		}
		if rst := vals["rst"]; rst != "" {
			qw.ResetAt = rst
		}
		out = append(out, qw)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// windowName names a window by its length: 300 minutes is "5h", 10080 "7d".
func windowName(mins int) string {
	switch {
	case mins%1440 == 0:
		return strconv.Itoa(mins/1440) + "d"
	case mins%60 == 0:
		return strconv.Itoa(mins/60) + "h"
	}
	return strconv.Itoa(mins) + "m"
}

// rateHeaders remembers the rate-limit and quota headers of the last answer
// each account got, so a provider without a quota endpoint (Groq, OpenRouter,
// NVIDIA…) still shows where it stands. Codex and Copilot also report their
// windows this way.
type rateHeaders struct {
	mu sync.Mutex
	m  map[string]rateSnapshot
}

func newRateHeaders() rateHeaders {
	return rateHeaders{m: map[string]rateSnapshot{}}
}

type rateSnapshot struct {
	At      time.Time         `json:"at"`
	Headers map[string]string `json:"headers"`
}

var ratePrefixes = []string{"x-ratelimit-", "ratelimit-", "anthropic-ratelimit-", "x-codex-", "x-quota-snapshot-", "retry-after"}

// capture keeps the quota headers of one answer, if it has any.
func (r *rateHeaders) capture(connID string, h http.Header) {
	got := map[string]string{}
	for k, v := range h {
		lk := strings.ToLower(k)
		for _, p := range ratePrefixes {
			if strings.HasPrefix(lk, p) && len(v) > 0 {
				got[lk] = v[0]
				break
			}
		}
	}
	if len(got) == 0 {
		return
	}
	r.mu.Lock()
	r.m[connID] = rateSnapshot{At: time.Now().UTC(), Headers: got}
	r.mu.Unlock()
}

func (r *rateHeaders) get(connID string) (rateSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[connID]
	return s, ok
}

// Quota endpoints, as the tools read them. Variables so a test can redirect.
var (
	codexUsageURL       = "https://chatgpt.com/backend-api/wham/usage"
	codexCreditsURL     = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	claudeUsageURL      = "https://api.anthropic.com/api/oauth/usage"
	copilotUserURL      = "https://api.github.com/copilot_internal/user"
	openrouterKeyURL    = "https://openrouter.ai/api/v1/key"
	groqModelsURL       = "https://api.groq.com/openai/v1/models"
	antigravityQuotaURL = "https://daily-cloudcode-pa.googleapis.com"
)

func init() {
	quotaFetchers["codex"] = quotaCodex
	quotaFetchers["claude"] = quotaClaude
	quotaFetchers["github"] = quotaCopilot
	quotaFetchers["antigravity"] = quotaAntigravity
	quotaFetchers["groq"] = quotaGroq
	quotaFetchers["openrouter"] = quotaOpenRouter
}

// fetchJSON sends one request and decodes a JSON answer; it returns the
// response headers too.
func fetchJSON(ctx context.Context, method, url string, headers map[string]string, body any, out any) (http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := upstream.Do(ctx, req, 1)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return resp.Header, fmt.Errorf("%d: %s", resp.StatusCode, msg)
	}
	if out != nil {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(out); err != nil {
			return resp.Header, fmt.Errorf("unreadable answer: %w", err)
		}
	}
	return resp.Header, nil
}

func jnum(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func jobj(v any) map[string]any { m, _ := v.(map[string]any); return m }

// resetTime reads a reset as ISO text or epoch seconds/milliseconds.
func resetTime(v any) string {
	if s, ok := v.(string); ok && s != "" {
		if _, err := time.Parse(time.RFC3339, s); err == nil {
			return s
		}
	}
	if f, ok := jnum(v); ok && f > 0 {
		if f > 1e12 {
			return time.UnixMilli(int64(f)).UTC().Format(time.RFC3339)
		}
		return time.Unix(int64(f), 0).UTC().Format(time.RFC3339)
	}
	return ""
}

func clampPct(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 100 {
		return 100
	}
	return f
}

// quotaCodex reads the ChatGPT plan's Codex windows.
func quotaCodex(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	var d map[string]any
	h := map[string]string{"Authorization": "Bearer " + token}
	if id := chatgptAccountID(token); id != "" {
		h["ChatGPT-Account-ID"] = id
	}
	u := codexUsageURL
	cu := codexCreditsURL
	if over, ok := a.baseOverride["codex"]; ok {
		u = over + "/backend-api/wham/usage"
		cu = over + "/backend-api/wham/rate-limit-reset-credits"
	}
	if _, err := fetchJSON(ctx, "GET", u, h, nil, &d); err != nil {
		return AccountQuota{}, err
	}
	q := AccountQuota{Windows: []QuotaWindow{}, Resets: []QuotaReset{}}
	if s, ok := d["plan_type"].(string); ok {
		q.Plan = s
	}
	window := func(prefix string, rl map[string]any) {
		if inner := jobj(rl["rate_limit"]); inner != nil {
			rl = inner
		}
		for _, k := range []string{"primary_window", "secondary_window"} {
			w := jobj(rl[k])
			if w == nil {
				w = jobj(rl[strings.TrimSuffix(k, "_window")])
			}
			if w == nil {
				continue
			}
			used, ok := jnum(w["used_percent"])
			if !ok {
				used, _ = jnum(w["percent_used"])
			}
			name := map[string]string{"primary_window": "session", "secondary_window": "weekly"}[k]
			if secs, ok := jnum(w["limit_window_seconds"]); ok && secs > 0 {
				name = windowName(int(secs / 60))
			}
			if prefix != "" {
				name = prefix + " " + name
			}
			reset := resetTime(w["reset_at"])
			if reset == "" {
				reset = resetTime(w["resets_at"])
			}
			q.Windows = append(q.Windows, QuotaWindow{Name: name, UsedPct: clampPct(used), ResetAt: reset})
		}
	}
	rl := jobj(d["rate_limit"])
	if rl == nil {
		rl = jobj(d["rate_limits"])
	}
	if rl == nil {
		rl = jobj(jobj(d["rate_limits_by_limit_id"])["codex"])
	}
	if rl != nil {
		window("", rl)
	}
	if rr := jobj(d["code_review_rate_limit"]); rr != nil {
		window("review", rr)
	}
	for _, x := range func() []any { l, _ := d["additional_rate_limits"].([]any); return l }() {
		m := jobj(x)
		name, _ := m["limit_name"].(string)
		if name == "" {
			name, _ = m["metered_feature"].(string)
		}
		if name != "" {
			window(name, m)
		}
	}

	availCount, _ := jnum(jobj(d["rate_limit_reset_credits"])["available_count"])
	if availCount > 0 {
		var cd map[string]any
		if _, err := fetchJSON(ctx, "GET", cu, h, nil, &cd); err != nil {
			q.ResetsError = err.Error()
		} else {
			var rollingWindows []string
			for _, w := range q.Windows {
				if !strings.Contains(w.Name, " ") {
					rollingWindows = append(rollingWindows, w.Name)
				}
			}
			q.Resets = codexResetRows(cd, time.Now().UTC(), rollingWindows)
		}
	}
	if q.Resets == nil {
		q.Resets = []QuotaReset{}
	}
	return q, nil
}

// quotaClaude reads the Claude plan's 5-hour and weekly utilization.
func quotaClaude(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	h := map[string]string{
		"Authorization":     "Bearer " + token,
		"Anthropic-Beta":    "oauth-2025-04-20",
		"Anthropic-Version": "2023-06-01",
	}
	if p, ok := provider.Lookup("claude"); ok {
		for k, v := range p.Identity {
			h[k] = v
		}
	}
	h["User-Agent"] = provider.ClaudeInteractiveUserAgent
	u := claudeUsageURL + "?at_wall=1&skip_spend=1"
	if over, ok := a.baseOverride["claude"]; ok {
		u = over + "/api/oauth/usage?at_wall=1&skip_spend=1"
	}
	var d map[string]any
	if _, err := fetchJSON(ctx, "GET", u, h, nil, &d); err != nil {
		return AccountQuota{}, err
	}
	q := AccountQuota{Windows: []QuotaWindow{}, Resets: []QuotaReset{}}
	add := func(name string, w map[string]any) {
		if w == nil {
			return
		}
		u, ok := jnum(w["utilization"])
		if !ok {
			return
		}
		q.Windows = append(q.Windows, QuotaWindow{Name: name, UsedPct: clampPct(u), ResetAt: resetTime(w["resets_at"])})
	}
	add("5h", jobj(d["five_hour"]))
	add("7d", jobj(d["seven_day"]))
	var keys []string
	for k := range d {
		if strings.HasPrefix(k, "seven_day_") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		add("7d "+strings.TrimPrefix(k, "seven_day_"), jobj(d[k]))
	}
	q.HasJuniperTide = jobj(d["juniper_tide"]) != nil
	q.HasCedarEmber = jobj(d["cedar_ember"]) != nil
	q.Resets = claudeResetRows(d, time.Now().UTC())
	if q.Resets == nil {
		q.Resets = []QuotaReset{}
	}
	return q, nil
}

// quotaCopilot reads the Copilot plan's monthly pools.
func quotaCopilot(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	var d map[string]any
	if _, err := fetchJSON(ctx, "GET", copilotUserURL, map[string]string{"Authorization": "token " + token,
		"X-Github-Api-Version": "2022-11-28", "User-Agent": "GitHubCopilotChat/" + provider.CopilotChatVersion,
		"Editor-Version": "vscode/" + provider.CopilotVSCodeVersion, "Editor-Plugin-Version": "copilot-chat/" + provider.CopilotChatVersion}, nil, &d); err != nil {
		return AccountQuota{}, err
	}
	q := AccountQuota{}
	q.Plan, _ = d["copilot_plan"].(string)
	reset := resetTime(d["quota_reset_date"])
	if reset == "" {
		if s, ok := d["quota_reset_date"].(string); ok {
			reset = s
		}
	}
	if snaps := jobj(d["quota_snapshots"]); snaps != nil {
		names := []string{}
		for k := range snaps {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			s := jobj(snaps[k])
			w := QuotaWindow{Name: k, UsedPct: -1, ResetAt: reset}
			if u, _ := s["unlimited"].(bool); u {
				w.Unlimited = true
			} else if ent, ok := jnum(s["entitlement"]); ok && ent > 0 {
				rem, _ := jnum(s["remaining"])
				w.UsedPct = clampPct(100 * (ent - rem) / ent)
				w.Used = strconv.FormatFloat(ent-rem, 'f', -1, 64)
				w.Limit = strconv.FormatFloat(ent, 'f', -1, 64)
			} else if pr, ok := jnum(s["percent_remaining"]); ok {
				w.UsedPct = clampPct(100 - pr)
			}
			q.Windows = append(q.Windows, w)
		}
		return q, nil
	}
	used, monthly := jobj(d["limited_user_quotas"]), jobj(d["monthly_quotas"])
	lreset := resetTime(d["limited_user_reset_date"])
	for _, k := range []string{"chat", "completions"} {
		total, ok := jnum(monthly[k])
		if !ok || total <= 0 {
			continue
		}
		rem, _ := jnum(used[k])
		// limited_user_quotas holds what is left of the month's pool.
		q.Windows = append(q.Windows, QuotaWindow{Name: k, UsedPct: clampPct(100 * (total - rem) / total),
			Used: strconv.FormatFloat(total-rem, 'f', -1, 64), Limit: strconv.FormatFloat(total, 'f', -1, 64), ResetAt: lreset})
	}
	if q.Plan == "" {
		q.Plan, _ = d["access_type_sku"].(string)
	}
	return q, nil
}

// quotaAntigravity reads the weekly buckets and each model's share.
func quotaAntigravity(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	project, err := a.antigravityProject(ctx, c, token)
	if err != nil {
		return AccountQuota{}, err
	}
	h := map[string]string{"Authorization": "Bearer " + token, "User-Agent": provider.AntigravityUserAgent,
		"X-Client-Name": "antigravity", "X-Client-Version": provider.AntigravityVersion}
	base := antigravityQuotaURL
	if over, ok := a.baseOverride["antigravity"]; ok {
		base = over
	}
	q := AccountQuota{}
	var sum map[string]any
	if _, err := fetchJSON(ctx, "POST", base+"/v1internal:retrieveUserQuotaSummary", h, map[string]any{"project": project}, &sum); err == nil {
		groups, _ := sum["groups"].([]any)
		if groups == nil {
			groups, _ = jobj(sum["quotaSummary"])["groups"].([]any)
		}
		for _, g := range groups {
			gm := jobj(g)
			gname, _ := gm["displayName"].(string)
			buckets, _ := gm["buckets"].([]any)
			for _, b := range buckets {
				bm := jobj(b)
				if dis, _ := bm["disabled"].(bool); dis {
					continue
				}
				name, _ := bm["displayName"].(string)
				if name == "" {
					name, _ = bm["bucketId"].(string)
				}
				name = shortBucket(gname) + " · " + shortBucket(name)
				frac, ok := jnum(bm["remainingFraction"])
				w := QuotaWindow{Name: name, UsedPct: -1, ResetAt: resetTime(bm["resetTime"])}
				if ok {
					w.UsedPct = clampPct(100 * (1 - frac))
				}
				q.Windows = append(q.Windows, w)
			}
		}
	}
	var models map[string]any
	if _, err := fetchJSON(ctx, "POST", base+"/v1internal:fetchAvailableModels", h, map[string]any{"project": project}, &models); err != nil {
		if len(q.Windows) == 0 {
			return AccountQuota{}, err
		}
		return q, nil
	}
	ms := jobj(models["models"])
	names := []string{}
	for k := range ms {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		m := jobj(ms[k])
		if in, _ := m["isInternal"].(bool); in {
			continue
		}
		qi := jobj(m["quotaInfo"])
		if qi == nil {
			continue
		}
		frac, ok := jnum(qi["remainingFraction"])
		if !ok {
			frac = 0
		}
		q.Windows = append(q.Windows, QuotaWindow{Name: "model " + k, UsedPct: clampPct(100 * (1 - frac)), ResetAt: resetTime(qi["resetTime"])})
	}
	return q, nil
}

// quotaGroq reads the request and token limits from the headers of a cheap
// call; Groq has no quota endpoint.
func quotaGroq(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	url := groqModelsURL
	if over, ok := a.baseOverride["groq"]; ok {
		url = over + "/models"
	}
	hdr, err := fetchJSON(ctx, "GET", url, map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	if err != nil {
		return AccountQuota{}, err
	}
	got := map[string]string{}
	for k, v := range hdr {
		if len(v) > 0 {
			got[strings.ToLower(k)] = v[0]
		}
	}
	return AccountQuota{Windows: windowsFromHeaders(got)}, nil
}

// quotaOpenRouter reads the key's credit use and limit.
func quotaOpenRouter(a *api, ctx context.Context, c store.Connection, token string) (AccountQuota, error) {
	var d struct {
		Data map[string]any `json:"data"`
	}
	if _, err := fetchJSON(ctx, "GET", openrouterKeyURL, map[string]string{"Authorization": "Bearer " + token}, nil, &d); err != nil {
		return AccountQuota{}, err
	}
	q := AccountQuota{}
	if free, _ := d.Data["is_free_tier"].(bool); free {
		q.Plan = "free tier"
	}
	usage, _ := jnum(d.Data["usage"])
	w := QuotaWindow{Name: "credits", UsedPct: -1, Used: "$" + strconv.FormatFloat(usage, 'f', 2, 64)}
	if limit, ok := jnum(d.Data["limit"]); ok && limit > 0 {
		w.Limit = "$" + strconv.FormatFloat(limit, 'f', 2, 64)
		w.UsedPct = clampPct(100 * usage / limit)
	} else {
		w.Unlimited = true
	}
	q.Windows = append(q.Windows, w)
	return q, nil
}

// shortBucket trims Antigravity's bucket wording: "Gemini Models" → "Gemini",
// "Weekly Limit Remaining" → "weekly", "Five Hour Limit Remaining" → "5h".
func shortBucket(s string) string {
	r := strings.NewReplacer(" Limit Remaining", "", " Limit", "", " Remaining", "", " Models", "", " models", "",
		"Five Hour", "5h", "Weekly", "weekly", "Daily", "daily")
	return strings.TrimSpace(r.Replace(s))
}
