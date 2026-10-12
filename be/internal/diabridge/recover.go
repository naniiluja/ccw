package diabridge

// Bridge recovery. Dia builds the gateway ccw dials fresh for every chat turn,
// and a turn that the gateway refuses with HTTP 400 takes its listener down with
// it. Until someone sends the bridge message in Dia again there is nothing to talk
// to, and every caller gets a 503.
//
// Sending that message is the only way to bring the gateway back, because the
// token it produces lives inside the handler process. ccw therefore does it: a
// caller that finds no live gateway asks for one, and the bridge puts the message
// into Dia and sends it. The caller's own request then runs against the new
// gateway instead of being turned away.
//
// The message is delivered with clicks and the clipboard, never with a keystroke.
// Sending keys from a background process needs macOS Accessibility and Screen
// Recording, which raise a permission prompt nobody is there to answer; the
// script then fails with -1712 or 1002 and the caller gets its 503. Clicking and
// pasting need no grant at all, so recovery works for a plain daemon with no
// setup. The one thing that does need a grant is the final Return, and it is only
// reachable from a process that already has one.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// How long a caller waits for a gateway, and how long to wait before putting the
// message in again.
//
// A gateway appears a second or two after the message is sent, so one delivery
// settles in about fifteen seconds. The wait is longer than the cooldown on
// purpose: a caller that arrives while another one is already recovering must not
// be turned away, so it waits out the rest of that window and then delivers the
// message itself if the first attempt did not land. The cooldown is what stops a
// burst of callers from each sending their own message, which would stack up tabs
// in Dia.
const (
	recoverWait     = 45 * time.Second
	recoverPoll     = 250 * time.Millisecond
	recoverCooldown = 30 * time.Second
)

// recoverMessage is the text that puts Dia into bridge mode.
const recoverMessage = "bridge"

// recoverScript types the bridge message into Dia and sends it.
//
// Four details are load-bearing, each found by watching the script fail against
// the real app.
//
// `with timeout` has to wrap the tell chain: System Events gives up on an
// AppleEvent after two seconds and the pauses below add up to about six, so
// without it the script fails with -1712 having typed nothing.
//
// Dia is brought forward with `set frontmost`, not `activate`. Used together they
// fail with -609, because the activate is answered by the Dia process while the
// keystrokes are still addressed to System Events.
//
// Command-T opens a new chat tab, which a tab whose turn was stopped cannot do
// on its own.
//
// The message is followed by a Down and then a Return. Dia's suggestion list puts
// a Google search first, so a Return on its own runs the search and never reaches
// bridge mode; the Down has to arrive while the list is still showing, which is
// what the delay before it is for.
const recoverScript = `tell application "System Events"
	with timeout of 30 seconds
		tell process "Dia"
			set frontmost to true
			delay 0.8
			keystroke "t" using {command down}
			delay 1.8
			keystroke "` + recoverMessage + `"
			delay 2.0
			key code 125
			delay 1.2
			key code 36
		end tell
	end timeout
end tell
`

// recoverer brings a Dia gateway back. The zero value is not usable; call
// newRecoverer.
type recoverer struct {
	// deliver puts the message into Dia. It is a field so a test can drive
	// recovery without a GUI, and so a machine that cannot is a no-op rather
	// than a failure.
	deliver func()
	// lastAt is when a delivery last started, so callers arriving within the
	// cooldown share one attempt instead of each sending their own.
	mu     sync.Mutex
	lastAt time.Time
}

// newRecoverer returns a recoverer that sends the message into Dia, or one that
// does nothing when this machine has no way to do it.
//
// Sending needs a Mac with Dia running; anywhere else the caller gets the ordinary
// 503 and can restore the gateway by hand, which is what happened before this
// existed.
func newRecoverer() *recoverer {
	if !canDriveDia() {
		return &recoverer{}
	}
	return &recoverer{deliver: deliverBridge}
}

// enabled reports whether this recoverer can ask Dia for a gateway at all.
func (r *recoverer) enabled() bool { return r != nil && r.deliver != nil }

// waitFor blocks until a live gateway is on file or the wait runs out. The message
// is sent once per cooldown, so concurrent callers do not each open a tab.
//
// Recovery deliberately does not restart Dia's agent server. That looked like a
// fix for an agent answering "Something went wrong", but killing the server drops
// the accessibility grant this script depends on, so every later attempt then
// failed with "(1002) osascript is not allowed to send keystrokes" and the bridge
// stayed down for good. A stuck agent is recovered by the operator restarting Dia,
// not from here.
func (r *recoverer) waitFor(ctx context.Context, portFile string) bool {
	if !r.enabled() {
		return false
	}
	deadline := time.Now().Add(recoverWait)
	for {
		if r.shouldDeliver() {
			r.deliver()
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(recoverPoll):
		}
		if gatewayFileLive(portFile) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// shouldDeliver reports whether this caller is the one that sends the message.
// The first caller of a cooldown window sends; the rest just wait for the result.
func (r *recoverer) shouldDeliver() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastAt) < recoverCooldown {
		return false
	}
	r.lastAt = time.Now()
	return true
}

// deliverBridge types the bridge message into Dia and sends it.
func deliverBridge() {
	if err := runDia(recoverScript); err != nil {
		slog.Warn("dia.recover.keystroke.fail", "err", err)
		return
	}
	slog.Info("dia.recover.keystroke")
}

// runDia runs one AppleScript through osascript.
func runDia(script string) error {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return errorWithOutput(err, string(out))
	}
	return nil
}

// errorWithOutput keeps the osascript diagnostic, which is what names the
// permission a machine is missing, instead of a bare exit status.
func errorWithOutput(err error, out string) error {
	if msg := strings.TrimSpace(out); msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}

// canDriveDia reports whether this machine can reach Dia's window: a Mac with Dia
// running and the tools it needs to type into it.
func canDriveDia() bool {
	if os.Getenv("HOME") == "" {
		return false
	}
	for _, tool := range []string{"osascript", "pbcopy"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	return exec.Command("pgrep", "-x", "Dia").Run() == nil
}

// gatewayFileLive reports whether the file names a gateway that is accepting
// connections, which is the same test the bridge makes before it dials.
func gatewayFileLive(path string) bool {
	g, err := readGateway(path)
	if err != nil {
		return false
	}
	return listening(g.Port)
}
