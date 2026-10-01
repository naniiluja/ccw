// Package httpapi serves the local API.
package httpapi

import (
	"compress/gzip"
	"compress/zlib"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/drift"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
)

const sessionCookie = "ccw_session"

type api struct {
	store *store.Store
	// baseOverride redirects a provider to a test server. Nil in production.
	baseOverride map[string]string
	// auth gates the server. Nil means no gate (loopback use).
	auth *auth.Config
	// rrMu guards rrNext, the rotation cursors: per provider ("p:"), or per
	// model for a pool across providers ("m:").
	rrMu   sync.Mutex
	rrNext map[string]rrCursor
	// keyLim counts each API key's requests against its per-minute cap.
	keyLim keyLimiter
	// cat caches each provider's model list for resolving a bare model id.
	cat catalog
	// filters caches the compiled request filters.
	filters filterCache
	// copilot caches exchanged Copilot tokens per connection.
	copilot copilotCache
	// sigs remembers Gemini thought signatures by function call id.
	sigs sigStore
	// drift learns the structure of what passes through and records changes.
	drift *drift.Observer
	// rate keeps the last rate-limit headers per account.
	rate rateHeaders
	// quota caches the quota read from each account.
	quota quotaCache
	// auto tracks the providers whose models are being auto-tested.
	auto autoState
	// review is the drift review.
	review reviewState
	// errReview is the error review's state.
	errReview reviewState
	// login caps failed sign-in attempts so the password cannot be brute forced.
	login *auth.LoginGuard
	// refresh serializes OAuth token refreshes per connection.
	refresh refreshLocks
	// claimLocks serializes manual quota reset claims per connection.
	claimLocks refreshLocks
	// claims tracks unknown, hold, and done claim states across connections.
	claims claimTables
	// zen holds the session pool and catalogue of the OpenCode Zen provider.
	zen zenState
	// web is the search service for a client that asks its model to search.
	web webState
}

// New builds the route table with no authentication (loopback use and tests).
func New(s *store.Store, baseOverride map[string]string) http.Handler {
	return NewWithAuth(s, baseOverride, nil)
}

// NewWithAuth builds the route table. When authCfg is not nil, a person must log
// in with the password to reach the dashboard, and a machine must present the
// master token or a dashboard key. Only the session and the master token are admin.
func NewWithAuth(s *store.Store, baseOverride map[string]string, authCfg *auth.Config) http.Handler {
	_, h := newServer(s, baseOverride, authCfg)
	return h
}

// newServer builds the api and its routes; tests reach the api through it.
func newServer(s *store.Store, baseOverride map[string]string, authCfg *auth.Config) (*api, http.Handler) {
	a := &api{
		store:        s,
		baseOverride: baseOverride,
		auth:         authCfg,
		rrNext:       map[string]rrCursor{},
		keyLim:       newKeyLimiter(),
		cat:          newCatalog(),
		copilot:      newCopilotCache(),
		sigs:         newSigStore(),
		drift:        drift.New(s),
		rate:         newRateHeaders(),
		quota:        newQuotaCache(),
		auto:         newAutoState(),
		login:        auth.NewLoginGuard(),
		claims:       newClaimTables(),
		zen:          newZenState(),
	}
	go a.autoTestLoop()
	a.loadDefs()
	a.migrateCustomEndpoints()
	go a.reviewLoop()
	go a.errorReviewLoop()
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	return a, a.guardRequest(collapseV1(mux))
}

// newCatalog lives here, not next to its type in v1.go, which this change
// leaves alone.
func newCatalog() catalog {
	return catalog{m: map[string]catalogEntry{}, inflight: map[string]*catalogFetch{}}
}

