package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
)

// modelTestKey marks a test call made from the dashboard, which may reach a
// model the operator switched off (to decide whether to switch it on). Its
// value is a *testRun.
type modelTestKey struct{}

// testRun pins a test to one account (pin, when set) and learns which account
// answered it (used).
type testRun struct {
	pin  string
	used string
}

// providerModelTable serves a provider's models with their switch and last
// test, and the provider's policy. It fetches the list first when the cache is stale.
func (a *api) providerModelTable(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	if r.URL.Query().Get("refresh") == "1" {
		a.refreshCatalog(r.Context(), prov)
	} else {
		a.catalogIDs(r.Context(), prov)
	}
	list, err := a.store.ListModels(prov)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read models")
		return
	}
	a.cat.mu.Lock()
	e := a.cat.m[prov]
	a.cat.mu.Unlock()
	variants := map[string][]string{}
	for base, g := range e.groups {
		variants[base] = g.Variants()
	}
	writeJSON(w, map[string]any{"models": list, "ok": e.ok, "fetchedAt": e.at.UTC().Format(time.RFC3339), "variants": variants, "info": e.info,
		"policy": a.modelPolicy(prov), "running": a.auto.isRunning(prov)})
}

type modelsBody struct {
	Models []string `json:"models"`
	Active bool     `json:"active"`
	Model  string   `json:"model"`
	// Account pins a test to one connection id.
	Account string `json:"account"`
}

func readModels(w http.ResponseWriter, r *http.Request) (modelsBody, bool) {
	var b modelsBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return b, false
	}
	return b, true
}

// setModelsActive switches models on or off: {"models":[…],"active":bool}.
func (a *api) setModelsActive(w http.ResponseWriter, r *http.Request) {
	b, ok := readModels(w, r)
	if !ok {
		return
	}
	n, err := a.store.SetModelsActive(r.PathValue("id"), b.Models, b.Active)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot update models")
		return
	}
	writeJSON(w, map[string]any{"updated": n})
}

// deleteModels forgets models: {"models":[…]}.
func (a *api) deleteModels(w http.ResponseWriter, r *http.Request) {
	b, ok := readModels(w, r)
	if !ok {
		return
	}
	n, err := a.store.DeleteModels(r.PathValue("id"), b.Models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot delete models")
		return
	}
	writeJSON(w, map[string]any{"deleted": n})
}

// testModel sends a short chat request to one model through the same /v1
// path a client uses (translation and failover included) and records the
// outcome: {"model":"…"}.
func (a *api) testModel(w http.ResponseWriter, r *http.Request) {
	b, ok := readModels(w, r)
	if !ok || b.Model == "" {
		if ok {
			writeError(w, http.StatusBadRequest, "model is required")
		}
		return
	}
	prov := r.PathValue("id")
	res := a.runModelTest(r.Context(), prov, b.Model, b.Account)
	a.store.RecordModelTest(prov, b.Model, res.OK, res.Ms, res.Message, res.Account)
	// Under auto test, a test decides the switch whoever runs it.
	if pol := a.modelPolicy(prov); pol.AutoTest {
		on := res.OK && pol.wanted(b.Model)
		a.store.SetModelsActive(prov, []string{b.Model}, on)
		res.Active = &on
	}
	writeJSON(w, res)
}

type modelTest struct {
	OK      bool   `json:"ok"`
	Status  int    `json:"status"`
	Ms      int64  `json:"ms"`
	Message string `json:"message"`
	// Account is the connection id that answered.
	Account string `json:"account,omitempty"`
	Model   string `json:"model,omitempty"`
	// Active is the model's switch after the test, when the test set it.
	Active *bool `json:"active,omitempty"`
}

// runModelTest tests one model, on the account pin when set, else on any
// account of the provider as a client call would.
func (a *api) runModelTest(ctx context.Context, prov, model, pin string) modelTest {
	run := &testRun{pin: pin}
	ctx = context.WithValue(ctx, modelTestKey{}, run)
	var res modelTest
	if p, ok := provider.Lookup(prov); ok && p.API == "typesafe" {
		res = a.runTypeSafeTest(ctx, prov, model)
	} else {
		res = a.runChatTest(ctx, prov, model)
	}
	res.Account, res.Model = run.used, model
	return res
}

