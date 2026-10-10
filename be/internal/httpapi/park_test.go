package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// A 429 with a Retry-After parks the account until that moment, and the next request
// skips it rather than paying the round trip for the same refusal.
func TestParkSkipsTheAccountUntilTheProviderNames(t *testing.T) {
	var secondHits int
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer second.Close()

	var firstHits int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer first.Close()

	s := openStore(t)
	c1, _ := s.CreateConnection("groq", "one", "k1")
	if _, err := s.CreateConnection("groq", "two", "k2"); err != nil {
		t.Fatal(err)
	}
	api, h := newServer(s, map[string]string{"groq": first.URL}, nil)

	if rec := postV1(h, `{"model":"groq/llama","messages":[]}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("first request = %d, want the 429 relayed", rec.Code)
	}

	// The account is parked now.
	if until := api.spentUntil(c1.ID, "llama"); until.IsZero() {
		t.Fatal("the 429 account was not parked")
	}

	// A request that names another model still tries it: a park is per model.
	if until := api.spentUntil(c1.ID, "other-model"); !until.IsZero() {
		t.Error("a park leaked to another model")
	}

	// The parked account is out of the rotation, so the request goes to the other one.
	api.baseOverride = map[string]string{"groq": second.URL}
	if rec := postV1(h, `{"model":"groq/llama","messages":[]}`); rec.Code != http.StatusOK {
		t.Errorf("second request = %d, want 200 from the unparked account", rec.Code)
	}
	if secondHits == 0 {
		t.Error("the unparked account was never called")
	}
	_ = firstHits
}

// Every account parked keeps them all: a gateway that answers "no usable account" for
// a provider that would have said why is worse than one that relays the truth.
func TestSkipSpentKeepsEveryAccountWhenAllAreParked(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c1, _ := s.CreateConnection("groq", "one", "k1")
	c2, _ := s.CreateConnection("groq", "two", "k2")
	attempts := []attempt{{conn: c1}, {conn: c2}}
	a.parks.set(c1.ID, "m", time.Now().Add(time.Hour))
	a.parks.set(c2.ID, "m", time.Now().Add(time.Hour))
	if got := a.skipSpent(attempts, "m"); len(got) != 2 {
		t.Errorf("all parked: kept %d attempts, want 2 so the provider's own error reaches the caller", len(got))
	}

	// One account left keeps the rotation honest.
	a.parks.forget(c2.ID, "m")
	got := a.skipSpent(attempts, "m")
	if len(got) != 1 || got[0].conn.ID != c2.ID {
		t.Errorf("one parked: kept %+v, want only the healthy account", got)
	}
}

// A 401 clears the park: a refused credential is not a spent quota, so the account
// must be tried again once the credential is refreshed. A provider that keeps saying
// 401 answers every retry, so the loop ends on that answer with the park cleared.
func TestAnUnauthorizedAnswerClearsThePark(t *testing.T) {
	s := openStore(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer up.Close()
	c1, _ := s.CreateConnection("groq", "one", "k1")
	if _, err := s.CreateConnection("groq", "two", "k2"); err != nil {
		t.Fatal(err)
	}
	a, h := newServer(s, map[string]string{"groq": up.URL}, nil)

	a.parks.set(c1.ID, "llama", time.Now().Add(time.Hour))
	if at := a.spentUntil(c1.ID, "llama"); at.IsZero() {
		t.Fatal("precondition: the account is parked")
	}
	// The parked account is skipped, so the 401 this request sees belongs to the
	// other one. Its own park is untouched, which is right: a park is per account.
	if rec := postV1(h, `{"model":"groq/llama","messages":[]}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the 401 relayed", rec.Code)
	}
	if at := a.spentUntil(c1.ID, "llama"); at.IsZero() {
		t.Error("another account's 401 cleared a park that was not its own")
	}

	// Once the park runs out the account is called again, and a 401 from it clears
	// its own park: a refused credential is not a spent quota.
	a.parks.set(c1.ID, "llama", time.Now().Add(time.Hour))
	a.noteRefusal(&failoverCall{body: []byte(`{"model":"groq/llama","messages":[]}`)},
		accountTry{conn: c1, model: "llama"},
		&http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}})
	if at := a.spentUntil(c1.ID, "llama"); !at.IsZero() {
		t.Errorf("its own 401 left the account parked until %s", at)
	}
}

// noteRefusal is where the two statuses are recorded, and it reads them off the
// answer alone: a test that drives it directly says which one is which.
func TestNoteRefusalClearsEveryParkOnA401(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c, _ := s.CreateConnection("groq", "one", "k1")
	a.parks.set(c.ID, "llama", time.Now().Add(time.Hour))
	a.parks.set(c.ID, "other", time.Now().Add(time.Hour))
	a.parks.set("another", "llama", time.Now().Add(time.Hour))

	a.noteRefusal(&failoverCall{body: []byte(`{"model":"groq/llama","messages":[]}`)},
		accountTry{conn: c, model: "llama"},
		&http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}})

	for _, m := range []string{"llama", "other"} {
		if at := a.parks.until(c.ID, m); !at.IsZero() {
			t.Errorf("401 left a park for %s until %s", m, at)
		}
	}
	// Another account's park is none of this account's business.
	if a.parks.until("another", "llama").IsZero() {
		t.Error("the 401 cleared another account's park")
	}
}

