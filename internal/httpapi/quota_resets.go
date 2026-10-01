package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
)

// QuotaReset is one reset that an account can spend.
type QuotaReset struct {
	ID          string   `json:"id"`   // "weekly", "grant:<id>", "credit:<id>"
	Kind        string   `json:"kind"` // weekly | grant | credit
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Clears      []string `json:"clears"`
	Left        int      `json:"left"`
	Total       int      `json:"total"`
	ValidFrom   string   `json:"validFrom,omitempty"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	NextAt      string   `json:"nextAt,omitempty"`
	Usable      bool     `json:"usable"`
	Blocked     string   `json:"blocked,omitempty"`
	Status      string   `json:"status,omitempty"`
}

var (
	claudeGrantIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,128}$`)
	codexCreditIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

func cutString(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

func mapClaudeClearName(name string) string {
	switch {
	case name == "five_hour":
		return "5h"
	case name == "seven_day":
		return "7d"
	case strings.HasPrefix(name, "seven_day_"):
		return "7d " + strings.TrimPrefix(name, "seven_day_")
	}
	return name
}

// claudeResetRows maps juniper_tide and cedar_ember blocks to QuotaReset rows.
func claudeResetRows(usage map[string]any, now time.Time) []QuotaReset {
	var rows []QuotaReset
	if jt := jobj(usage["juniper_tide"]); jt != nil {
		rows = append(rows, weeklyResetRow(jt))
	}
	if ce := jobj(usage["cedar_ember"]); ce != nil {
		rows = append(rows, grantResetRows(ce, now)...)
	}
	return rows
}

// weeklyResetRow maps the juniper_tide block to the weekly reset row.
func weeklyResetRow(jt map[string]any) QuotaReset {
	eligible := true
	if e, ok := jt["eligible"].(bool); ok && !e {
		eligible = false
	}
	ineligReason, _ := jt["ineligible_reason"].(string)
	available, _ := jt["available"].(bool)
	resetsPerWeek, _ := jnum(jt["resets_per_week"])
	nextAt, _ := jt["next_available_at"].(string)

	left := 0
	if available {
		left = 1
	}
	w := QuotaReset{
		ID:     "weekly",
		Kind:   "weekly",
		Title:  "5-hour limit reset",
		Clears: []string{"5h"},
		Left:   left,
		Total:  max(int(resetsPerWeek), 0),
		NextAt: nextAt,
	}
	switch {
	case !eligible:
		w.Blocked = "ineligible:" + ineligReason
	case !available && nextAt != "":
		w.Blocked = "used"
	case !available:
		w.Blocked = "not_at_limit"
	default:
		w.Usable = true
	}
	return w
}

// grantGate is the cedar_ember state that blocks every grant alike.
type grantGate struct {
	eligible bool
	reason   string
	cooldown bool
}

// grantResetRows maps the cedar_ember grants to reset rows. seenIDs spans the
// whole list, so a repeated id is caught across grants.
func grantResetRows(ce map[string]any, now time.Time) []QuotaReset {
	gate := grantGate{eligible: true}
	if e, ok := ce["eligible"].(bool); ok && !e {
		gate.eligible = false
	}
	gate.reason, _ = ce["ineligible_reason"].(string)
	if cu, ok := ce["cooldown_until"].(string); ok && cu != "" {
		if t, err := time.Parse(time.RFC3339, cu); err == nil && now.Before(t) {
			gate.cooldown = true
		}
	}

	var rows []QuotaReset
	rawGrants, _ := ce["grants"].([]any)
	seenIDs := map[string]bool{}
	for i, item := range rawGrants {
		// The position counts every raw item, objects or not.
		if gm := jobj(item); gm != nil {
			rows = append(rows, grantRow(gm, i+1, seenIDs, gate, now))
		}
	}
	return rows
}

// grantRow maps the grant at position pos and records its id in seenIDs. An
// empty or repeated id gets the row id bad:<pos>.
func grantRow(gm map[string]any, pos int, seenIDs map[string]bool, gate grantGate, now time.Time) QuotaReset {
	id, _ := gm["id"].(string)
	label, _ := gm["label"].(string)
	if label == "" {
		label = "Grant"
	}
	leftVal, _ := jnum(gm["resets_left"])
	totalVal, _ := jnum(gm["resets_total"])
	startsAt, _ := gm["starts_at"].(string)
	endsAt, _ := gm["ends_at"].(string)

	clears := []string{}
	if rawClears, ok := gm["clears"].([]any); ok {
		for _, c := range rawClears {
			if cs, ok := c.(string); ok {
				clears = append(clears, cutString(mapClaudeClearName(cs), 40))
			}
		}
	}

	rowID := "grant:" + id
	badID := false
	if id == "" || !claudeGrantIDPattern.MatchString(id) || seenIDs[id] {
		badID = true
		if id == "" || seenIDs[id] {
			rowID = fmt.Sprintf("bad:%d", pos)
		}
	}
	if id != "" {
		seenIDs[id] = true
	}

	r := QuotaReset{
		ID:        rowID,
		Kind:      "grant",
		Title:     cutString(label, 200),
		Clears:    clears,
		Left:      max(int(leftVal), 0),
		Total:     max(int(totalVal), 0),
		ValidFrom: startsAt,
		ExpiresAt: endsAt,
	}
	if badID {
		r.Blocked = "bad_id"
	} else {
		r.Blocked = grantBlockedReason(gm, gate, r.Left, now)
	}
	r.Usable = r.Blocked == ""
	return r
}

// grantBlockedReason applies Claude's blocked rules to a grant in their order,
// and returns "" for a usable grant.
func grantBlockedReason(gm map[string]any, gate grantGate, left int, now time.Time) string {
	startsAt, _ := gm["starts_at"].(string)
	endsAt, _ := gm["ends_at"].(string)
	paused, _ := gm["paused"].(bool)
	usableNow, _ := gm["usable_now"].(bool)

	var notStarted, isExpired bool
	if startsAt != "" {
		if t, err := time.Parse(time.RFC3339, startsAt); err == nil && now.Before(t) {
			notStarted = true
		}
	}
	if endsAt != "" {
		if t, err := time.Parse(time.RFC3339, endsAt); err == nil && !now.Before(t) {
			isExpired = true
		}
	}

	switch {
	case !gate.eligible:
		return "ineligible:" + gate.reason
	case gate.cooldown:
		return "cooldown"
	case paused:
		return "paused"
	case notStarted:
		return "not_started"
	case isExpired:
		return "expired"
	case left == 0:
		return "used"
	case !usableNow:
		return "not_at_limit"
	}
	return ""
}

// codexResetRows maps the Codex rate-limit-reset-credits response to QuotaReset rows.
func codexResetRows(credits map[string]any, now time.Time, clears ...[]string) []QuotaReset {
	var defaultClears []string
	if len(clears) > 0 && clears[0] != nil {
		for _, c := range clears[0] {
			defaultClears = append(defaultClears, cutString(c, 40))
		}
	}
	if defaultClears == nil {
		defaultClears = []string{"5h", "7d"}
	}

	rawList, _ := credits["credits"].([]any)
	var rows []QuotaReset
	seenIDs := map[string]bool{}

	for i, item := range rawList {
		cm := jobj(item)
		if cm == nil {
			continue
		}
		pos := i + 1
		id, _ := cm["id"].(string)
		status, _ := cm["status"].(string)
		title, _ := cm["title"].(string)
		if title == "" {
			title = "Full reset"
		}
		title = cutString(title, 200)

		desc, _ := cm["description"].(string)
		if desc == "" {
			desc = "Reset your current usage limits"
		}
		desc = cutString(desc, 1000)

		grantedAt, _ := cm["granted_at"].(string)
		expiresAt, _ := cm["expires_at"].(string)

		rowID := "credit:" + id
		badID := false
		if id == "" || !codexCreditIDPattern.MatchString(id) || seenIDs[id] {
			badID = true
			if id == "" || seenIDs[id] {
				rowID = fmt.Sprintf("bad:%d", pos)
			}
		}
		if id != "" {
			seenIDs[id] = true
		}

		r := QuotaReset{
			ID:          rowID,
			Kind:        "credit",
			Title:       title,
			Description: desc,
			Clears:      defaultClears,
			Left:        1,
			Total:       1,
			Status:      status,
			ValidFrom:   grantedAt,
			ExpiresAt:   expiresAt,
		}
		if status != "available" {
			r.Left = 0
		}

		if badID {
			r.Usable = false
			r.Blocked = "bad_id"
		} else {
			isExpired := false
			if expiresAt != "" {
				if t, err := time.Parse(time.RFC3339, expiresAt); err == nil && !now.Before(t) {
					isExpired = true
				}
			}

			if isExpired {
				r.Usable = false
				r.Blocked = "expired"
			} else {
				switch status {
				case "available":
					r.Usable = true
					r.Blocked = ""
				case "redeeming":
					r.Usable = false
					r.Blocked = "in_progress"
				case "redeemed":
					r.Usable = false
					r.Blocked = "used"
				default:
					r.Usable = false
					r.Blocked = "unknown_status"
				}
			}
		}

		rows = append(rows, r)
	}

	return rows
}

var (
	uuidRegex        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	claudeProfileURL = "https://api.anthropic.com/api/oauth/profile"
)

func isUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// claudeOrgID returns the Claude organization UUID for a connection, reading
// meta.claudeOrgId first, or fetching it from the profile endpoint.
func (a *api) claudeOrgID(ctx context.Context, c store.Connection, token string) (string, error) {
	conns, _ := a.store.ListConnections()
	for _, conn := range conns {
		if conn.ID == c.ID {
			if orgID, ok := conn.Meta["claudeOrgId"]; ok && orgID != "" && isUUID(orgID) {
				return orgID, nil
			}
			break
		}
	}

	u := claudeProfileURL
	if over, ok := a.baseOverride["claude"]; ok {
		u = over + "/api/oauth/profile"
	}
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

	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()

	var prof struct {
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
	}
	if _, err := fetchJSON(pctx, "GET", u, h, nil, &prof); err != nil {
		return "", err
	}

	uuid := prof.Organization.UUID
	if !isUUID(uuid) {
		return "", fmt.Errorf("profile organization.uuid %q is not a valid UUID", uuid)
	}

	if err := a.store.SetMeta(c.ID, map[string]string{"claudeOrgId": uuid}); err != nil {
		log.Printf("store claudeOrgId for %s: %v", c.ID, err)
	}
	return uuid, nil
}

// claimResult is the normalized outcome from a provider claim attempt.
type claimResult struct {
	Outcome      string // reset, not_needed, spent, not_allowed, failed, unknown, likely_spent
	ProviderCode string
	Message      string
}

// claimTables holds the unknown, hold, and done claim tables guarded by a single mutex.
type claimTables struct {
	mu      sync.Mutex
	unknown map[string]claimUnknownEntry // key: connID + ":" + resetId
	hold    map[string]claimHoldEntry    // key: connID + ":" + resetId
	done    map[string]claimDoneEntry    // key: connID + ":" + requestId
}

func newClaimTables() claimTables {
	return claimTables{unknown: map[string]claimUnknownEntry{}, hold: map[string]claimHoldEntry{}, done: map[string]claimDoneEntry{}}
}

type claimUnknownEntry struct {
	requestID string
	at        time.Time
	left      int
}

type claimHoldEntry struct {
	left int
	at   time.Time
}

type claimDoneEntry struct {
	outcome string
	at      time.Time
}

// resetClaimer spends one reset of a connection at its provider.
type resetClaimer func(ctx context.Context, a *api, c store.Connection, token string, r QuotaReset, requestID string) (claimResult, error)

var (
	claimRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	resetClaimers         = map[string]resetClaimer{}
)

func init() {
	resetClaimers["claude"] = claimClaude
	resetClaimers["codex"] = claimCodex
}

// claimClaude performs one claim against Anthropic's reset_rate_limits endpoint.
func claimClaude(ctx context.Context, a *api, c store.Connection, token string, r QuotaReset, requestID string) (claimResult, error) {
	orgID, err := a.claudeOrgID(ctx, c, token)
	if err != nil {
		pCode := ""
		if strings.HasPrefix(err.Error(), "401:") {
			pCode = "401"
		}
		return claimResult{Outcome: "failed", ProviderCode: pCode, Message: err.Error()}, nil
	}

	u := "https://api.anthropic.com/api/organizations/" + orgID + "/reset_rate_limits"
	if over, ok := a.baseOverride["claude"]; ok {
		u = over + "/api/organizations/" + orgID + "/reset_rate_limits"
	}

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

	var reqBody any
	if r.Kind == "weekly" {
		reqBody = map[string]any{"program": "juniper_tide"}
	} else if r.Kind == "grant" {
		grantID := strings.TrimPrefix(r.ID, "grant:")
		reqBody = map[string]any{
			"program":    "cedar_ember",
			"grant_id":   grantID,
			"request_id": requestID,
		}
	} else {
		return claimResult{Outcome: "failed", Message: "unknown reset kind"}, nil
	}

	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()

	var d map[string]any
	_, ferr := fetchJSON(cctx, "POST", u, h, reqBody, &d)
	if ferr != nil {
		msg := ferr.Error()
		if strings.HasPrefix(msg, "401:") {
			return claimResult{Outcome: "failed", ProviderCode: "401", Message: msg}, nil
		}
		if strings.HasPrefix(msg, "403:") {
			_ = a.store.SetMeta(c.ID, map[string]string{"claudeOrgId": ""})
			return claimResult{Outcome: "failed", ProviderCode: "403", Message: msg}, nil
		}
		if strings.HasPrefix(msg, "404:") {
			_ = a.store.SetMeta(c.ID, map[string]string{"claudeOrgId": ""})
			return claimResult{Outcome: "unknown", ProviderCode: "404", Message: msg}, nil
		}
		if strings.HasPrefix(msg, "429:") {
			return claimResult{Outcome: "failed", ProviderCode: "429", Message: msg}, nil
		}
		return claimResult{Outcome: "unknown", Message: msg}, nil
	}

	resStr, _ := d["result"].(string)
	reasonStr, _ := d["reason"].(string)

	switch resStr {
	case "reset":
		return claimResult{Outcome: "reset", ProviderCode: resStr, Message: reasonStr}, nil
	case "not_limited":
		return claimResult{Outcome: "not_needed", ProviderCode: resStr, Message: reasonStr}, nil
	case "already_used":
		return claimResult{Outcome: "spent", ProviderCode: resStr, Message: reasonStr}, nil
	case "ineligible", "cooldown":
		return claimResult{Outcome: "not_allowed", ProviderCode: resStr, Message: reasonStr}, nil
	default:
		return claimResult{Outcome: "unknown", ProviderCode: resStr, Message: reasonStr}, nil
	}
}

// claimCodex performs one claim against Codex rate-limit-reset-credits consume endpoint.
func claimCodex(ctx context.Context, a *api, c store.Connection, token string, r QuotaReset, requestID string) (claimResult, error) {
	u := "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
	if over, ok := a.baseOverride["codex"]; ok {
		u = over + "/backend-api/wham/rate-limit-reset-credits/consume"
	}

	h := map[string]string{"Authorization": "Bearer " + token}
	if id := chatgptAccountID(token); id != "" {
		h["ChatGPT-Account-ID"] = id
	}

	creditID := strings.TrimPrefix(r.ID, "credit:")
	reqBody := map[string]any{
		"redeem_request_id": requestID,
		"credit_id":         creditID,
	}

	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()

	var d map[string]any
	_, ferr := fetchJSON(cctx, "POST", u, h, reqBody, &d)
	if ferr != nil {
		msg := ferr.Error()
		if strings.HasPrefix(msg, "401:") {
			return claimResult{Outcome: "failed", ProviderCode: "401", Message: msg}, nil
		}
		if strings.HasPrefix(msg, "403:") {
			return claimResult{Outcome: "failed", ProviderCode: "403", Message: msg}, nil
		}
		if strings.HasPrefix(msg, "429:") {
			return claimResult{Outcome: "failed", ProviderCode: "429", Message: msg}, nil
		}
		return claimResult{Outcome: "unknown", Message: msg}, nil
	}

	code, _ := d["code"].(string)
	switch code {
	case "reset":
		return claimResult{Outcome: "reset", ProviderCode: code}, nil
	case "nothing_to_reset":
		return claimResult{Outcome: "not_needed", ProviderCode: code}, nil
	case "already_redeemed", "no_credit":
		return claimResult{Outcome: "spent", ProviderCode: code}, nil
	default:
		return claimResult{Outcome: "unknown", ProviderCode: code}, nil
	}
}

// claimRefusal is a claim answered before it reaches the provider: a plain
// error with a status, or a 409 with a body.
type claimRefusal struct {
	status   int
	msg      string
	conflict map[string]string
}

func refuseClaim(status int, msg string) *claimRefusal {
	return &claimRefusal{status: status, msg: msg}
}

func conflictClaim(body map[string]string) *claimRefusal {
	return &claimRefusal{status: http.StatusConflict, conflict: body}
}

func (e *claimRefusal) write(w http.ResponseWriter) {
	if e.conflict != nil {
		writeConflictError(w, e.conflict)
		return
	}
	writeError(w, e.status, e.msg)
}

// claimRequest is a claim that passed claimPreflight. resetKey and reqKey key
// the claim tables by the path's connection id.
type claimRequest struct {
	connID    string
	conn      store.Connection
	claimer   resetClaimer
	resetID   string
	requestID string
	resetKey  string
	reqKey    string
}

// claimReset handles POST /quota/{connectionId}/reset.
func (a *api) claimReset(w http.ResponseWriter, r *http.Request) {
	cr, refusal := a.claimPreflight(w, r)
	if refusal != nil {
		refusal.write(w)
		return
	}

	// One claim per connection at a time.
	l, ok := a.claimLocks.tryLock(cr.connID)
	if !ok {
		writeConflictError(w, map[string]string{"error": "claim_in_progress"})
		return
	}
	defer l.Unlock()

	ue, hasUnknown, refusal := a.checkClaimTables(cr, time.Now())
	if refusal != nil {
		refusal.write(w)
		return
	}
	target, refusal := a.claimTarget(r.Context(), cr)
	if refusal != nil {
		refusal.write(w)
		return
	}
	// A retry of an unknown claim, 60 seconds on, whose reset has gone down.
	if hasUnknown && ue.requestID == cr.requestID && likelySpent(target, ue) {
		a.recordLikelySpent(cr)
		writeJSON(w, map[string]any{"outcome": "likely_spent", "requestId": cr.requestID})
		return
	}
	if refusal := a.checkClaimHold(cr, target); refusal != nil {
		refusal.write(w)
		return
	}
	if refusal := unusableTarget(target); refusal != nil {
		refusal.write(w)
		return
	}

	log.Printf("claim sent: connection=%s resetId=%s requestId=%s", cr.connID, cr.resetID, cr.requestID)
	tctx, tcancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Second)
	defer tcancel()
	token, terr := a.secretFor(tctx, cr.connID)
	if terr != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot read connection token")
		return
	}
	claimRes, _ := cr.claimer(r.Context(), a, cr.conn, token, *target, cr.requestID)
	if claimRes.Outcome == "failed" && claimRes.ProviderCode == "401" {
		go a.refreshAfterClaim(cr.connID)
	}
	log.Printf("claim outcome: connection=%s resetId=%s requestId=%s outcome=%s providerCode=%s",
		cr.connID, cr.resetID, cr.requestID, claimRes.Outcome, claimRes.ProviderCode)

	a.recordClaimOutcome(cr, *target, claimRes)
	writeJSON(w, a.claimAnswer(r.Context(), cr, claimRes))
}