func (a *api) runChatTest(ctx context.Context, prov, model string) modelTest {
	body, _ := json.Marshal(map[string]any{"model": prov + "/" + model, "max_tokens": 64,
		"messages": []any{map[string]any{"role": "user", "content": "Reply with the single word OK."}}})
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req := withPrincipal(httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))), ccwJob)
	req.SetPathValue("path", "chat/completions")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	start := time.Now()
	a.v1(rec, req)
	res := modelTest{Status: rec.Code, Ms: time.Since(start).Milliseconds()}
	raw, _ := io.ReadAll(rec.Body)
	var d struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	json.Unmarshal(raw, &d)
	switch {
	case rec.Code == http.StatusOK && len(d.Choices) > 0:
		res.OK = true
		res.Message = strings.TrimSpace(d.Choices[0].Message.Content)
		if res.Message == "" && d.Choices[0].Message.Reasoning != "" {
			res.Message = "(reasoning only)"
		}
	case len(d.Error) > 0:
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(d.Error, &e) == nil && e.Message != "" {
			res.Message = e.Message
		} else {
			res.Message = strings.Trim(string(d.Error), `"`)
		}
	default:
		res.Message = strings.TrimSpace(string(raw))
	}
	if len(res.Message) > 300 {
		res.Message = res.Message[:300]
	}
	return res
}

// modelRows is the model table for MCP and the API.
func (a *api) modelRows(ctx context.Context, prov string) ([]store.ProviderModel, error) {
	a.catalogIDs(ctx, prov)
	return a.store.ListModels(prov)
}

func accountTestKey(id string) string { return "account-test:" + id }

// testAccount tests one account: {"model":"…"} or, without a model, the
// provider's model that last passed a test, else its first model switched on.
// The result is kept and served with the account.
func (a *api) testAccount(w http.ResponseWriter, r *http.Request) {
	var b modelsBody
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&b)
	res, code, err := a.runAccountTest(r.Context(), r.PathValue("id"), b.Model)
	if err != nil {
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, res)
}

