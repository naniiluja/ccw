package auth

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// The login takes a password of at least MinPasswordLength characters and
// nothing else. The public budget admits at most LoginPublicMax wrong passwords
// per loginPublicWin: 100 a day, so 3000 in 30 days for a caller that rotates
// its address, against at least 26^12 candidates for the weakest accepted
// password. Every attempt is charged before the password is checked, so a
// concurrent burst cannot pass the cap.
const (
	LoginPerIPMax  = 5                // wrong passwords from one public address ...
	loginPerIPWin  = 15 * time.Minute // ... inside this window
	LoginDeviceMax = 10               // wrong passwords from one known browser ...
	loginDeviceWin = 24 * time.Hour   // ... inside this window
	LoginPublicMax = 100              // wrong passwords from all public addresses ...
	loginPublicWin = 24 * time.Hour   // ... inside this rolling window
)

// DeviceCookie marks a browser that signed in before. It grants no access.
const DeviceCookie = "ccw_device"

// loginLane is the budget that pays for an attempt. An attempt has exactly one.
type loginLane int

const (
	// laneLocal is a client on this machine that sends no forwarding header, and
	// only when CCW_OWNER_LOOPBACK=1. It is off by default: a front proxy that
	// adds no forwarding header makes every internet caller look like this one.
	laneLocal loginLane = iota
	// laneDevice is a browser that signed in before and kept its cookie.
	laneDevice
	// lanePublic is everything else.
	lanePublic
)

// LoginSource names the one budget an attempt is charged to.
type LoginSource struct {
	lane   loginLane
	ip     string // public lane only
	device string // device lane only
}

// LoginGuard limits wrong passwords per lane: per public address, per known
// browser and across all public addresses. It is safe for concurrent use.
type LoginGuard struct {
	mu     sync.Mutex
	perIP  map[string][]time.Time // wrong passwords per public address
	device map[string][]time.Time // wrong passwords per known browser
	public []time.Time            // wrong passwords from all public addresses
	// ownerLoopback enables laneLocal. Set it only when every proxy in front of
	// ccw names the caller in a forwarding header.
	ownerLoopback bool
}

// NewLoginGuard returns an empty guard. CCW_OWNER_LOOPBACK=1 enables the
// unmetered owner lane.
func NewLoginGuard() *LoginGuard {
	return &LoginGuard{perIP: map[string][]time.Time{}, device: map[string][]time.Time{},
		ownerLoopback: os.Getenv("CCW_OWNER_LOOPBACK") == "1"}
}

// OwnerLoopback reports whether the owner lane is enabled, for LoginSourceOf.
func (g *LoginGuard) OwnerLoopback() bool { return g.ownerLoopback }

// Counts returns how many charges each budget holds: the public window, the
// number of tracked addresses and the number of tracked browsers. It reads
// only, so callers and tests can check that a budget was charged or refunded.
func (g *LoginGuard) Counts() (public, addresses, devices int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.public), len(g.perIP), len(g.device)
}

// Reserve charges one attempt to the source's budget BEFORE the password is
// checked. The charge and the test happen under one lock, so a burst of
// requests cannot get more password checks than the cap. A correct password
// gives the charge back with Refund. When the answer is false, the duration is
// the wait before a retry.
func (g *LoginGuard) Reserve(src LoginSource, now time.Time) (bool, time.Duration) {
	if src.lane == laneLocal {
		return true, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if src.lane == laneDevice {
		win := trimWindow(g.device[src.device], now, loginDeviceWin)
		if len(win) >= LoginDeviceMax {
			g.device[src.device] = win
			return false, windowWait(win, now, loginDeviceWin)
		}
		g.device[src.device] = append(win, now)
		return true, 0
	}
	perIP := trimWindow(g.perIP[src.ip], now, loginPerIPWin)
	if len(perIP) >= LoginPerIPMax {
		g.store(src.ip, perIP)
		return false, windowWait(perIP, now, loginPerIPWin)
	}
	public := trimWindow(g.public, now, loginPublicWin)
	if len(public) >= LoginPublicMax {
		g.public = public
		g.store(src.ip, perIP)
		return false, windowWait(public, now, loginPublicWin)
	}
	g.perIP[src.ip] = append(perIP, now)
	g.public = append(public, now)
	return true, 0
}

// Refund gives back the charge of a correct password. The public budget gives back
// one charge and not the whole window, so a stranger's failures stay counted.
func (g *LoginGuard) Refund(src LoginSource) {
	if src.lane == laneLocal {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if src.lane == laneDevice {
		delete(g.device, src.device)
		return
	}
	delete(g.perIP, src.ip)
	if n := len(g.public); n > 0 {
		g.public = g.public[:n-1]
	}
}

// store keeps an address's window, and drops the key when the window is empty
// so a source that stopped trying does not stay in the map. The caller holds
// the lock.
func (g *LoginGuard) store(ip string, win []time.Time) {
	if len(win) == 0 {
		delete(g.perIP, ip)
		return
	}
	g.perIP[ip] = win
}

// trimWindow drops the attempts that left the window.
func trimWindow(times []time.Time, now time.Time, win time.Duration) []time.Time {
	cut := now.Add(-win)
	i := 0
	for i < len(times) && !times[i].After(cut) {
		i++
	}
	return times[i:]
}

// windowWait is the time until the oldest attempt in a full window expires.
func windowWait(times []time.Time, now time.Time, win time.Duration) time.Duration {
	if len(times) == 0 {
		return win
	}
	if d := times[0].Add(win).Sub(now); d > 0 {
		return d
	}
	return time.Second
}

// LoginSourceOf decides which budget pays for an attempt. It reads the
// connection, the presence of the forwarding headers and the device cookie's
// signature. It never reads the password and never trusts a header's value, so a
// stranger cannot take the owner's lane and the owner cannot lose it.
func LoginSourceOf(r *http.Request, deviceID func(string) (string, bool), ownerLoopback bool) LoginSource {
	forwarded := r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("X-Forwarded-For") != ""
	if ownerLoopback && !forwarded && loopbackAddr(remoteHost(r)) {
		return LoginSource{lane: laneLocal}
	}
	if deviceID != nil {
		if ck, err := r.Cookie(DeviceCookie); err == nil {
			if id, ok := deviceID(ck.Value); ok {
				return LoginSource{lane: laneDevice, device: id}
			}
		}
	}
	return LoginSource{lane: lanePublic, ip: clientIP(r)}
}

// clientIP names the public source of a request. Behind the Cloudflare tunnel
// the real address is in CF-Connecting-IP, and cloudflared connects from this
// machine, so a forwarding header counts only on a loopback connection. A
// caller that reaches the port directly cannot choose its own bucket.
func clientIP(r *http.Request) string {
	addr := remoteHost(r)
	if !loopbackAddr(addr) {
		return addr
	}
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if first = strings.TrimSpace(first); first != "" {
			return first
		}
	}
	return addr
}

// remoteHost is the address of the connection, without its port.
func remoteHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// loopbackAddr reports whether a connection address is on this machine. It
// reads an address, not a Host header, so it accepts the whole 127.0.0.0/8
// range. Use httpapi's loopbackHost for a Host header, which answers another
// question.
func loopbackAddr(host string) bool {
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}
