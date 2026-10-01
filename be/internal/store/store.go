// Package store keeps provider credentials in a local SQLite database.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// Store owns the database handle.
type Store struct {
	DB *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS connections (
	id          TEXT PRIMARY KEY,
	provider    TEXT NOT NULL,
	label       TEXT NOT NULL,
	secret      TEXT NOT NULL,
	is_active   INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS usage_daily (
	day           TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	model         TEXT NOT NULL,
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	requests      INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (day, connection_id, model)
);`

// Open opens the database at path and applies the schema.
// WAL keeps a reader from blocking the writer, which matters because the
// dashboard reads while requests are being served.
func Open(path string) (*Store, error) { return openWith(path, migrations) }

// openWith is Open with the migration list as a parameter, so a test can run
// a list that fails on purpose.
func openWith(path string, list []migration) (*Store, error) {
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open database file: %w", err)
		}
		_ = f.Close()
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("chmod database file: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_sync=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	for _, sc := range []struct{ name, sql string }{
		{"schema", schema}, {"models schema", modelsSchema}, {"errors schema", errorsSchema},
		{"error verdicts schema", errorVerdictsSchema}, {"provider defs schema", providerDefsSchema},
		{"pending schema", pendingSchema}, {"drift schema", driftSchema}, {"api key schema", apiKeySchema},
		{"filter schema", filterSchema},
	} {
		if _, err := db.Exec(sc.sql); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %s: %w", sc.name, err)
		}
	}
	if err := migrate(db, list); err != nil {
		db.Close()
		return nil, err
	}
	st := &Store{DB: db}
	if err := st.seedFilters(); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed filters: %w", err)
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		for _, p := range []string{path + "-wal", path + "-shm"} {
			if _, err := os.Stat(p); err == nil {
				if err := os.Chmod(p, 0o600); err != nil {
					db.Close()
					return nil, fmt.Errorf("chmod database sidecar: %w", err)
				}
			}
		}
	}
	return st, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.DB.Close() }

// migration is one numbered step of the schema. Step i (counting from 1) runs
// when PRAGMA user_version is below i, inside its own transaction together
// with the write of the new version, so a failed step changes nothing.
type migration func(tx *sql.Tx) error

// column is one column a migration adds. Names and definitions are constants
// of this package, never input, so concatenating them into SQL is safe.
type column struct{ name, def string }

// migrations is the ordered schema history. Append only: the position of a
// step is its version number. Every step skips the columns a table already
// has, because a database written before the schema was versioned has
// user_version 0 and some of these columns already.
var migrations = []migration{
	// 1: the account that answered the last model test.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "provider_models", column{"test_conn", "TEXT NOT NULL DEFAULT ''"})
		return err
	},
	// 2: the dashboard key that sent the failed request.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "upstream_errors", column{"client_key_id", "TEXT NOT NULL DEFAULT ''"})
		return err
	},
	// 3: whether a verdict was replayed first.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "error_verdicts", column{"replayed", "INTEGER NOT NULL DEFAULT 0"})
		return err
	},
	// 4: who sent a shape change and the reviewer's verdict on it.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "shape_changes",
			column{"client", "TEXT NOT NULL DEFAULT ''"},
			column{"client_key_id", "TEXT NOT NULL DEFAULT ''"},
			column{"verdict", "TEXT NOT NULL DEFAULT ''"},
			column{"verdict_conf", "REAL NOT NULL DEFAULT 0"},
			column{"verdict_at", "TEXT NOT NULL DEFAULT ''"},
			column{"auto_acked", "INTEGER NOT NULL DEFAULT 0"},
			column{"verdict_by", "TEXT NOT NULL DEFAULT ''"},
			column{"verdict_note", "TEXT NOT NULL DEFAULT ''"},
			column{"resolved", "INTEGER NOT NULL DEFAULT 0"})
		return err
	},
	// 5: the legacy flag of learned shape fields. Only a table the code before
	// the field-name fix wrote lacks the column, so that is the one place the
	// already learned "{*}" paths are marked.
	func(tx *sql.Tx) error {
		added, err := addColumns(tx, "shape_fields", column{"legacy", "INTEGER NOT NULL DEFAULT 0"})
		if err != nil || added == 0 {
			return err
		}
		if _, err := tx.Exec(`UPDATE shape_fields SET legacy = 1 WHERE path LIKE '%{*}%'`); err != nil {
			return fmt.Errorf("mark learned shape fields: %w", err)
		}
		return nil
	},
	// 6: API key model list, expiry, request cap and last use.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "api_keys",
			column{"models", "TEXT NOT NULL DEFAULT ''"},
			column{"expires_at", "TEXT NOT NULL DEFAULT ''"},
			column{"rpm", "INTEGER NOT NULL DEFAULT 0"},
			column{"last_used", "TEXT NOT NULL DEFAULT ''"})
		return err
	},
	// 7: OAuth refresh configuration and per-connection settings.
	func(tx *sql.Tx) error {
		_, err := addColumns(tx, "connections", oauthColumns...)
		return err
	},
}

// migrate runs the steps of list the database has not applied yet.
func migrate(db *sql.DB, list []migration) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for i := version; i < len(list); i++ {
		if err := runMigration(db, i+1, list[i]); err != nil {
			return fmt.Errorf("migrate schema to version %d: %w", i+1, err)
		}
	}
	return nil
}

// runMigration applies one step and records version in the same transaction.
func runMigration(db *sql.DB, version int, step migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	if err := step(tx); err != nil {
		return err
	}
	// PRAGMA takes no bound parameters; version is an int, not input.
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		return fmt.Errorf("write schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// addColumns adds the columns table has not got and reports how many it added.
func addColumns(tx *sql.Tx, table string, cols ...column) (int, error) {
	have := map[string]bool{}
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return 0, fmt.Errorf("read columns of %s: %w", table, err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan columns of %s: %w", table, err)
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("read columns of %s: %w", table, err)
	}
	rows.Close()
	added := 0
	for _, c := range cols {
		if have[c.name] {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
			return added, fmt.Errorf("add column %s.%s: %w", table, c.name, err)
		}
		added++
	}
	return added, nil
}