// claimPreflight checks the session, the fetch site, the content type, the
// connection and its claimer, and decodes the body of at most 4 KiB.
func (a *api) claimPreflight(w http.ResponseWriter, r *http.Request) (claimRequest, *claimRefusal) {
	// No-auth mode or no valid session cookie answers 403.
	if a.auth == nil {
		return claimRequest{}, refuseClaim(http.StatusForbidden, "claims need sign-in")
	}
	if ck, err := r.Cookie(sessionCookie); err != nil || !a.auth.ValidSession(ck.Value) {
		return claimRequest{}, refuseClaim(http.StatusForbidden, "claims need sign-in")
	}
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" {
		return claimRequest{}, refuseClaim(http.StatusForbidden, "cross-site request refused")
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		return claimRequest{}, refuseClaim(http.StatusBadRequest, "Content-Type must be application/json")
	}

	connID := r.PathValue("id")
	if connID == "" {
		connID = r.PathValue("connectionId")
	}
	if connID == "" {
		return claimRequest{}, refuseClaim(http.StatusNotFound, "unknown connection")
	}
	conn, err := a.connection(connID)
	if err != nil || !conn.IsActive {
		return claimRequest{}, refuseClaim(http.StatusNotFound, "unknown connection")
	}
	claimer, hasClaimer := resetClaimers[conn.Provider]
	if !hasClaimer {
		return claimRequest{}, refuseClaim(http.StatusNotFound, "provider has no claimer")
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var body struct {
		ResetID   string `json:"resetId"`
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return claimRequest{}, refuseClaim(http.StatusBadRequest, "bad json body: "+err.Error())
	}
	if len(body.ResetID) == 0 || len(body.ResetID) > 200 {
		return claimRequest{}, refuseClaim(http.StatusBadRequest, "resetId must be 1-200 characters")
	}
	if !claimRequestIDPattern.MatchString(body.RequestID) {
		return claimRequest{}, refuseClaim(http.StatusBadRequest, "invalid requestId format")
	}
	return claimRequest{
		connID: connID, conn: conn, claimer: claimer,
		resetID: body.ResetID, requestID: body.RequestID,
		resetKey: connID + ":" + body.ResetID, reqKey: connID + ":" + body.RequestID,
	}, nil
}