func (a *api) runAccountTest(ctx context.Context, id, model string) (map[string]any, int, error) {
	var conn *store.Connection
	if list, err := a.store.ListConnections(); err == nil {
		for i := range list {
			if list[i].ID == id {
				conn = &list[i]
			}
		}
	}
	if conn == nil {
		return nil, http.StatusNotFound, errors.New("no such account")
	}
	if model == "" {
		model = a.accountTestModel(ctx, conn.Provider)
	}
	if model == "" {
		return nil, http.StatusBadRequest, errors.New("the provider lists no model to test")
	}
	res := a.runModelTest(ctx, conn.Provider, model, conn.ID)
	saved := map[string]any{"ok": res.OK, "status": res.Status, "ms": res.Ms, "message": res.Message,
		"model": model, "at": time.Now().UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(saved)
	a.store.SetSetting(accountTestKey(id), string(raw))
	return saved, http.StatusOK, nil
}

// accountTestModel picks the model an account test uses.
func (a *api) accountTestModel(ctx context.Context, prov string) string {
	rows, _ := a.modelRows(ctx, prov)
	for _, m := range rows {
		if m.Active && m.TestOK {
			return m.Model
		}
	}
	for _, m := range rows {
		if m.Active {
			return m.Model
		}
	}
	if len(rows) > 0 {
		return rows[0].Model
	}
	return ""
}

// accountTests serves the last test of every account, by connection id.
func (a *api) accountTests(w http.ResponseWriter, r *http.Request) {
	out := map[string]json.RawMessage{}
	if list, err := a.store.ListConnections(); err == nil {
		for _, c := range list {
			if v, _ := a.store.GetSetting(accountTestKey(c.ID)); v != "" {
				out[c.ID] = json.RawMessage(v)
			}
		}
	}
	writeJSON(w, out)
}

// providerModelsRaw serves the provider's last model list answer as it came,
// fetching it first when there is none.
func (a *api) providerModelsRaw(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	a.catalogIDs(r.Context(), prov)
	a.cat.mu.Lock()
	raw := a.cat.m[prov].raw
	a.cat.mu.Unlock()
	if raw == nil {
		writeError(w, http.StatusNotFound, "no list answer from this provider")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// modelPolicy is how a provider's models are switched on without the
// operator: with AutoTest, a model is on only when a test call works; with
// OnlyFree, only models whose name says free are on (and tested).
type modelPolicy struct {
	AutoTest   bool   `json:"autoTest"`
	OnlyFree   bool   `json:"onlyFree"`
	LastRun    string `json:"lastRun,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
}

const (
	autoTestEvery   = 6 * time.Hour
	fetchEvery      = time.Hour
	autoLoopTick    = 10 * time.Minute
	autoTestWorkers = 3
	// A model still rate-limited after the retry is switched off and tested
	// again this much later, instead of waiting for the next full run.
	limitedRetestAfter = 30 * time.Minute
)

// autoTestRetry is how long a rate-limited test waits before its one retry
// (a variable for tests).
var autoTestRetry = 15 * time.Second

// autoState tracks the providers being auto-tested, one run at a time each.
// A run can be cancelled (the policy changed under it); cancel waits until
// it has stopped, so the next run or switch change is not undone by it.
type autoState struct {
	mu      sync.Mutex
	running map[string]*autoRun
}

func newAutoState() autoState {
	return autoState{running: map[string]*autoRun{}}
}

type autoRun struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *autoState) start(prov string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[prov] != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	s.running[prov] = &autoRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	return true
}

func (s *autoState) run(prov string) *autoRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[prov]
}

func (s *autoState) done(prov string) {
	s.mu.Lock()
	if r := s.running[prov]; r != nil {
		r.cancel()
		close(r.done)
	}
	delete(s.running, prov)
	s.mu.Unlock()
}

// stop cancels a provider's run and waits for it to end.
func (s *autoState) stop(prov string) {
	if r := s.run(prov); r != nil {
		r.cancel()
		<-r.done
	}
}

func (s *autoState) isRunning(prov string) bool { return s.run(prov) != nil }

func policyKey(prov string) string { return "model-policy:" + prov }

func (a *api) modelPolicy(prov string) modelPolicy {
	var p modelPolicy
	if v, _ := a.store.GetSetting(policyKey(prov)); v != "" {
		json.Unmarshal([]byte(v), &p)
	}
	return p
}

func (a *api) saveModelPolicy(prov string, p modelPolicy) error {
	b, _ := json.Marshal(p)
	return a.store.SetSetting(policyKey(prov), string(b))
}

// isFree reports whether a model's name says it is free (OpenRouter's
// ":free" models, "…-free" variants).
func isFree(model string) bool { return strings.Contains(strings.ToLower(model), "free") }

// wanted reports whether a policy lets a model be on at all.
func (p modelPolicy) wanted(model string) bool { return !p.OnlyFree || isFree(model) }

// startsOn decides the switch of a model seen for the first time: on, unless
// it waits for its test or is not free under OnlyFree.
func (p modelPolicy) startsOn(model string) bool { return !p.AutoTest && p.wanted(model) }

// applyPolicy sets every model's switch from the policy: without AutoTest,
// on when wanted; with AutoTest, a full test run in the background.
func (a *api) applyPolicy(prov string, p modelPolicy) error {
	if p.AutoTest {
		// Marked running before the reply, so a caller polling sees the run.
		if a.auto.start(prov) {
			go a.autoTestHeld(prov, nil, false)
		}
		return nil
	}
	rows, err := a.store.ListModels(prov)
	if err != nil {
		return err
	}
	var on, off []string
	for _, r := range rows {
		if p.wanted(r.Model) {
			on = append(on, r.Model)
		} else {
			off = append(off, r.Model)
		}
	}
	if _, err := a.store.SetModelsActive(prov, on, true); err != nil {
		return err
	}
	_, err = a.store.SetModelsActive(prov, off, false)
	return err
}

// autoTest fetches the provider's list again (which drops the models it no
// longer lists), tests each wanted model and switches it on when it answers,
// off when it does not. A model still rate-limited after a retry is switched
// off too, and tested again limitedRetestAfter later. With
// models, only those are tested and the list is not fetched again.
func (a *api) autoTest(prov string, models []string) {
	if a.auto.start(prov) {
		a.autoTestHeld(prov, models, false)
	}
}

// autoTestHeld is autoTest once the provider is marked running. A retest (of
// models that were rate-limited) does not schedule another.
func (a *api) autoTestHeld(prov string, models []string, retest bool) {
	defer a.auto.done(prov)
	ctx := a.auto.run(prov).ctx
	p := a.modelPolicy(prov)
	full := models == nil
	if full {
		a.refreshCatalog(ctx, prov)
		rows, err := a.store.ListModels(prov)
		if err != nil {
			log.Printf("autotest %s: %v", prov, err)
			return
		}
		for _, r := range rows {
			models = append(models, r.Model)
		}
	}
	var test, off []string
	for _, m := range models {
		if p.wanted(m) {
			test = append(test, m)
		} else {
			off = append(off, m)
		}
	}
	a.store.SetModelsActive(prov, off, false)
	var mu sync.Mutex
	works, next := 0, 0
	var limited []string
	var wg sync.WaitGroup
	for w := 0; w < autoTestWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if next >= len(test) || ctx.Err() != nil {
					mu.Unlock()
					return
				}
				m := test[next]
				next++
				mu.Unlock()
				res := a.runModelTest(ctx, prov, m, "")
				if res.Status == http.StatusTooManyRequests {
					select {
					case <-time.After(autoTestRetry):
						res = a.runModelTest(ctx, prov, m, "")
					case <-ctx.Done():
					}
				}
				if ctx.Err() != nil {
					return
				}
				if res.Status == http.StatusTooManyRequests {
					res.Message = "rate limited, tested again later: " + res.Message
					mu.Lock()
					limited = append(limited, m)
					mu.Unlock()
				}
				a.store.RecordModelTest(prov, m, res.OK, res.Ms, res.Message, res.Account)
				a.store.SetModelsActive(prov, []string{m}, res.OK)
				if res.OK {
					mu.Lock()
					works++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if len(limited) > 0 && !retest && ctx.Err() == nil {
		time.AfterFunc(limitedRetestAfter, func() {
			if a.modelPolicy(prov).AutoTest && a.auto.start(prov) {
				a.autoTestHeld(prov, limited, true)
			}
		})
	}
	if full && ctx.Err() == nil {
		p = a.modelPolicy(prov)
		p.LastRun = time.Now().UTC().Format(time.RFC3339)
		p.LastResult = fmt.Sprintf("%d of %d tested work", works, len(test))
		a.saveModelPolicy(prov, p)
	}
}

// autoTestLoop keeps model lists current without a client asking: every
// autoLoopTick it fetches the list of each provider with an active account
// whose last fetch is older than fetchEvery (a failed fetch is retried on the
// next tick), then re-tests every provider with AutoTest on once its last run
// is older than autoTestEvery. Model lists change over days, so an hourly
// fetch catches a new or dropped model soon enough for one GET per provider.
func (a *api) autoTestLoop() {
	time.Sleep(time.Minute)
	for {
		provs := map[string]bool{}
		if conns, err := a.store.ListConnections(); err == nil {
			for _, c := range conns {
				if _, ok := a.providerFor(c); ok && c.IsActive {
					provs[c.Provider] = true
				}
			}
		}
		for prov := range provs {
			a.cat.mu.Lock()
			e, hit := a.cat.m[prov]
			a.cat.mu.Unlock()
			if !hit || !e.ok || time.Since(e.at) >= fetchEvery {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				a.refreshCatalog(ctx, prov)
				cancel()
			}
			p := a.modelPolicy(prov)
			if !p.AutoTest {
				continue
			}
			if t, err := time.Parse(time.RFC3339, p.LastRun); err == nil && time.Since(t) < autoTestEvery {
				continue
			}
			a.autoTest(prov, nil)
		}
		time.Sleep(autoLoopTick)
	}
}

// getModelPolicy serves a provider's policy and whether a test run is going.
func (a *api) getModelPolicy(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	writeJSON(w, map[string]any{"policy": a.modelPolicy(prov), "running": a.auto.isRunning(prov)})
}

// setModelPolicy stores {"autoTest":bool,"onlyFree":bool} and applies it.
func (a *api) setModelPolicy(w http.ResponseWriter, r *http.Request) {
	var b modelPolicy
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	prov := r.PathValue("id")
	p, err := a.updatePolicy(prov, b.AutoTest, b.OnlyFree)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot save the policy")
		return
	}
	writeJSON(w, map[string]any{"policy": p, "running": a.auto.isRunning(prov)})
}

func (a *api) updatePolicy(prov string, autoTest, onlyFree bool) (modelPolicy, error) {
	// A run under the old policy would undo the new one.
	a.auto.stop(prov)
	p := a.modelPolicy(prov)
	p.AutoTest, p.OnlyFree = autoTest, onlyFree
	if err := a.saveModelPolicy(prov, p); err != nil {
		return p, err
	}
	return p, a.applyPolicy(prov, p)
}

// TypeSafe's System One models answer typed questions, not chat, so their
// test is one /v1/systemone call that asks one question of each type about a
// state whose answers are not in doubt, and checks both the shape of every
// answer and that it is right.
const typeSafeTestState = "I was charged twice for my subscription this month. Refund the extra charge today, I need that money back urgently."

var typeSafeTestQuestions = map[string]any{
	"urgent":  map[string]any{"type": "noul", "instructions": "Does this message express urgency?"},
	"bug":     map[string]any{"type": "noul", "instructions": "Does this message report a software bug or crash?"},
	"team":    map[string]any{"type": "choice", "instructions": "Which team should handle this", "criteria": map[string]string{"billing": "Payment, charge or refund issues", "technical": "Bugs or integration problems", "sales": "Pricing questions from new customers"}},
	"feeling": map[string]any{"type": "score", "instructions": "How upset is the customer?", "criteria": []string{"Calm", "Frustrated", "Very angry"}},
}

type typeSafeAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Choice        string             `json:"choice"`
	Score         *float64           `json:"score"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// checkTypeSafe reads a System One answer to the test questions and returns
// a short summary, or what is wrong with it.
func checkTypeSafe(raw []byte) (string, error) {
	var d struct {
		Model   string                    `json:"model"`
		Answers map[string]typeSafeAnswer `json:"answers"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return "", fmt.Errorf("answer is not JSON")
	}
	get := func(k, typ string) (typeSafeAnswer, error) {
		a, ok := d.Answers[k]
		if !ok {
			return a, fmt.Errorf("no answer to %q", k)
		}
		if a.Type != "" && a.Type != typ {
			return a, fmt.Errorf("%q answered as %s, asked as %s", k, a.Type, typ)
		}
		return a, nil
	}
	var problems []string
	urgent, err := get("urgent", "noul")
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case urgent.Noul == nil || *urgent.Noul < 0 || *urgent.Noul > 1:
		problems = append(problems, "urgent: noul missing or out of 0..1")
	case *urgent.Noul < 0.5:
		problems = append(problems, fmt.Sprintf("urgent: %.2f, expected yes", *urgent.Noul))
	}
	bug, err := get("bug", "noul")
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case bug.Noul == nil || *bug.Noul < 0 || *bug.Noul > 1:
		problems = append(problems, "bug: noul missing or out of 0..1")
	case *bug.Noul >= 0.5:
		problems = append(problems, fmt.Sprintf("bug: %.2f, expected no", *bug.Noul))
	}
	team, err := get("team", "choice")
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case team.Confidence == nil || len(team.Probabilities) != 3:
		problems = append(problems, "team: confidence or probabilities missing")
	case team.Choice != "billing":
		problems = append(problems, fmt.Sprintf("team: %q, expected billing", team.Choice))
	}
	feeling, err := get("feeling", "score")
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case feeling.Score == nil || *feeling.Score < 0 || *feeling.Score > 2 || feeling.Confidence == nil:
		problems = append(problems, "feeling: score out of 0..2 or confidence missing")
	case *feeling.Score < 0.5:
		problems = append(problems, fmt.Sprintf("feeling: %.2f, expected upset", *feeling.Score))
	}
	if len(problems) > 0 {
		return "", fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return fmt.Sprintf("%s: urgent %.2f · bug %.2f · %s · upset %.2f", d.Model, *urgent.Noul, *bug.Noul, team.Choice, *feeling.Score), nil
}

// runTypeSafeTest sends the System One test through /v1, as a client would.
func (a *api) runTypeSafeTest(ctx context.Context, prov, model string) modelTest {
	body, _ := json.Marshal(map[string]any{"model": prov + "/" + model, "state": typeSafeTestState, "questions": typeSafeTestQuestions})
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req := withPrincipal(httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/systemone", strings.NewReader(string(body))), ccwJob)
	req.SetPathValue("path", "systemone")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	start := time.Now()
	a.v1(rec, req)
	res := modelTest{Status: rec.Code, Ms: time.Since(start).Milliseconds()}
	raw, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusOK {
		res.Message = strings.TrimSpace(string(raw))
	} else if sum, err := checkTypeSafe(raw); err != nil {
		res.Message = err.Error()
	} else {
		res.OK, res.Message = true, sum
	}
	if len(res.Message) > 300 {
		res.Message = res.Message[:300]
	}
	return res
}
