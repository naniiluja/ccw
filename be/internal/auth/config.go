package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the single operator's credentials. It keeps only the hash of the
// password, so no plaintext is compiled in, committed or held in memory. The
// person logs in with the password alone; a machine uses the bearer token.
type Config struct {
	APIToken     string
	passwordHash string
	sessionKey   []byte
	ttlSeconds   int64
}

func nowUnix() int64 { return time.Now().Unix() }

// CheckPassword verifies the password, the only browser login factor. The login
// guard in httpapi caps how many wrong ones are checked.
func (c *Config) CheckPassword(password string) bool {
	return verifyPassword(c.passwordHash, password)
}

// CheckAPIToken verifies an "Authorization: Bearer <token>" header for machines.
func (c *Config) CheckAPIToken(header string) bool {
	const p = "Bearer "
	if len(header) <= len(p) || header[:len(p)] != p {
		return false
	}
	got := header[len(p):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(c.APIToken)) == 1
}

// IssueSession returns a signed cookie value valid for the configured TTL.
func (c *Config) IssueSession() string {
	return signSession(c.sessionKey, nowUnix()+c.ttlSeconds)
}

// ValidSession reports whether a cookie value is authentic and unexpired.
func (c *Config) ValidSession(tok string) bool {
	return validSession(c.sessionKey, tok, nowUnix())
}

// TTLSeconds is the session lifetime, for the cookie Max-Age.
func (c *Config) TTLSeconds() int { return int(c.ttlSeconds) }

// IssueDevice returns a value for the device cookie: a random id and an HMAC
// over it. It grants no access. The login guard reads it to charge a wrong
// password to this browser's own budget instead of the public one.
func (c *Config) IssueDevice() string {
	id := hex.EncodeToString(randomBytes(16))
	return id + "." + sign(c.sessionKey, id)
}

// DeviceID returns the id inside a device cookie. ok is false when the value
// carries no authentic signature, so a forged cookie names no device.
func (c *Config) DeviceID(value string) (id string, ok bool) {
	id, sig, cut := strings.Cut(value, ".")
	if !cut || id == "" {
		return "", false
	}
	if !hmac.Equal([]byte(sig), []byte(sign(c.sessionKey, id))) {
		return "", false
	}
	return id, true
}

// FromHash builds a Config for a stored password hash; the token, session key
// and session lifetime come from the environment. A hash that does not parse is
// an error: it would refuse every password, so the caller must stop instead of
// starting a dead gate. A missing session key is generated so a restart
// invalidates old cookies.
func FromHash(passwordHash string) (*Config, error) {
	if _, err := parseHash(passwordHash); err != nil {
		return nil, fmt.Errorf("the stored password hash: %w", err)
	}
	ttl := int64(12 * 3600)
	if v, err := strconv.ParseInt(os.Getenv("CCW_SESSION_TTL"), 10, 64); err == nil && v > 0 {
		ttl = v
	}
	key := []byte(os.Getenv("CCW_SESSION_KEY"))
	if len(key) == 0 {
		key = randomBytes(32)
	}
	return &Config{
		APIToken:     os.Getenv("CCW_API_TOKEN"),
		passwordHash: passwordHash,
		sessionKey:   key,
		ttlSeconds:   ttl,
	}, nil
}

// NewConfig builds a Config from explicit values, for callers that hold them.
// passwordHash is a value from HashPassword.
func NewConfig(passwordHash, apiToken string, sessionKey []byte, ttlSeconds int64) *Config {
	if len(sessionKey) == 0 {
		sessionKey = randomBytes(32)
	}
	return &Config{APIToken: apiToken, passwordHash: passwordHash, sessionKey: sessionKey, ttlSeconds: ttlSeconds}
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