// checkClaimTables drops done entries older than 15 minutes, refuses a
// request id already done, and refuses while another unknown claim of the
// reset is pending or is younger than 60 seconds. It returns the unknown entry
// of the reset, if any, for the likely-spent check after the fresh read.
func (a *api) checkClaimTables(cr claimRequest, now time.Time) (claimUnknownEntry, bool, *claimRefusal) {
	a.claims.mu.Lock()
	defer a.claims.mu.Unlock()
	for k, de := range a.claims.done {
		if now.Sub(de.at) > 15*time.Minute {
			delete(a.claims.done, k)
		}
	}
	if de, ok := a.claims.done[cr.reqKey]; ok {
		return claimUnknownEntry{}, false, conflictClaim(map[string]string{"error": "already_done", "outcome": de.outcome})
	}
	ue, hasUnknown := a.claims.unknown[cr.resetKey]
	if hasUnknown && (ue.requestID != cr.requestID || now.Sub(ue.at) < 60*time.Second) {
		return ue, true, conflictClaim(map[string]string{"error": "unknown_pending", "requestId": ue.requestID})
	}
	return ue, hasUnknown, nil
}

// claimTarget reads the resets fresh from the provider and returns the row of
// the claimed reset, or nil when the read no longer lists it.
func (a *api) claimTarget(ctx context.Context, cr claimRequest) (*QuotaReset, *claimRefusal) {
	unreadable := refuseClaim(http.StatusServiceUnavailable, "cannot read resets now")
	fresh, ferr := a.freshQuota(ctx, cr.conn)
	if ferr != nil || fresh.ResetsError != "" {
		return nil, unreadable
	}
	if cr.conn.Provider == "claude" {
		if cr.resetID == "weekly" && !fresh.HasJuniperTide {
			return nil, unreadable
		}
		if strings.HasPrefix(cr.resetID, "grant:") && !fresh.HasCedarEmber {
			return nil, unreadable
		}
	}
	for i := range fresh.Resets {
		if fresh.Resets[i].ID == cr.resetID {
			return &fresh.Resets[i], nil
		}
	}
	return nil, nil
}

