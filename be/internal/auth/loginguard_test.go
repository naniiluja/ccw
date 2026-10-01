package auth

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	addrLoopback = "127.0.0.1:54321"
	addrTunnel   = "127.0.0.1:54322" // cloudflared connects from this machine
	addrPublic   = "203.0.113.9:443"
)

// T1-1 (1) and T7-5 (b): the public budget bounds how many passwords a caller that
// rotates its address can have evaluated. The numbers come from the constants.
func TestLoginBruteForceBoundIsBelowOnePercent(t *testing.T) {
	const span = 30 * 24 * time.Hour
	windows := int(span / loginPublicWin)
	maxChecks := windows * LoginPublicMax
	// The weakest accepted password is the minimum length drawn from the 26
	// lowercase letters, so one guess matches with 1 of 26^MinPasswordLength.
	perGuess := math.Pow(26, -float64(MinPasswordLength))
	if maxChecks > 3000 {
		t.Errorf("%d evaluated passwords in 30 days, want at most 3000", maxChecks)
	}
	if p := float64(maxChecks) * perGuess; p >= 0.01 {
		t.Errorf("%d passwords in 30 days gives p=%.4g, want below 0.01", maxChecks, p)
	}
}

// T1-1 (2) and T7-5: the charge and the test happen under one lock, so a burst
// cannot go past the cap.
func TestLoginGuardReserveIsAtomicUnderLoad(t *testing.T) {
	for _, tc := range []struct {
		name string
		ipOf func(i int) string
		want int
	}{
		{"rotating addresses stop at the public cap", func(i int) string { return fmt.Sprintf("198.51.100.%d", i) }, LoginPublicMax},
		{"one address stops at its own cap", func(int) string { return "198.51.100.7" }, LoginPerIPMax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewLoginGuard()
			now := time.Now()
			var admitted atomic.Int64
			var wg sync.WaitGroup
			for i := 0; i < 200; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					if ok, _ := g.Reserve(LoginSource{lane: lanePublic, ip: tc.ipOf(i)}, now); ok {
						admitted.Add(1)
					}
				}(i)
			}
			wg.Wait()
			if got := int(admitted.Load()); got != tc.want {
				t.Fatalf("admitted %d of 200 attempts, want %d", got, tc.want)
			}
		})
	}
}

// The owner path is never charged, so nothing an attacker does can fill it.
func TestLocalLaneIsNeverCharged(t *testing.T) {
	g := NewLoginGuard()
	now := time.Now()
	for i := 0; i < 1000; i++ {
		if ok, _ := g.Reserve(LoginSource{lane: laneLocal}, now); !ok {
			t.Fatalf("the local lane refused attempt %d", i)
		}
	}
	if public, addresses, devices := g.Counts(); public != 0 || addresses != 0 || devices != 0 {
		t.Fatalf("local attempts were charged: public=%d addresses=%d devices=%d", public, addresses, devices)
	}
}

func TestLoginGuardBlocksOneAddressAndClearsItsWindow(t *testing.T) {
	g := NewLoginGuard()
	now := time.Now()
	src := LoginSource{lane: lanePublic, ip: "198.51.100.40"}
	for i := 0; i < LoginPerIPMax; i++ {
		if ok, _ := g.Reserve(src, now); !ok {
			t.Fatalf("attempt %d refused too early", i)
		}
	}
	ok, wait := g.Reserve(src, now)
	if ok || wait <= 0 {
		t.Fatalf("address not blocked after %d failures: ok=%v wait=%v", LoginPerIPMax, ok, wait)
	}
	// Another address still has its own budget.
	if ok, _ := g.Reserve(LoginSource{lane: lanePublic, ip: "198.51.100.41"}, now); !ok {
		t.Fatal("a second address was blocked by the first's failures")
	}
	// The window clears with time.
	if ok, _ := g.Reserve(src, now.Add(loginPerIPWin+time.Second)); !ok {
		t.Fatal("the per-address window did not clear")
	}
}

func TestPublicWindowClearsAfterItsSpan(t *testing.T) {
	g := NewLoginGuard()
	now := time.Now()
	for i := 0; i < LoginPublicMax; i++ {
		g.Reserve(LoginSource{lane: lanePublic, ip: fmt.Sprintf("198.51.100.%d", i)}, now)
	}
	fresh := LoginSource{lane: lanePublic, ip: "203.0.113.5"}
	if ok, _ := g.Reserve(fresh, now); ok {
		t.Fatal("the public budget admitted an attempt past its cap")
	}
	if ok, _ := g.Reserve(fresh, now.Add(loginPublicWin+time.Second)); !ok {
		t.Fatal("the public window did not clear after its span")
	}
}

