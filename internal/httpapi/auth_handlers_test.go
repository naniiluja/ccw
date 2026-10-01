package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
)

const (
	addrLoopback = "127.0.0.1:54321"
	addrTunnel   = "127.0.0.1:54322" // cloudflared connects from this machine
	// wrongPassword is long enough to be a valid password, and is not testPassword.
	wrongPassword = "wrong-password-1"
)

// loginPost builds a POST /login the way a browser sends it. remoteAddr is the
// connection the server sees: the tunnel connects from loopback and names the
// real caller in CF-Connecting-IP.
func loginPost(password, remoteAddr string, header map[string]string, cookies ...*http.Cookie) *http.Request {
	form := url.Values{"password": {password}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Host = "127.0.0.1:20130"
	r.RemoteAddr = remoteAddr
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return r
}

func send(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// fillPublicBudget sends wrong passwords from a new address each time, so only the
// public budget can stop them. It returns the handler's last status.
func fillPublicBudget(t *testing.T, h http.Handler) {
	t.Helper()
	for i := 0; i < auth.LoginPublicMax; i++ {
		rec := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{
			"CF-Connecting-IP": fmt.Sprintf("198.51.100.%d", i%256),
		}))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("fill attempt %d: code=%d, want 401", i, rec.Code)
		}
	}
}

// T1-1 (2) and T7-5: 200 concurrent wrong passwords through the real handler. A 401
// means the password was checked; a 429 means the guard stopped the request
// before that, so the count of 401 answers is the count of password checks.
func TestLoginHandlerEvaluatesNoMoreCodesThanTheCap(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfOf func(i int) string
		cap  int
	}{
		{"rotating addresses", func(i int) string { return fmt.Sprintf("198.51.100.%d", i) }, auth.LoginPublicMax},
		{"one address", func(int) string { return "198.51.100.7" }, auth.LoginPerIPMax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStore(t)
			h := NewWithAuth(s, nil, authConfig())
			var evaluated, refused atomic.Int64
			var wg sync.WaitGroup
			for i := 0; i < 200; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					rec := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{
						"CF-Connecting-IP": tc.cfOf(i),
					}))
					switch rec.Code {
					case http.StatusUnauthorized:
						evaluated.Add(1)
					case http.StatusTooManyRequests:
						refused.Add(1)
					default:
						t.Errorf("attempt %d: code=%d, want 401 or 429", i, rec.Code)
					}
				}(i)
			}
			wg.Wait()
			if got := int(evaluated.Load()); got > tc.cap {
				t.Fatalf("%d passwords checked, want at most %d", got, tc.cap)
			}
			if evaluated.Load()+refused.Load() != 200 {
				t.Fatalf("answers: %d evaluated + %d refused, want 200", evaluated.Load(), refused.Load())
			}
		})
	}
}

// T1-1 (3): wrong passwords below the cap never block the correct one.
func TestLoginAcceptsACorrectPasswordAfterFourWrongOnes(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	head := map[string]string{"CF-Connecting-IP": "198.51.100.30"}
	for i := 0; i < auth.LoginPerIPMax-1; i++ {
		if rec := send(h, loginPost(wrongPassword, addrTunnel, head)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: code=%d, want 401", i, rec.Code)
		}
	}
	rec := send(h, loginPost(testPassword, addrTunnel, head))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("correct password after %d wrong ones: code=%d loc=%q, want 302 -> /",
			auth.LoginPerIPMax-1, rec.Code, rec.Header().Get("Location"))
	}
	if cookieNamed(rec, sessionCookie) == nil {
		t.Fatal("the correct password set no session cookie")
	}
}

// T1-1 (4): a correct password gives its charge back, so it leaves nothing behind.
func TestSuccessfulLoginLeavesNoEntryInAnyBudget(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	a, h := newServer(s, nil, cfg)
	rec := send(h, loginPost(testPassword, addrTunnel,
		map[string]string{"CF-Connecting-IP": "198.51.100.31"}))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	public, addresses, devices := a.login.Counts()
	if public != 0 || addresses != 0 || devices != 0 {
		t.Fatalf("after a correct password: public=%d addresses=%d devices=%d, want 0 0 0", public, addresses, devices)
	}
}

// T1-2: a stranger who fills the public budget cannot keep the owner out. The
// owner path is a loopback connection with no forwarding header.
func TestAFullPublicBudgetNeverLocksOutTheOwnerPath(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "1") // the owner lane exists only with the opt-in
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	fillPublicBudget(t, h)

	// The budget is full: one more public attempt never reaches the password check.
	full := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{"CF-Connecting-IP": "198.51.100.200"}))
	if full.Code != http.StatusTooManyRequests {
		t.Fatalf("public attempt with a full budget: code=%d, want 429", full.Code)
	}

	// (b) a correct password that arrives through the tunnel still gets 429.
	password := testPassword
	tun := send(h, loginPost(password, addrTunnel, map[string]string{"CF-Connecting-IP": "198.51.100.201"}))
	if tun.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password from the tunnel with a full budget: code=%d, want 429", tun.Code)
	}

	// (a) the same password on the owner path signs in.
	own := send(h, loginPost(password, addrLoopback, nil))
	if own.Code != http.StatusFound || own.Header().Get("Location") != "/" {
		t.Fatalf("owner path with a full budget: code=%d loc=%q, want 302 -> /", own.Code, own.Header().Get("Location"))
	}
	if cookieNamed(own, sessionCookie) == nil {
		t.Fatal("the owner path set no session cookie")
	}
}