// registerRoutes adds every route to mux. Each route names its auth tier:
// requireSession for the dashboard, requireToken for reads by a machine, and
// requireAdmin for every write through /api.
func (a *api) registerRoutes(mux *http.ServeMux) {
	// One base URL: the model in the body picks the provider and its accounts.
	// The contract of /v1 is public, like the API it describes.
	mux.HandleFunc("GET /openapi.json", serveOpenAPI)
	mux.HandleFunc("GET /v1/models", v1API(a.requireToken(a.models)))
	mux.HandleFunc("GET /v1/models/{model...}", v1API(a.requireToken(a.model)))
	mux.HandleFunc("/v1/{path...}", v1API(a.requireToken(a.v1)))
	// Management API for machines: the same token as /v1.
	mux.HandleFunc("GET /api/providers", a.requireToken(a.apiProviders))
	mux.HandleFunc("GET /api/accounts", a.requireToken(a.accounts))
	mux.HandleFunc("POST /api/accounts/{id}/active", a.requireAdmin(a.setActive))
	mux.HandleFunc("GET /api/zen/sessions", a.requireAdmin(a.zenSessions))
	mux.HandleFunc("GET /api/usage", a.requireToken(a.usage))
	mux.HandleFunc("GET /api/quota", a.requireToken(a.quotaList(false)))
	mux.HandleFunc("GET /quota", a.requireSession(a.quotaList(true)))
	mux.HandleFunc("POST /quota/{id}/reset", a.requireSession(a.claimReset))
	mux.HandleFunc("GET /ui-settings/{key}", a.requireSession(a.getUISetting))
	mux.HandleFunc("POST /ui-settings/{key}", a.requireSession(a.setUISetting))
	mux.HandleFunc("GET /api/drift/changes", a.requireToken(a.driftChanges))
	mux.HandleFunc("POST /api/drift/ack", a.requireAdmin(a.driftAck))
	mux.HandleFunc("GET /api/drift/fields", a.requireToken(a.driftFields))
	mux.HandleFunc("POST /api/drift/seed", a.requireAdmin(a.driftSeed))
	for _, pre := range []string{"", "/api"} {
		// read: the dashboard session, or any machine token. write: the session
		// or the master token only, so a shared inference key cannot trigger a
		// review.
		read, write := a.requireSession, a.requireSession
		if pre != "" {
			read, write = a.requireToken, a.requireAdmin
		}
		mux.HandleFunc("GET "+pre+"/errors/review", read(a.errorReview))
		mux.HandleFunc("POST "+pre+"/errors/review", write(a.errorReview))
		mux.HandleFunc("GET "+pre+"/errors/verdicts", read(a.errorVerdicts))
	}
	mux.HandleFunc("GET /errors", a.requireSession(a.errorsList))
	mux.HandleFunc("GET /errors/stats", a.requireSession(a.errorStats))
	mux.HandleFunc("GET /errors/{id}", a.requireSession(a.errorGet))
	mux.HandleFunc("GET /api/errors", a.requireToken(a.errorsList))
	mux.HandleFunc("GET /api/errors/stats", a.requireToken(a.errorStats))
	mux.HandleFunc("GET /api/errors/{id}", a.requireToken(a.errorGet))
	mux.HandleFunc("GET /drift/changes", a.requireSession(a.driftChanges))
	mux.HandleFunc("GET /drift/review", a.requireSession(a.driftReview))
	mux.HandleFunc("POST /drift/review", a.requireSession(a.driftReview))
	mux.HandleFunc("GET /api/drift/review", a.requireToken(a.driftReview))
	mux.HandleFunc("POST /api/drift/review", a.requireAdmin(a.driftReview))
	mux.HandleFunc("POST /drift/ack", a.requireSession(a.driftAck))
	mux.HandleFunc("GET /drift/fields", a.requireSession(a.driftFields))
	mux.HandleFunc("GET /api/filters", a.requireToken(a.listFilters))
	mux.HandleFunc("POST /api/filters", a.requireAdmin(a.saveFilter))
	mux.HandleFunc("DELETE /api/filters/{id}", a.requireAdmin(a.deleteFilter))
	mux.HandleFunc("POST /api/filters/{id}/delete", a.requireAdmin(a.deleteFilter))
	// MCP (Streamable HTTP): the management API as tools for an agent.
	mux.HandleFunc("POST /mcp", a.requireToken(a.mcp))
	mux.HandleFunc("GET /mcp", a.requireToken(a.mcpGet))
	mux.HandleFunc("GET /accounts", a.requireSession(a.accounts))
	mux.HandleFunc("POST /accounts", a.requireSession(a.createAccount))
	mux.HandleFunc("POST /accounts/{id}/delete", a.requireSession(a.deleteAccount))
	mux.HandleFunc("GET /accounts/{id}/models", a.requireSession(a.modelsForAccount))
	mux.HandleFunc("POST /accounts/{id}/active", a.requireSession(a.setActive))
	mux.HandleFunc("POST /accounts/{id}/label", a.requireSession(a.setLabel))
	mux.HandleFunc("POST /accounts/{id}/test", a.requireSession(a.testAccount))
	mux.HandleFunc("GET /account-tests", a.requireSession(a.accountTests))
	mux.HandleFunc("POST /api/accounts/{id}/test", a.requireAdmin(a.testAccount))
	mux.HandleFunc("GET /providers", a.requireSession(a.providers))
	mux.HandleFunc("GET /providers/{id}/models", a.requireSession(a.providerModelList))
	mux.HandleFunc("GET /providers/{id}/model-table", a.requireSession(a.providerModelTable))
	mux.HandleFunc("POST /providers/{id}/models/active", a.requireSession(a.setModelsActive))
	mux.HandleFunc("POST /providers/{id}/models/delete", a.requireSession(a.deleteModels))
	mux.HandleFunc("POST /providers/{id}/models/test", a.requireSession(a.testModel))
	mux.HandleFunc("GET /providers/{id}/rotation", a.requireSession(a.getRotation))
	mux.HandleFunc("POST /providers/{id}/rotation", a.requireSession(a.setRotation))
	mux.HandleFunc("GET /api/providers/{id}/rotation", a.requireToken(a.getRotation))
	mux.HandleFunc("POST /api/providers/{id}/rotation", a.requireAdmin(a.setRotation))
	mux.HandleFunc("GET /providers/{id}/model-policy", a.requireSession(a.getModelPolicy))
	mux.HandleFunc("POST /providers/{id}/model-policy", a.requireSession(a.setModelPolicy))
	mux.HandleFunc("GET /api/providers/{id}/model-policy", a.requireToken(a.getModelPolicy))
	mux.HandleFunc("POST /api/providers/{id}/model-policy", a.requireAdmin(a.setModelPolicy))
	mux.HandleFunc("GET /api/providers/{id}/models", a.requireToken(a.providerModelTable))
	mux.HandleFunc("GET /api/providers/{id}/models/raw", a.requireToken(a.providerModelsRaw))
	mux.HandleFunc("GET /api/provider-defs", a.requireToken(a.listDefs))
	mux.HandleFunc("GET /api/provider-defs/{id}", a.requireToken(a.getDef))
	mux.HandleFunc("POST /api/provider-defs", a.requireAdmin(a.putDef))
	mux.HandleFunc("DELETE /api/provider-defs/{id}", a.requireAdmin(a.deleteDef))
	mux.HandleFunc("GET /provider-defs", a.requireSession(a.listDefs))
	mux.HandleFunc("GET /provider-defs/{id}", a.requireSession(a.getDef))
	mux.HandleFunc("POST /provider-defs", a.requireSession(a.putDef))
	mux.HandleFunc("POST /provider-defs/{id}/delete", a.requireSession(a.deleteDef))
	mux.HandleFunc("POST /api/providers/{id}/models/active", a.requireAdmin(a.setModelsActive))
	mux.HandleFunc("POST /api/providers/{id}/models/delete", a.requireAdmin(a.deleteModels))
	mux.HandleFunc("POST /api/providers/{id}/models/test", a.requireAdmin(a.testModel))
	mux.HandleFunc("POST /oauth/{provider}/start", a.requireSession(a.loginStart))
	mux.HandleFunc("POST /oauth/{provider}/finish", a.requireSession(a.loginFinish))
	mux.HandleFunc("POST /oauth/github/poll", a.requireSession(a.githubDevicePoll))
	mux.HandleFunc("POST /oauth/{provider}/poll", a.requireSession(a.devicePoll))
	mux.HandleFunc("GET /keys", a.requireSession(a.listKeys))
	mux.HandleFunc("POST /keys", a.requireSession(a.createKey))
	mux.HandleFunc("POST /keys/{id}/reveal", a.requireSession(a.revealKey))
	mux.HandleFunc("POST /keys/{id}/active", a.requireSession(a.setKeyActive))
	mux.HandleFunc("POST /keys/{id}/models", a.requireSession(a.setKeyModels))
	mux.HandleFunc("POST /keys/{id}/limits", a.requireSession(a.setKeyLimits))
	mux.HandleFunc("GET /keys/{id}/usage", a.requireSession(a.keyUsage))
	mux.HandleFunc("POST /keys/{id}/delete", a.requireSession(a.deleteKey))
	mux.HandleFunc("GET /filters", a.requireSession(a.listFilters))
	mux.HandleFunc("POST /filters", a.requireSession(a.saveFilter))
	mux.HandleFunc("POST /filters/{id}/delete", a.requireSession(a.deleteFilter))
	mux.HandleFunc("GET /usage", a.requireSession(a.usage))
	mux.HandleFunc("POST /login", a.loginSubmit)
	mux.HandleFunc("POST /logout", a.logout)
}

