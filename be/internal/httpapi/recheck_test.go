package httpapi

import (
	"sync"
	"testing"
	"time"
)

// A burst of 429s must not hammer a provider's quota endpoint: one read per account
// per interval, whatever the burst. Without the gate, ten 429s in a row are ten quota
// reads on a provider that is already refusing.
func TestRecheckGateAdmitsOneReadPerAccountPerInterval(t *testing.T) {
	var g recheckGate
	if !g.allow("acct-1") {
		t.Fatal("the first read was refused")
	}
	for range 9 {
		if g.allow("acct-1") {
			t.Fatal("a burst let a second read through inside the interval")
		}
	}
	// A second account is its own gate: one account's burst must not delay another's.
	if !g.allow("acct-2") {
		t.Error("a different account was blocked by the first one's gate")
	}
}

// The gate is keyed per account and holds its own clock, and it is safe under the
// concurrent 429s that a busy gateway produces.
func TestRecheckGateIsPerAccountAndConcurrencySafe(t *testing.T) {
	var g recheckGate
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := map[string]int{}
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := []string{"a", "b", "c"}[i%3]
			if g.allow(id) {
				mu.Lock()
				admitted[id]++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	for id, n := range admitted {
		if n != 1 {
			t.Errorf("account %s got %d reads in one burst, want 1", id, n)
		}
	}
	if len(admitted) != 3 {
		t.Errorf("%d accounts were admitted, want 3", len(admitted))
	}
}

// The interval passes, so a spent account is re-read rather than believed forever.
func TestRecheckGateOpensAgainAfterItsInterval(t *testing.T) {
	g := recheckGate{last: map[string]time.Time{
		"acct-1": time.Now().Add(-2 * quotaRecheckEvery),
	}}
	if !g.allow("acct-1") {
		t.Error("a gate whose interval had passed still refused")
	}
}