// The device lane: a browser that signed in before keeps its own budget, so a
// full public budget does not reach it. A forged cookie names no device.
func TestDeviceCookieSignsInWhileThePublicBudgetIsFull(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	fillPublicBudget(t, h)
	password := testPassword
	head := map[string]string{"CF-Connecting-IP": "198.51.100.210"}

	forged := &http.Cookie{Name: auth.DeviceCookie, Value: "0123456789abcdef.not-a-real-signature"}
	if rec := send(h, loginPost(password, addrTunnel, head, forged)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged device cookie: code=%d, want 429 (the public lane)", rec.Code)
	}

	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	rec := send(h, loginPost(password, addrTunnel, head, known))
	if rec.Code != http.StatusFound {
		t.Fatalf("known device with a full public budget: code=%d, want 302", rec.Code)
	}
	if cookieNamed(rec, sessionCookie) == nil {
		t.Fatal("the device lane set no session cookie")
	}
}

func TestSuccessfulLoginSetsADeviceCookie(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	rec := send(h, loginPost(testPassword, addrLoopback, nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	ck := cookieNamed(rec, auth.DeviceCookie)
	if ck == nil {
		t.Fatal("login set no device cookie")
	}
	if _, ok := cfg.DeviceID(ck.Value); !ok {
		t.Errorf("the device cookie %q does not carry a valid signature", ck.Value)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.MaxAge != deviceMaxAge {
		t.Errorf("device cookie flags: HttpOnly=%v SameSite=%v MaxAge=%d, want true Strict %d",
			ck.HttpOnly, ck.SameSite, ck.MaxAge, deviceMaxAge)
	}
	// A plain http request over loopback must not ask for Secure, or the
	// browser drops the cookie.
	if ck.Secure {
		t.Error("device cookie is Secure on a plain loopback request")
	}
}

// A browser keeps the device it already has, so its budget cannot be reset by
// signing in again.
func TestLoginKeepsAKnownDeviceCookie(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	rec := send(h, loginPost(testPassword, addrLoopback, nil, known))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	if ck := cookieNamed(rec, auth.DeviceCookie); ck == nil || ck.Value != known.Value {
		t.Fatalf("device cookie changed on sign-in: %v", ck)
	}
}

// One attempt is charged to one lane only: a device pays from its own budget.
func TestDeviceLaneHasItsOwnBudget(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	a, h := newServer(s, nil, cfg)
	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	head := map[string]string{"CF-Connecting-IP": "198.51.100.220"}
	for i := 0; i < auth.LoginDeviceMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrTunnel, head, known)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("device attempt %d: code=%d, want 401", i, rec.Code)
		}
	}
	if rec := send(h, loginPost(wrongPassword, addrTunnel, head, known)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d from one device: code=%d, want 429", auth.LoginDeviceMax+1, rec.Code)
	}
	public, addresses, devices := a.login.Counts()
	if public != 0 || addresses != 0 {
		t.Errorf("device failures reached another budget: public=%d addresses=%d, want 0 0", public, addresses)
	}
	if devices != 1 {
		t.Errorf("device budgets=%d, want 1", devices)
	}
}

// A sign-in body carries one password field. Anything larger than the cap is
// refused before the form is read.
func TestLoginBodyIsCapped(t *testing.T) {
	s := openStore(t)
	h := NewWithAuth(s, nil, authConfig())
	body := "password=" + strings.Repeat("1", loginBodyMax+1)
	r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	r.Host = "127.0.0.1:20130"
	r.RemoteAddr = addrLoopback
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := send(h, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized login body: code=%d, want 400", rec.Code)
	}
}

// C2 (round 2): without the opt-in, a loopback caller with no forwarding header
// is public. A proxy that adds no header makes every internet caller look local.
// The lane decision itself is tested in internal/auth.
func TestLoopbackWithoutHeaderIsPublicByDefault(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "")
	h := NewWithAuth(openStore(t), nil, authConfig())
	for i := 1; i <= auth.LoginPerIPMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i, rec.Code)
		}
	}
	if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("wrong password %d: status %d, want 429", auth.LoginPerIPMax+1, rec.Code)
	}
}

// C2 (round 2): CCW_OWNER_LOOPBACK=1 gives the owner's unmetered lane back.
func TestOwnerLoopbackOptInRestoresLocalLane(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "1")
	h := NewWithAuth(openStore(t), nil, authConfig())
	for i := 1; i <= 3*auth.LoginPerIPMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i, rec.Code)
		}
	}
}