// collapseV1 routes /v1/v1/… (any number of repeats) as /v1/…. The base URL
// ccw shows ends in /v1, and Anthropic clients add /v1 themselves.
func collapseV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := collapseV1Path(r.URL.Path); p != r.URL.Path {
			u := *r.URL
			u.Path, u.RawPath = p, ""
			r2 := r.Clone(r.Context())
			r2.URL = &u
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}

// collapseV1Path removes repeats of a leading /v1 segment.
func collapseV1Path(p string) string {
	for strings.HasPrefix(p, "/v1/v1/") || p == "/v1/v1" {
		p = p[len("/v1"):]
	}
	return p
}

// requireSession lets a request through when auth is off or the session cookie
// is valid; otherwise it redirects a browser to the login page.
func (a *api) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.auth == nil {
			next(w, withPrincipal(r, principal{admin: true}))
			return
		}
		if ck, err := r.Cookie(sessionCookie); err == nil && a.auth.ValidSession(ck.Value) {
			next(w, withPrincipal(r, principal{admin: true}))
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// requireToken lets a request through when auth is off or it carries a valid
// API token: the one in the environment, or an enabled key made in the
// dashboard. The token is read from Authorization: Bearer, or from x-api-key,
// which Anthropic clients send. Otherwise it answers 401 for the machine caller.
func (a *api) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearerToken(r)
		// Who the caller is travels with the request: the master token is
		// admin, and a dashboard key owns only what its key id caused. The key
		// name is a label for drift and the error log, never a right.
		if tok != "" && a.auth != nil && a.auth.CheckAPIToken("Bearer "+tok) {
			next(w, withPrincipal(r, principal{admin: true, name: "env"}))
			return
		}
		if k, ok := a.store.APIKeyByToken(tok); ok {
			if k.Expired(time.Now()) {
				writeError(w, http.StatusUnauthorized, "api key expired")
				return
			}
			if k.RPM > 0 {
				if ok, wait := a.keyLim.allow(k.ID, k.RPM, time.Now()); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
					writeError(w, http.StatusTooManyRequests, "api key rate limit: "+strconv.Itoa(k.RPM)+" requests per minute")
					return
				}
			}
			next(w, withPrincipal(r, principal{keyID: k.ID, name: k.Name, models: k.Models}))
			return
		}
		if ck, err := r.Cookie(sessionCookie); err == nil && a.auth != nil && a.auth.ValidSession(ck.Value) {
			next(w, withPrincipal(r, principal{admin: true, name: "session"}))
			return
		}
		if a.auth == nil {
			next(w, withPrincipal(r, principal{admin: true}))
			return
		}
		writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
	}
}

