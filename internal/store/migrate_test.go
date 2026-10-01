package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	return v
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	return n == 1
}

// A database written before the schema was versioned has user_version 0 and
// some of the newer columns already. Opening it must add the missing ones,
// keep every row and mark the learned "{*}" paths exactly as the old code did.
func TestOpenUpgradesAnOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	for _, q := range []string{
		`CREATE TABLE connections (
			id TEXT PRIMARY KEY, provider TEXT NOT NULL, label TEXT NOT NULL, secret TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			refresh_token TEXT NOT NULL DEFAULT '', token_url TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO connections (id, provider, label, secret, created_at, updated_at, refresh_token)
			VALUES ('c1', 'groq', 'main', 'placeholder-secret', 'now', 'now', 'placeholder-refresh')`,
		`CREATE TABLE upstream_errors (
			id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, provider TEXT NOT NULL,
			connection TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', client TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '', status INTEGER NOT NULL, latency_ms INTEGER NOT NULL DEFAULT 0,
			class TEXT NOT NULL DEFAULT '', signature TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
			quota_left REAL NOT NULL DEFAULT -1, headers TEXT NOT NULL DEFAULT '', resp_body TEXT NOT NULL DEFAULT '',
			req_body TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO upstream_errors (at, provider, client, status, message)
			VALUES ('2026-01-01T00:00:00Z', 'groq', 'old-key', 500, 'old row')`,
		`CREATE TABLE shape_fields (
			key TEXT NOT NULL, path TEXT NOT NULL, type TEXT NOT NULL, seen INTEGER NOT NULL DEFAULT 0,
			first_obs INTEGER NOT NULL, last_obs INTEGER NOT NULL, gone INTEGER NOT NULL DEFAULT 0,
			last_at TEXT NOT NULL DEFAULT '', PRIMARY KEY (key, path))`,
		`INSERT INTO shape_fields (key, path, type, first_obs, last_obs) VALUES
			('k', 'a.{*}.b', 'string', 1, 1), ('k', 'a.b', 'string', 1, 1)`,
		`CREATE TABLE shape_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, direction TEXT NOT NULL, provider TEXT NOT NULL,
			endpoint TEXT NOT NULL, event TEXT NOT NULL DEFAULT '', path TEXT NOT NULL, kind TEXT NOT NULL,
			old_type TEXT NOT NULL DEFAULT '', new_type TEXT NOT NULL DEFAULT '', sample TEXT NOT NULL DEFAULT '',
			acked INTEGER NOT NULL DEFAULT 0, client_key_id TEXT NOT NULL DEFAULT '', verdict TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, key TEXT NOT NULL UNIQUE, enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL)`,
		`CREATE TABLE provider_models (
			provider TEXT NOT NULL, model TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1,
			stale INTEGER NOT NULL DEFAULT 0, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL,
			test_at TEXT NOT NULL DEFAULT '', test_ok INTEGER NOT NULL DEFAULT 0, test_ms INTEGER NOT NULL DEFAULT 0,
			test_msg TEXT NOT NULL DEFAULT '', PRIMARY KEY (provider, model))`,
		`CREATE TABLE error_verdicts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, provider TEXT NOT NULL, signature TEXT NOT NULL,
			action TEXT NOT NULL, applied INTEGER NOT NULL DEFAULT 0, verified INTEGER NOT NULL DEFAULT 0,
			detail TEXT NOT NULL DEFAULT '', cause TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '', by_model TEXT NOT NULL DEFAULT '', errors INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("old schema: %v\n%s", err, q)
		}
	}
	if v := userVersion(t, db); v != 0 {
		t.Fatalf("old database user_version = %d, want 0", v)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if v := userVersion(t, s.DB); v != len(migrations) {
		t.Errorf("user_version = %d, want %d", v, len(migrations))
	}
	for table, cols := range map[string][]string{
		"connections":     {"refresh_token", "token_url", "client_id", "client_secret", "expires_at", "base_url", "meta", "standby"},
		"upstream_errors": {"client_key_id"},
		"shape_fields":    {"legacy"},
		"shape_changes":   {"client", "client_key_id", "verdict", "verdict_conf", "verdict_at", "auto_acked", "verdict_by", "verdict_note", "resolved"},
		"api_keys":        {"models", "expires_at", "rpm", "last_used"},
		"provider_models": {"test_conn"},
		"error_verdicts":  {"replayed"},
	} {
		for _, c := range cols {
			if !hasColumn(t, s.DB, table, c) {
				t.Errorf("%s.%s missing after upgrade", table, c)
			}
		}
	}

	c, err := s.OAuth("c1")
	if err != nil || c.RefreshToken != "placeholder-refresh" {
		t.Errorf("connection row: %+v, err = %v", c, err)
	}
	list, err := s.ListUpstreamErrors(ErrorFilter{})
	if err != nil || len(list) != 1 || list[0].Message != "old row" {
		t.Errorf("upstream_errors rows: %+v, err = %v", list, err)
	}
	_, fields, err := s.LoadShapes()
	if err != nil || len(fields) != 2 {
		t.Fatalf("shape fields: %+v, err = %v", fields, err)
	}
	for _, f := range fields {
		if want := f.Path == "a.{*}.b"; f.Legacy != want {
			t.Errorf("shape field %q legacy = %v, want %v", f.Path, f.Legacy, want)
		}
	}
}

// Opening the same database twice runs no migration the second time.
func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.db")
	for i := range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		if v := userVersion(t, s.DB); v != len(migrations) {
			t.Errorf("open %d: user_version = %d, want %d", i+1, v, len(migrations))
		}
		s.Close()
	}

	// A migration that would fail if it ran proves the second open skips it.
	ran := false
	failing := append(append([]migration{}, migrations[:len(migrations)-1]...), func(*sql.Tx) error {
		ran = true
		return errors.New("must not run")
	})
	s, err := openWith(path, failing)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s.Close()
	if ran {
		t.Error("an applied migration ran again")
	}
}

// A migration that fails halfway leaves user_version and the schema as they
// were, and the database still opens at the old version.
func TestFailedMigrationKeepsTheOldVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fail.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()

	failing := append(append([]migration{}, migrations...), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`ALTER TABLE connections ADD COLUMN half_done TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if _, err := openWith(path, failing); err == nil {
		t.Fatal("openWith with a failing migration succeeded")
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if v := userVersion(t, s.DB); v != len(migrations) {
		t.Errorf("user_version = %d, want %d", v, len(migrations))
	}
	if hasColumn(t, s.DB, "connections", "half_done") {
		t.Error("the failed migration's column survived the rollback")
	}
}
