package diabridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// recoveryPath points the recoverer tests at a private port file, so a test that
// builds a gateway never writes over the file Dia really uses.
func recoveryPath(t *testing.T) string {
	t.Helper()
	t.Setenv(portFileEnv, filepath.Join(t.TempDir(), "dia-handler.json"))
	return portFilePath()
}

// A recoverer with no way to type must not wait: a machine that cannot drive
// Dia's UI should give the caller its 503 immediately, not twenty seconds later.
func TestARecovererThatCannotTypeFailsAtOnce(t *testing.T) {
	r := &recoverer{}
	if r.enabled() {
		t.Fatal("a recoverer with no type function must be disabled")
	}
	start := time.Now()
	if r.waitFor(context.Background(), recoveryPath(t)) {
		t.Fatal("waitFor reported success without typing anything")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waitFor took %v, want it to return at once", elapsed)
	}
}

// The whole point of recovery: a caller that finds no gateway gets one typed
// into Dia and then served, without having to be told to do anything.
func TestWaitForTypesAndWaitsForTheNewGateway(t *testing.T) {
	// A live listener stands in for the gateway Dia is about to build.
	srv := newFakeHandler(t)
	var typed atomic.Int32
	r := &recoverer{deliver: func() {
		typed.Add(1)
		// The keystroke lands, Dia builds a gateway, and the handler writes it.
		writeGateway(t, portOf(t, srv.front), "sk-test")
	}}

	if !r.waitFor(context.Background(), recoveryPath(t)) {
		t.Fatal("waitFor did not find the gateway it typed into Dia")
	}
	if got := typed.Load(); got != 1 {
		t.Errorf("typed %d times, want once", got)
	}
}

// A burst of callers must not each open a tab in Dia; they share one attempt.
func TestConcurrentCallersShareOneTypingAttempt(t *testing.T) {
	srv := newFakeHandler(t)
	var typed atomic.Int32
	r := &recoverer{deliver: func() {
		typed.Add(1)
		writeGateway(t, portOf(t, srv.front), "sk-test")
	}}
	// One path for the whole test: the callers share a gateway, so they have to
	// share the file it is written to.
	path := recoveryPath(t)
	const callers = 8
	done := make(chan bool, callers)
	for range callers {
		go func() { done <- r.waitFor(context.Background(), path) }()
	}
	for range callers {
		if !<-done {
			t.Error("a concurrent caller did not get the gateway")
		}
	}
	if got := typed.Load(); got != 1 {
		t.Errorf("typed %d times for %d concurrent callers, want one", got, callers)
	}
}

// A caller that goes away must not keep the bridge typing into Dia.
func TestWaitForStopsWhenTheCallerGivesUp(t *testing.T) {
	var typed atomic.Int32
	r := &recoverer{deliver: func() { typed.Add(1) }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if r.waitFor(ctx, recoveryPath(t)) {
		t.Fatal("waitFor reported success for a cancelled caller")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waitFor took %v after cancellation, want it to return at once", elapsed)
	}
}

// Recovery is what a caller hits after a turn ends, so the back face must use it
// and still answer 200 when a gateway appears.
func TestTheBackFaceRecoversInsteadOfRefusingTheCaller(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	// No gateway on file yet: this is the state a dead turn leaves behind.
	b.recover = &recoverer{deliver: func() {
		writeGateway(t, portOf(t, f.front), "sk-test")
	}}

	resp, body := postBack(t, b, `{"model":"claude-opus-4-8","stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	if got := <-f.gotAuth; got != "Bearer sk-test" {
		t.Errorf("handler saw credential %q, want the one recovery typed in", got)
	}
}

func TestTheFrontFaceStartsRecoveryWhenDiaHasNoGateway(t *testing.T) {
	// Dia's own chat arrives at the front face. With no gateway alive it cannot
	// be forwarded, but it is also the one request that proves Dia is mid-turn,
	// so recovery starts from here and not only from a caller's own request.
	b := startBridge(t)
	var typed atomic.Int32
	b.recover = &recoverer{deliver: func() {
		typed.Add(1)
		// Port 1 never accepts, so the wait ends without a usable gateway.
		writeGateway(t, 1, "sk-test")
	}}

	req, err := http.NewRequest(http.MethodPost,
		"http://"+b.Addr()+"/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-5-5:streamRawPredict",
		strings.NewReader(`{"model":"claude-opus-5-5"}`))
	if err != nil {
		t.Fatalf("build front request: %v", err)
	}
	rec := httptest.NewRecorder()
	b.front.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 for a Dia turn with no gateway", rec.Code)
	}
}