// The refund gives back one charge, never the whole budget: a stranger's
// failures stay counted after the owner signs in.
func TestRefundReturnsOnlyItsOwnCharge(t *testing.T) {
	g := NewLoginGuard()
	now := time.Now()
	for i := 0; i < 3; i++ {
		g.Reserve(LoginSource{lane: lanePublic, ip: fmt.Sprintf("198.51.100.%d", i)}, now)
	}
	mine := LoginSource{lane: lanePublic, ip: "203.0.113.6"}
	if ok, _ := g.Reserve(mine, now); !ok {
		t.Fatal("reserve refused a free budget")
	}
	g.Refund(mine)
	if public, addresses, _ := g.Counts(); public != 3 || addresses != 3 {
		t.Fatalf("after a refund: public=%d addresses=%d, want 3 3", public, addresses)
	}
}

func TestLoginSourceOfPicksOneLane(t *testing.T) {
	cfg := newTestConfig(t)
	device := cfg.IssueDevice()
	id, _ := cfg.DeviceID(device)

	for _, tc := range []struct {
		name       string
		remoteAddr string
		header     map[string]string
		cookie     string
		want       LoginSource
	}{
		{"loopback with no header is the owner path", addrLoopback, nil, "", LoginSource{lane: laneLocal}},
		{"ipv6 loopback is the owner path", "[::1]:41000", nil, "", LoginSource{lane: laneLocal}},
		{"a tunnel request is public", addrTunnel, map[string]string{"CF-Connecting-IP": "9.9.9.9"}, "",
			LoginSource{lane: lanePublic, ip: "9.9.9.9"}},
		{"a forwarded header alone takes the owner path away", addrTunnel,
			map[string]string{"X-Forwarded-For": "9.9.9.8"}, "", LoginSource{lane: lanePublic, ip: "9.9.9.8"}},
		{"a direct remote caller is public", addrPublic, nil, "", LoginSource{lane: lanePublic, ip: "203.0.113.9"}},
		{"a known device gets its own lane", addrTunnel, map[string]string{"CF-Connecting-IP": "9.9.9.9"}, device,
			LoginSource{lane: laneDevice, device: id}},
		{"a forged device cookie is public", addrTunnel, map[string]string{"CF-Connecting-IP": "9.9.9.9"}, "abc.def",
			LoginSource{lane: lanePublic, ip: "9.9.9.9"}},
		{"a device cookie does not move the owner path", addrLoopback, nil, device, LoginSource{lane: laneLocal}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tc.remoteAddr
			for k, v := range tc.header {
				r.Header.Set(k, v)
			}
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: DeviceCookie, Value: tc.cookie})
			}
			if got := LoginSourceOf(r, cfg.DeviceID, true); got != tc.want {
				t.Errorf("LoginSourceOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// C2 (round 2): without the opt-in, a loopback caller with no forwarding header
// is public. A proxy that adds no header makes every internet caller look local.
func TestLoginSourceOfIsPublicWithoutTheOwnerOptIn(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = addrLoopback
	if got := LoginSourceOf(r, nil, false); got.lane != lanePublic {
		t.Fatalf("lane = %v, want lanePublic", got.lane)
	}
}

// T8-4: which address a public attempt is charged to. A forwarding header is
// read only from a loopback connection, because only the tunnel writes it.
func TestClientIPPrefersCFThenXFFFromLoopbackOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		remoteAddr string
		cf, xff    string
		want       string
	}{
		{"CF wins over XFF", addrLoopback, "1.1.1.1", "2.2.2.2", "1.1.1.1"},
		{"the first XFF entry, trimmed", addrLoopback, "", " 2.2.2.2 , 3.3.3.3", "2.2.2.2"},
		{"one XFF entry", addrLoopback, "", "4.4.4.4", "4.4.4.4"},
		{"no header is the connection", "5.5.5.5:1234", "", "", "5.5.5.5"},
		{"ipv6 loopback", "[::1]:80", "", "", "::1"},
		{"a remote caller cannot name itself with CF", "5.5.5.5:1234", "1.1.1.1", "", "5.5.5.5"},
		{"a remote caller cannot name itself with XFF", "5.5.5.5:1234", "", "2.2.2.2", "5.5.5.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.cf != "" {
				r.Header.Set("CF-Connecting-IP", tc.cf)
			}
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