// likelySpent reports whether the fresh row shows that an unknown claim went
// through: the row is gone, or it has fewer resets left (for the weekly reset,
// also with a next reset time).
func likelySpent(target *QuotaReset, ue claimUnknownEntry) bool {
	if target == nil {
		return true
	}
	if target.Kind == "weekly" {
		return target.Left < ue.left && target.NextAt != ""
	}
	return target.Left < ue.left
}

func (a *api) recordLikelySpent(cr claimRequest) {
	a.claims.mu.Lock()
	delete(a.claims.unknown, cr.resetKey)
	a.claims.done[cr.reqKey] = claimDoneEntry{outcome: "likely_spent", at: time.Now()}
	a.claims.mu.Unlock()
}

// checkClaimHold refuses a claim right after a reset while the provider still
// reports the resets left before it. The hold ends when Left drops or after
// 60 seconds.
func (a *api) checkClaimHold(cr claimRequest, target *QuotaReset) *claimRefusal {
	a.claims.mu.Lock()
	defer a.claims.mu.Unlock()
	he, ok := a.claims.hold[cr.resetKey]
	if !ok {
		return nil
	}
	if (target == nil || target.Left >= he.left) && time.Since(he.at) < 60*time.Second {
		return conflictClaim(map[string]string{"error": "just_reset", "blocked": "just_reset"})
	}
	delete(a.claims.hold, cr.resetKey)
	return nil
}