// requireAdmin gates management that can redirect a stored credential or wipe a
// request: a valid session, or the environment's master token. A key made in
// the dashboard (handed to a machine for /v1) is refused here, so such a key
// cannot change a provider's URL, install a body filter, or read another
// client's stored data.
func (a *api) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.auth == nil {
			next(w, withPrincipal(r, principal{admin: true}))
			return
		}
		if ck, err := r.Cookie(sessionCookie); err == nil && a.auth.ValidSession(ck.Value) {
			next(w, withPrincipal(r, principal{admin: true}))
			return
		}
		if tok := bearerToken(r); tok != "" && a.auth.CheckAPIToken("Bearer "+tok) {
			next(w, withPrincipal(r, principal{admin: true, name: "env"}))
			return
		}
		writeError(w, http.StatusForbidden, "this action needs the dashboard session or the master token")
	}
}

// bearerToken reads the caller's token from Authorization: Bearer, or from
// x-api-key, which Anthropic clients send.
func bearerToken(r *http.Request) string {
	tok := r.Header.Get("X-Api-Key")
	if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		tok = b
	}
	return tok
}

// openAPIDoc describes the /v1 surface. Keep it in step with the routes in
// newServer and the envelopes in writeAPIError; TestOpenAPIDocNamesEveryRoute
// fails when a /v1 route is missing from it.
//
//go:embed openapi.json
var openAPIDoc []byte

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPIDoc)
}

// apiWriter marks a /v1 response with the shape its caller reads, so every
// error ccw raises there leaves in that API's own envelope. Dashboard and
// management routes keep {"error": "<message>"}.
type apiWriter struct {
	http.ResponseWriter
	shape string
}

