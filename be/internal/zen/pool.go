package zen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sync"
	"time"
)

// The upstream gives a prompt cache per session id, so every call of one
// conversation has to arrive with the same id. The pool holds one entry per
// caller conversation: a Claude Code session keeps one upstream session
// for as long as it keeps talking, and only for that long. An entry is released
// after ttl without a request, and the least recently used one goes when the pool
// is full. The key is a caller id or a digest, never prompt text.

// Session is one upstream session id, held for a while and then dropped.
type Session struct {
	ID  string
	Key string
	// Caller is the caller's own session id and Source where it was read from.
	// Both are empty for a session keyed on the body alone.
	Caller, Source string
	Created        time.Time
	LastUsed       time.Time
	Uses           int
}

// Pool is the caller to session dictionary.
type Pool struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int // 0 means no limit
	byKey map[string]*Session
	now   func() time.Time
}

// NewPool makes a pool that releases an entry idle for ttl and holds at most max
// (0 for no limit).
func NewPool(ttl time.Duration, max int) *Pool {
	return &Pool{ttl: ttl, max: max, byKey: map[string]*Session{}, now: time.Now}
}

// For returns the session of one call, minting it when its key is new. When the
// caller sent a session id it is the key, so a body that grows on every turn
// still lands on one session; a caller with none is keyed on a digest of its
// body. A caller id that is already an upstream session id is used as it stands.
func (p *Pool) For(caller, source string, body []byte) Session {
	now := p.now()
	key := "body:" + BodyKey(body)
	if caller != "" {
		key = "caller:" + caller
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked(now)
	if s := p.byKey[key]; s != nil {
		s.LastUsed = now
		s.Uses++
		return *s
	}
	// Room is made first: past the cap the least recently used entry goes, so a
	// busy gateway keeps serving instead of growing until the next sweep.
	for p.max > 0 && len(p.byKey) >= p.max {
		p.evictOldestLocked()
	}
	id := caller
	if !IsSessionID(id) {
		id = NewSessionID(now)
	}
	s := &Session{ID: id, Key: key, Caller: caller, Source: source, Created: now, LastUsed: now, Uses: 1}
	p.byKey[key] = s
	return *s
}

// TTL is how long an idle session is held; Max is how many are held at once.
func (p *Pool) TTL() time.Duration { return p.ttl }
func (p *Pool) Max() int           { return p.max }

// Release drops one session, so the next call of its key mints a new id. It is
// for a call that never reached the upstream: nothing was cached there.
func (p *Pool) Release(key string) {
	p.mu.Lock()
	delete(p.byKey, key)
	p.mu.Unlock()
}

// Live returns the held sessions, newest use first.
func (p *Pool) Live() []Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked(p.now())
	out := make([]Session, 0, len(p.byKey))
	for _, s := range p.byKey {
		out = append(out, *s)
	}
	for i := 1; i < len(out); i++ { // insertion sort: the pool holds a few hundred at most
		for j := i; j > 0 && out[j].LastUsed.After(out[j-1].LastUsed); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (p *Pool) sweepLocked(now time.Time) {
	for k, s := range p.byKey {
		if now.Sub(s.LastUsed) > p.ttl {
			delete(p.byKey, k)
		}
	}
}

func (p *Pool) evictOldestLocked() {
	var oldest *Session
	for _, s := range p.byKey {
		if oldest == nil || s.LastUsed.Before(oldest.LastUsed) {
			oldest = s
		}
	}
	if oldest != nil {
		delete(p.byKey, oldest.Key)
	}
}

const (
	base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	hexits = "0123456789abcdef"
)

// NewSessionID mints an id in OpenCode's shape: "ses_", 12 hex, 14 base62. The
// head is a millisecond timestamp because that is what OpenCode's own create
// puts there. The upstream only requires the shape, but matching the client
// keeps the id ordinary and guarantees one is never handed out twice.
func NewSessionID(now time.Time) string {
	ms := uint64(now.UnixMilli())
	head := make([]byte, 12)
	for i := range head {
		head[i] = hexits[(ms>>(44-4*uint(i)))&0xF]
	}
	tail := make([]byte, 14)
	for i := range tail {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(base62))))
		if err != nil {
			n = big.NewInt(int64(i))
		}
		tail[i] = base62[n.Int64()]
	}
	return "ses_" + string(head) + string(tail)
}

// volatile fields change on every call and carry no conversation. Left in the
// key they would mint a session per request.
var volatile = map[string]bool{
	"request_id": true, "requestId": true, "id": true, "timestamp": true, "ts": true,
	"created": true, "created_at": true, "nonce": true, "client_request_id": true,
	"session_id": true, "sessionId": true, "conversation_id": true, "conversationId": true,
	"user": true, "metadata": true,
}

// BodyKey is the pool key of a body: a digest of it with the volatile fields
// dropped and the keys sorted, so key order and whitespace cannot split one
// conversation in two. A body that is not JSON is hashed as it is.
func BodyKey(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}
	dropVolatile(v)
	norm, err := json.Marshal(v) // map keys are marshalled sorted
	if err != nil {
		norm = body
	}
	sum := sha256.Sum256(norm)
	return hex.EncodeToString(sum[:])
}

func dropVolatile(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k := range volatile {
			delete(x, k)
		}
		for _, e := range x {
			dropVolatile(e)
		}
	case []any:
		for _, e := range x {
			dropVolatile(e)
		}
	}
}