// unusableTarget refuses a reset the fresh read does not list or blocks.
func unusableTarget(target *QuotaReset) *claimRefusal {
	if target == nil {
		return conflictClaim(map[string]string{"error": "not_found", "blocked": "not_found"})
	}
	if !target.Usable {
		return conflictClaim(map[string]string{"error": target.Blocked, "blocked": target.Blocked})
	}
	return nil
}

// refreshAfterClaim refreshes a token the provider refused with 401. It runs
// after the answer, so it has its own context.
func (a *api) refreshAfterClaim(connID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.forceRefresh(ctx, connID)
}

// recordClaimOutcome drops the cached quota and records the outcome in the
// claim tables. A failed claim never enters the done table and keeps any
// unknown entry.
func (a *api) recordClaimOutcome(cr claimRequest, target QuotaReset, res claimResult) {
	a.invalidateQuota(cr.connID)
	a.claims.mu.Lock()
	defer a.claims.mu.Unlock()
	switch res.Outcome {
	case "reset":
		delete(a.claims.unknown, cr.resetKey)
		a.claims.done[cr.reqKey] = claimDoneEntry{outcome: "reset", at: time.Now()}
		a.claims.hold[cr.resetKey] = claimHoldEntry{left: target.Left, at: time.Now()}
	case "spent", "not_needed", "not_allowed":
		delete(a.claims.unknown, cr.resetKey)
		a.claims.done[cr.reqKey] = claimDoneEntry{outcome: res.Outcome, at: time.Now()}
	case "unknown":
		a.claims.unknown[cr.resetKey] = claimUnknownEntry{requestID: cr.requestID, at: time.Now(), left: target.Left}
	}
}

// claimAnswer is the body of a claim that reached the provider. After a reset
// it carries the quota read again.
func (a *api) claimAnswer(ctx context.Context, cr claimRequest, res claimResult) map[string]any {
	body := map[string]any{
		"outcome":      res.Outcome,
		"providerCode": res.ProviderCode,
		"message":      res.Message,
		"requestId":    cr.requestID,
	}
	if res.Outcome == "reset" {
		if postFresh, err := a.freshQuota(ctx, cr.conn); err == nil {
			body["quota"] = postFresh
		}
	}
	return body
}

func writeConflictError(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	b, _ := json.Marshal(body)
	w.Write(b)
}
