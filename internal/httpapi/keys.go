package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// API keys are managed from the dashboard only: a machine holding one key must
// not be able to mint more.

func (a *api) listKeys(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListAPIKeys()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read api keys")
		return
	}
	writeJSON(w, map[string]any{"keys": list})
}

// createKey makes a key from {"name": "…", "models": […]} and returns it in
// full once. No models means every model.
func (a *api) createKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string   `json:"name"`
		Models []string `json:"models"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "key"
	}
	k, err := a.store.CreateAPIKey(name, body.Models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create api key")
		return
	}
	writeJSON(w, k)
}

func (a *api) revealKey(w http.ResponseWriter, r *http.Request) {
	k, err := a.store.RevealAPIKey(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	writeJSON(w, map[string]any{"key": k})
}

func (a *api) setKeyActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := a.store.SetAPIKeyEnabled(r.PathValue("id"), body.Active); err != nil {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// setKeyModels replaces the models a key may call from {"models": […]}.
func (a *api) setKeyModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := a.store.SetAPIKeyModels(r.PathValue("id"), body.Models); err != nil {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *api) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteAPIKey(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// setKeyLimits stores {"expiresAt":"<RFC 3339>"|"","rpm":n}; a field left out
// keeps its value.
func (a *api) setKeyLimits(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpiresAt *string `json:"expiresAt"`
		RPM       *int    `json:"rpm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	id := r.PathValue("id")
	cur, ok := a.keyByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	if body.ExpiresAt != nil {
		cur.ExpiresAt = ""
		if *body.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "expiresAt: an RFC 3339 time, or empty for none")
				return
			}
			cur.ExpiresAt = t.UTC().Format(time.RFC3339)
		}
	}
	if body.RPM != nil {
		if *body.RPM < 0 || *body.RPM > maxKeyRPM {
			writeError(w, http.StatusBadRequest, "rpm: 0 (no cap) to "+strconv.Itoa(maxKeyRPM))
			return
		}
		cur.RPM = *body.RPM
	}
	if err := a.store.SetAPIKeyLimits(id, cur.ExpiresAt, cur.RPM); err != nil {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "expiresAt": cur.ExpiresAt, "rpm": cur.RPM})
}

// keyUsage serves a key's last 30 days by day and model, and its requests in
// the last minute against its cap.
func (a *api) keyUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	k, ok := a.keyByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown api key")
		return
	}
	now := time.Now()
	rows, err := a.store.KeyUsage(id, usageDay(now.AddDate(0, 0, -29)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read usage")
		return
	}
	writeJSON(w, map[string]any{"rows": rows, "rpm": k.RPM, "lastMinute": a.keyLim.count(id, now),
		"expiresAt": k.ExpiresAt, "lastUsed": k.LastUsed, "today": usageDay(now)})
}

func (a *api) keyByID(id string) (store.APIKey, bool) {
	list, err := a.store.ListAPIKeys()
	if err != nil {
		return store.APIKey{}, false
	}
	for _, k := range list {
		if k.ID == id {
			return k, true
		}
	}
	return store.APIKey{}, false
}

// maxKeyRPM bounds a key's cap, and with it the timestamps kept per key.
const maxKeyRPM = 100000

// keyLimiter counts each API key's requests in the last 60 seconds.
type keyLimiter struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func newKeyLimiter() keyLimiter {
	return keyLimiter{seen: map[string][]time.Time{}}
}

// allow records a request by key at now when the key made fewer than rpm in
// the minute before. Otherwise it records nothing and returns how long until
// the oldest of those leaves the window.
func (l *keyLimiter) allow(key string, rpm int, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := l.window(key, now)
	if len(ts) >= rpm {
		return false, ts[len(ts)-rpm].Add(time.Minute).Sub(now)
	}
	l.seen[key] = append(ts, now)
	return true, 0
}

// count returns the requests key made in the minute before now.
func (l *keyLimiter) count(key string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.window(key, now))
}

func (l *keyLimiter) window(key string, now time.Time) []time.Time {
	ts := l.seen[key]
	i := 0
	for i < len(ts) && !ts[i].After(now.Add(-time.Minute)) {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.seen, key)
		return nil
	}
	l.seen[key] = ts
	return ts
}

// ccw keeps stored timestamps in UTC, which is correct for comparison. The
// one value a person reads as a calendar day is the usage total, so its day
// boundary must follow the operator's clock, not UTC. On a GMT+7 server a UTC
// day rolls the total over at 07:00 local; a local day rolls it at midnight.

var (
	tzOnce sync.Once
	tzLoc  *time.Location
)

// reportLocation is the zone the usage day boundary follows. CCW_TZ names an
// IANA zone (for example "Asia/Bangkok"); when it is empty or unknown, the zone
// is a fixed GMT+7, which is Vietnam's offset all year (no daylight saving).
func reportLocation() *time.Location {
	tzOnce.Do(func() {
		if name := os.Getenv("CCW_TZ"); name != "" {
			if loc, err := time.LoadLocation(name); err == nil {
				tzLoc = loc
				return
			}
		}
		tzLoc = time.FixedZone("+07", 7*60*60)
	})
	return tzLoc
}

// usageDay is the calendar day of t in the report zone, as "2006-01-02".
func usageDay(t time.Time) string { return usageDayIn(t, reportLocation()) }

// usageDayIn is the testable core of usageDay.
func usageDayIn(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02")
}
