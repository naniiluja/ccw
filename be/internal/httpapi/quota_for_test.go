package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

func openQuotaStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestQuotaForFillsAZeroCache shows that quotaFor works on an api whose quota
// cache maps were never made, and stores what it read.
func TestQuotaForFillsAZeroCache(t *testing.T) {
	s := openQuotaStore(t)
	c, _ := s.CreateConnection("mistral", "g", "key-x")
	a := &api{store: s}
	q := a.quotaFor(context.Background(), c, false)
	if q.Error != "" || q.Windows == nil || q.Resets == nil || q.ConnectionID != c.ID {
		t.Errorf("quota = %+v, want empty windows and resets, no error", q)
	}
	if _, ok := a.quota.m[c.ID]; !ok {
		t.Error("quota was not cached")
	}
	if len(a.quota.flights) != 0 {
		t.Errorf("flights = %v, want none left", a.quota.flights)
	}
}

// TestQuotaForWaiterGivesUpWithItsContext shows that a caller waiting on
// another caller's read returns its own context error.
func TestQuotaForWaiterGivesUpWithItsContext(t *testing.T) {
	s := openQuotaStore(t)
	c, _ := s.CreateConnection("mistral", "g", "key-x")
	a, _ := newServer(s, nil, nil)
	a.quota.mu.Lock()
	a.quota.flights[c.ID] = &quotaFlight{seq: 1, done: make(chan struct{})}
	a.quota.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := a.quotaFor(ctx, c, false)
	if q.Error != context.Canceled.Error() || q.Windows == nil || q.Resets == nil || q.Label != "g" {
		t.Errorf("quota = %+v, want the context error", q)
	}
}

// TestQuotaForLeaderGivesUpWithItsContext shows that the caller that starts a
// read stops waiting with its context, while the read still lands in the cache.
func TestQuotaForLeaderGivesUpWithItsContext(t *testing.T) {
	orig := quotaFetchers["claude"]
	t.Cleanup(func() { quotaFetchers["claude"] = orig })
	release := make(chan struct{})
	quotaFetchers["claude"] = func(_ *api, _ context.Context, _ store.Connection, _ string) (AccountQuota, error) {
		<-release
		return AccountQuota{Plan: "late", Windows: []QuotaWindow{{Name: "5h"}}}, nil
	}
	s := openQuotaStore(t)
	c, _ := s.CreateConnection("claude", "acc", "token-x")
	a, _ := newServer(s, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	q := a.quotaFor(ctx, c, false)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || q.Error != context.DeadlineExceeded.Error() {
		t.Fatalf("quota = %+v, want the deadline error", q)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.quota.mu.Lock()
		got, ok := a.quota.m[c.ID]
		_, flying := a.quota.flights[c.ID]
		a.quota.mu.Unlock()
		if ok && !flying {
			if got.Plan != "late" || got.Source != "api" {
				t.Errorf("cached = %+v, want the late read", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the read never reached the cache")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestQuotaForHeadersReplaceAFailedRead shows that rate-limit headers fill the
// windows, and clear the error, when the provider's quota read fails.
func TestQuotaForHeadersReplaceAFailedRead(t *testing.T) {
	orig := quotaFetchers["claude"]
	t.Cleanup(func() { quotaFetchers["claude"] = orig })
	quotaFetchers["claude"] = func(_ *api, _ context.Context, _ store.Connection, _ string) (AccountQuota, error) {
		return AccountQuota{}, errors.New("boom")
	}
	s := openQuotaStore(t)
	c, _ := s.CreateConnection("claude", "acc", "token-x")
	a, _ := newServer(s, nil, nil)

	if q := a.quotaFor(context.Background(), c, true); q.Error != "boom" || len(q.Windows) != 0 {
		t.Fatalf("without headers quota = %+v, want error boom", q)
	}
	a.rate.capture(c.ID, http.Header{
		"X-Ratelimit-Limit-Requests":     {"100"},
		"X-Ratelimit-Remaining-Requests": {"40"},
	})
	q := a.quotaFor(context.Background(), c, true)
	if q.Error != "" || q.Source != "headers" || len(q.Windows) != 1 || q.Windows[0].Name != "requests" {
		t.Errorf("with headers quota = %+v, want one header window and no error", q)
	}
}
