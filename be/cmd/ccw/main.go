// Command ccw serves provider credentials and forwards requests unchanged.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/diabridge"
	"github.com/naniiluja/ccw/internal/httpapi"
	"github.com/naniiluja/ccw/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:20130", "listen address")
	dbPath := flag.String("db", "ccw.db", "path to the database file")
	reset := flag.Bool("reset-password", false, "generate a new sign-in password, print it once, store only its hash, then exit")
	insecure := flag.Bool("insecure-no-auth", false, "start with no sign-in gate; every caller reaches every stored credential")
	diaFront := flag.String("dia-bridge-front", diabridge.DefaultFrontAddr, "Dia bridge front address that Dia's claude calls (empty to disable)")
	diaBack := flag.String("dia-bridge-back", diabridge.DefaultBackAddr, "Dia bridge back address that ccw's dia provider calls")
	flag.Parse()

	// Structured lines on stderr. The log package's output goes through the
	// same handler from here on, log.Fatal included.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	s, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if err := s.SeedKeylessConnections(); err != nil {
		log.Fatalf("seed keyless accounts: %v", err)
	}

	// The password notice goes to stderr as plain text: through log.Writer()
	// it would now be folded into one JSON line.
	if *reset {
		if err := resetPassword(s, os.Stderr); err != nil {
			log.Fatal(err)
		}
		if os.Getenv("CCW_PASSWORD") != "" {
			slog.Warn("auth.password.env_precedence", "detail", "CCW_PASSWORD is set and takes precedence over the stored password")
		}
		return
	}

	noAuth := *insecure || os.Getenv("CCW_INSECURE_NO_AUTH") == "1"
	authCfg, err := setupAuth(s, os.Getenv("CCW_PASSWORD"), noAuth, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}

	// The api is built first so the bridge can share its search service: Dia's
	// gateway refuses the hosted search tool, and the bridge answers a search
	// with the same provider /v1 uses rather than dropping it.
	ccw, handler := httpapi.NewAPI(s, nil, authCfg)

	// Dia's own claude is pointed at the bridge's front address (see
	// scripts/dia-patch.sh), and the dia provider in the registry calls the back
	// address. An empty front address leaves the bridge off while keeping the
	// dia provider's base address intact.
	if *diaFront != "" {
		bridge := diabridge.New()
		bridge.SetSearch(ccw.Searcher())
		if err := bridge.Start(*diaFront, *diaBack); err != nil {
			log.Fatalf("start dia bridge: %v", err)
		}
		defer bridge.Close()
		slog.Info("dia.bridge.listen", "front", bridge.Addr(), "back", bridge.BackAddr())
	}

	srv := newHTTPServer(*addr, handler)
	slog.Info("server.listen", "url", "http://"+*addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// idleTimeout closes a keep-alive connection that has sat idle this long.
const idleTimeout = 120 * time.Second

// newHTTPServer bounds the header read and an idle connection only. A read or
// write deadline would cut a long stream, so WriteTimeout stays unset.
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       idleTimeout,
	}
}

// passwordSetting is the settings key that holds the hash of the sign-in
// password. The plaintext is never stored.
const passwordSetting = "password_hash"

// setupAuth builds the sign-in gate. env is CCW_PASSWORD. With noAuth and no
// password the server starts open, and says so on out. out is the startup log:
// the only secret ever written to it is a password this call generated.
func setupAuth(s *store.Store, env string, noAuth bool, out io.Writer) (*auth.Config, error) {
	var authCfg *auth.Config
	if !noAuth || env != "" {
		hash, err := passwordHash(s, env, out)
		if err != nil {
			return nil, err
		}
		if authCfg, err = auth.FromHash(hash); err != nil {
			return nil, err
		}
	}
	if err := authDecision(authCfg, noAuth); err != nil {
		return nil, err
	}
	if authCfg == nil {
		fmt.Fprintln(out, "WARNING: the sign-in gate is off, the server is not authenticated")
	}
	return authCfg, nil
}

// passwordHash returns the hash the gate checks: the hash of the environment's
// password, else the one this install stored, else the hash of a new password
// that is generated, printed once on out and stored as a hash only.
func passwordHash(s *store.Store, env string, out io.Writer) (string, error) {
	if env != "" {
		if err := auth.ValidatePassword(env); err != nil {
			return "", fmt.Errorf("CCW_PASSWORD: %w", err)
		}
		return auth.HashPassword(env)
	}
	saved, err := s.GetSetting(passwordSetting)
	if err != nil {
		return "", fmt.Errorf("read the password hash: %w", err)
	}
	if saved != "" {
		return saved, nil
	}
	if err := resetPassword(s, out); err != nil {
		return "", err
	}
	return s.GetSetting(passwordSetting)
}

// resetPassword generates a new password, stores only its hash, and prints the
// password once on out. This notice is the one place a secret is written out.
func resetPassword(s *store.Store, out io.Writer) error {
	password := auth.GeneratePassword()
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.SetSetting(passwordSetting, hash); err != nil {
		return fmt.Errorf("save the password hash: %w", err)
	}
	fmt.Fprint(out, passwordNotice(password))
	return nil
}

// passwordNotice tells the owner the new dashboard password. It is shown once.
func passwordNotice(password string) string {
	return "\nDashboard sign-in: this install's password is shown once and only its hash is kept.\n" +
		"  Password: " + password + "\n" +
		"Store it in a password manager. Run `ccw -db <file> -reset-password` to replace it,\n" +
		"or set CCW_PASSWORD (at least 12 characters) to choose your own.\n"
}

// authDecision refuses to start an ungated server. The reference deployment
// binds loopback behind a tunnel, so a loopback bind proves nothing about who
// reaches the port.
func authDecision(authCfg *auth.Config, insecureNoAuth bool) error {
	if authCfg == nil && !insecureNoAuth {
		return errors.New("no sign-in gate: every caller would reach every stored credential. " +
			"Set CCW_PASSWORD or let ccw generate one, or pass -insecure-no-auth (or CCW_INSECURE_NO_AUTH=1) to start with no gate")
	}
	return nil
}
