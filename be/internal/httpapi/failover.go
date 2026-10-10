package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
)

// The failover loop: try every account in order, relay the first answer that is not a
// busy status, and remember what each refusal taught the next request.
//
// Four things are recorded on the way through, and each exists because the next
// request would otherwise repeat the same work. A 429 parks the account until the
// moment the provider named, so the next request skips it instead of paying a round
// trip for the same refusal. A 401 clears that park, because a refused credential is
// not a spent quota. A 429 on a long conversation shortens the history before the
// retry, because the account that answers next holds no cache for this prefix and the
// full history costs it in full for nothing. And a compaction that happened is
// reported to the caller, so a client can see that ccw shortened the conversation it
// sent rather than believe the model forgot.

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
	if model, _ := bodyModel(fc.body); model != "" {
		attempts = a.skipSpent(attempts, model)
	}
	// shortens holds what one compaction did, reported only if an answer reaches the
	// caller: a request that ends in every account failing never had an answer to
	// attach it to.
	var shortens *shortening
	for i, at := range attempts {
		acc, ok := a.resolveAttempt(fc, at)
		if !ok {
			continue
		}
		call, err := a.buildUpstreamCall(fc, acc)
		if err != nil {
			var skip errSkipAccount
			if errors.As(err, &skip) {
				a.logFor(fc.r.Context()).Warn("proxy.account.skip", "connection", acc.conn.ID, "err", skip.err)
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
			a.logFor(fc.r.Context()).Warn("proxy.upstream.busy", "connection", acc.conn.ID,
				"provider", acc.conn.Provider, "status", resp.StatusCode, "model", at.model)
		}
		a.shortenForRateLimit(fc, acc, at, resp, attempts, i, &shortens)
		if retryableStatus(resp.StatusCode) && i < len(attempts)-1 {
			resp.Body.Close()
			continue
		}
		a.reportShortening(fc, w, shortens)
		a.relayAnswer(fc, acc, call, resp, injected)
		return
	}
	if tried == 0 {
		writeError(w, http.StatusInternalServerError, "no usable account")
		return
	}
	writeError(w, http.StatusBadGateway, "all accounts failed")
}

// shortening is one compaction of the request body, and the account that caused it.
type shortening struct {
	stat   compactStat
	connID string
}

// shortenForRateLimit carries the history shortened when a 429 has an account behind
// it to retry on.
//
// Once per request. A second 429 says the limit is the account's, not the
// conversation's length, and shortening again would only throw away more. The park
// itself is set in sendWithRecovery, once every retry one account can do has run out.
func (a *api) shortenForRateLimit(fc *failoverCall, acc accountTry, at attempt, resp *http.Response, attempts []attempt, i int, done **shortening) {
	if resp.StatusCode != http.StatusTooManyRequests || *done != nil || i >= len(attempts)-1 {
		return
	}
	next := attempts[i+1]
	// A standby is the last resort for a provider with nothing else; a shortened
	// conversation is not worth handing it.
	if next.conn.ID == acc.conn.ID || next.conn.Standby {
		return
	}
	rest := make([]store.Connection, 0, len(attempts)-i-1)
	for _, nextAt := range attempts[i+1:] {
		rest = append(rest, nextAt.conn)
	}
	if shorter, st, ok := a.compactForRateLimit(acc.conn.Provider, rest, fc.body); ok {
		fc.body = shorter
		// The translations were built from the body this replaced.
		clear(fc.translated)
		*done = &shortening{stat: st, connID: acc.conn.ID}
	}
}

// reportShortening tells the caller how much of its history ccw dropped, so a client
// that measures its own prompt can tell a shortened conversation from a forgetful
// model.
func (a *api) reportShortening(fc *failoverCall, w http.ResponseWriter, s *shortening) {
	if s == nil {
		return
	}
	w.Header().Set("X-Ccw-Compacted-Bytes", strconv.Itoa(s.stat.saved()))
	a.logFor(fc.r.Context()).Info("proxy.history.shortened", "connection", s.connID,
		"before", s.stat.before, "after", s.stat.after,
		"kept_head", s.stat.keptHead, "kept_middle", s.stat.keptMiddle,
		"dropped_middle", s.stat.droppedMiddle, "dropped_tool_only", s.stat.droppedToolOnly)
}