// Retry-After is read in both forms RFC 9110 allows, and an unusable one parks nothing.
func TestParkFromHeader(t *testing.T) {
	for _, c := range []struct {
		name     string
		header   string
		wantPark bool
	}{
		{"seconds", "120", true},
		{"zero parks nothing", "0", false},
		{"negative parks nothing", "-30", false},
		{"past date parks nothing", "Mon, 01 Jan 2001 00:00:00 GMT", false},
		{"unparsable parks nothing", "soon", false},
		{"absent parks nothing", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			if c.header != "" {
				h.Set("Retry-After", c.header)
			}
			got := parkFromHeader(h)
			if c.wantPark && !got.After(time.Now()) {
				t.Errorf("parkFromHeader(%q) parked nothing", c.header)
			}
			if !c.wantPark && !got.IsZero() {
				t.Errorf("parkFromHeader(%q) = %s, want no park", c.header, got)
			}
		})
	}

	// A future HTTP date is a park, and a past one is not.
	future := http.Header{}
	future.Set("Retry-After", time.Now().Add(2*time.Hour).UTC().Format(http.TimeFormat))
	if !parkFromHeader(future).After(time.Now()) {
		t.Error("an HTTP date in the future did not park")
	}
	past := http.Header{}
	past.Set("Retry-After", time.Now().Add(-2*time.Hour).UTC().Format(http.TimeFormat))
	if !parkFromHeader(past).IsZero() {
		t.Error("an HTTP date in the past parked the account")
	}
}

// A Retry-After longer than parkMax is not honoured in full: an account parked for a
// day would look like a broken gateway rather than a spent quota.
func TestParkFromHeaderIsCapped(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "999999")
	if got := parkFromHeader(h); got.After(time.Now().Add(parkMax)) {
		t.Errorf("park = %s, want at most parkMax (%s)", got, parkMax)
	}
}

// The quota ccw already cached parks an account with no Retry-After at all, which is
// the case for every provider that reports a window instead of a header.
func TestParkFromQuotaUsesTheWindowsReset(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c, _ := s.CreateConnection("groq", "one", "k1")

	// No cache: nothing to park on.
	if at := a.parkFromQuota(c.ID, "llama"); !at.IsZero() {
		t.Errorf("no cached quota parked the account until %s", at)
	}

	reset := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	a.quota.mu.Lock()
	a.quota.m[c.ID] = AccountQuota{ConnectionID: c.ID,
		Windows: []QuotaWindow{{Name: "5h", UsedPct: 100, ResetAt: reset}}}
	a.quota.mu.Unlock()

	until := a.parkFromQuota(c.ID, "llama")
	if until.IsZero() {
		t.Fatal("a spent window with a reset did not park the account")
	}
	if d := time.Until(until); d < 25*time.Minute || d > 35*time.Minute {
		t.Errorf("park = %s (%s away), want the window's own reset ~30m", until, d)
	}

	// Quota left is not a park.
	a.quota.mu.Lock()
	a.quota.m[c.ID] = AccountQuota{ConnectionID: c.ID,
		Windows: []QuotaWindow{{Name: "5h", UsedPct: 40, ResetAt: reset}}}
	a.quota.mu.Unlock()
	if at := a.parkFromQuota(c.ID, "llama"); !at.IsZero() {
		t.Errorf("quota left but parked until %s", at)
	}

	// Spent with no reset named is not a park either: an account is never put aside
	// for good over a window ccw cannot date.
	a.quota.mu.Lock()
	a.quota.m[c.ID] = AccountQuota{ConnectionID: c.ID,
		Windows: []QuotaWindow{{Name: "5h", UsedPct: 100}}}
	a.quota.mu.Unlock()
	if at := a.parkFromQuota(c.ID, "llama"); !at.IsZero() {
		t.Errorf("a reset ccw cannot read still parked the account until %s", at)
	}
}

// A park expires on its own: past the moment it named, the entry is gone and the
// account is ordinary again.
func TestAParkExpiresWithoutAnyoneClearingIt(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c, _ := s.CreateConnection("groq", "one", "k1")

	a.parks.set(c.ID, "llama", time.Now().Add(40*time.Millisecond))
	if a.spentUntil(c.ID, "llama").IsZero() {
		t.Fatal("precondition: parked")
	}
	time.Sleep(60 * time.Millisecond)
	if at := a.spentUntil(c.ID, "llama"); !at.IsZero() {
		t.Errorf("still parked after its moment, until %s", at)
	}
	a.parks.mu.Lock()
	n := len(a.parks.held)
	a.parks.mu.Unlock()
	if n != 0 {
		t.Errorf("the expired entry was kept: %d held", n)
	}
}

// OpenCode Zen has no quota endpoint, so Retry-After is the only thing that says when
// its free tier comes back. The whole point of the port.
func TestZen429ParksByItsRetryAfterAlone(t *testing.T) {
	h, up, a := zenRig(t)
	up.status = http.StatusTooManyRequests
	up.retryAfter = "54000"

	if rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","messages":[]}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want the 429 relayed", rec.Code)
	}

	conns, _ := a.store.ListConnections()
	var zen store.Connection
	for _, c := range conns {
		if c.Provider == "opencode" {
			zen = c
		}
	}
	if zen.ID == "" {
		t.Fatal("no opencode connection")
	}
	until := a.spentUntil(zen.ID, "big-pickle")
	if until.IsZero() {
		t.Fatal("the Zen 429 with a Retry-After did not park the account")
	}
	// parkMax caps the wait: an account parked for the 15 hours Zen names would look
	// like a broken gateway rather than a spent quota, so the park is the cap.
	if d := time.Until(until); d > parkMax+time.Minute {
		t.Errorf("park = %s away, want at most parkMax (%s)", d, parkMax)
	}
}