func (w apiWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// requestIDHeaders carry one id per /v1 call. An upstream's own id replaces it
// when the answer is relayed, so a provider ticket can still quote it.
var requestIDHeaders = []string{"Request-Id", "X-Request-Id"}

// v1API wraps a /v1 handler: the error shape by path, a request id, and an
// answer to a CORS preflight, which /v1 does not serve.
func v1API(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shape := translate.OpenAI
		if isAnthropicPath(r.URL.Path) {
			shape = translate.Anthropic
		}
		b := make([]byte, 12)
		rand.Read(b)
		id := "req_" + hex.EncodeToString(b)
		for _, h := range requestIDHeaders {
			w.Header().Set(h, id)
		}
		aw := apiWriter{ResponseWriter: w, shape: shape}
		if r.Method == http.MethodOptions {
			writeError(aw, http.StatusMethodNotAllowed, "CORS preflight is not served: call /v1 from a server, not a browser")
			return
		}
		next(aw, r)
	}
}

// isAnthropicPath covers the Messages family, count_tokens included. The
// server has already collapsed a doubled /v1.
func isAnthropicPath(p string) bool {
	return p == "/v1/messages" || strings.HasPrefix(p, "/v1/messages/")
}

// writeAPIError answers with an error in the caller's envelope. code and param
// are OpenAI's optional fields; the Anthropic envelope has no such fields.
func writeAPIError(w http.ResponseWriter, status int, code, param, msg string) {
	aw, ok := findAPIWriter(w)
	if !ok {
		writeLegacyError(w, status, msg)
		return
	}
	var body any
	if aw.shape == translate.Anthropic {
		body = map[string]any{"type": "error", "error": map[string]any{"type": translate.AnthropicErrorType(status, ""), "message": msg}}
	} else {
		typ := "invalid_request_error"
		switch {
		case status == http.StatusTooManyRequests:
			typ = "requests"
			if code == "" {
				code = "rate_limit_exceeded"
			}
		case status == http.StatusUnauthorized:
			if code == "" {
				code = "invalid_api_key"
			}
		case status >= 500:
			typ = "server_error"
		}
		e := map[string]any{"message": msg, "type": typ, "param": nil, "code": nil}
		if code != "" {
			e["code"] = code
		}
		if param != "" {
			e["param"] = param
		}
		body = map[string]any{"error": e}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(body)
	w.Write(b)
}

// findAPIWriter looks through writers that wrap the /v1 one.
func findAPIWriter(w http.ResponseWriter) (apiWriter, bool) {
	for {
		if aw, ok := w.(apiWriter); ok {
			return aw, true
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return apiWriter{}, false
		}
		w = u.Unwrap()
	}
}

// maxV1Body bounds a /v1 request, before and after it is decompressed. A call
// carries whole conversations and screenshots, so the bound is generous: it is
// there to keep one request from filling the memory, not to shape a prompt.
var maxV1Body int64 = 50 << 20

// readBody reads a /v1 request body and undoes a gzip or deflate Content-Encoding,
// so everything after it sees the JSON. It reports the status to answer with when
// the body cannot be used.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	raw := http.MaxBytesReader(w, r.Body, maxV1Body)
	defer r.Body.Close()
	var src io.Reader = raw
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(raw)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("cannot read a gzip body")
		}
		defer zr.Close()
		src = zr
	case "deflate":
		zr, err := zlib.NewReader(raw)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("cannot read a deflate body")
		}
		defer zr.Close()
		src = zr
	default:
		return nil, http.StatusUnsupportedMediaType, fmt.Errorf("Content-Encoding %q is not supported; send the body as plain JSON, gzip or deflate", enc)
	}
	// The limit applies to what the body becomes as well: a small compressed
	// body can inflate to far more than it is.
	body, err := io.ReadAll(io.LimitReader(src, maxV1Body+1))
	if err == nil && int64(len(body)) > maxV1Body {
		err = &http.MaxBytesError{Limit: maxV1Body}
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("the request is larger than %d MB", maxV1Body>>20)
		}
		return nil, http.StatusBadRequest, errors.New("cannot read body")
	}
	// What goes upstream is the decoded JSON, so the header that described the
	// wire form must not follow it.
	r.Header.Del("Content-Encoding")
	return body, 0, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// writeError replies with a JSON body that never names a credential; on /v1
// in the caller's API envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeAPIError(w, status, "", "", msg)
}

func writeLegacyError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(b)
}
