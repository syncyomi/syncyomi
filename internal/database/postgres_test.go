package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog"
)

// testPostgresDSNEnv enables the Postgres test matrix. When unset (the default),
// every test uses a throwaway SQLite database.
const testPostgresDSNEnv = "SYNCYOMI_TEST_POSTGRES_DSN"

func testPostgresDSN() string { return os.Getenv(testPostgresDSNEnv) }

// requireSQLite skips tests that inspect SQLite internals (PRAGMA, sqlite_master,
// ALTER TABLE ... DROP COLUMN) when the Postgres matrix is enabled.
func requireSQLite(t *testing.T) {
	t.Helper()
	if testPostgresDSN() != "" {
		t.Skipf("SQLite-specific test skipped because %s is set; unset it to run the SQLite matrix", testPostgresDSNEnv)
	}
}

// newTestPostgresDB opens a throwaway Postgres schema with the full schema
// applied. Each call gets its own schema so tests stay isolated.
func newTestPostgresDB(t *testing.T) *DB {
	t.Helper()

	baseDSN := testPostgresDSN()
	if baseDSN == "" {
		t.Fatalf("%s is not set", testPostgresDSNEnv)
	}
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("parse %s: %v", testPostgresDSNEnv, err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		t.Fatalf("%s must be a postgres URL (got scheme %q); lib/pq keyword DSNs are not supported here", testPostgresDSNEnv, u.Scheme)
	}

	base, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("open base postgres connection: %v", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelPing()
	if err := base.PingContext(pingCtx); err != nil {
		_ = base.Close()
		t.Fatalf("postgres unreachable at %s: %v", u.Redacted(), err)
	}

	schema := fmt.Sprintf("syncyomi_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	quotedSchema := `"` + schema + `"` // pid/nanoseconds only; no quoting hazards
	if _, err := base.Exec(`CREATE SCHEMA ` + quotedSchema); err != nil {
		_ = base.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}
	_ = base.Close()

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	schemaDSN := u.String()

	db := &DB{
		squirrel: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		log:      zerolog.Nop(),
		Driver:   "postgres",
		DSN:      schemaDSN,
	}
	db.ctx, db.cancel = context.WithCancel(context.Background())

	databaseDriver = "postgres"
	if err := db.openPostgres(); err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()

		cleanup, err := sql.Open("postgres", baseDSN)
		if err != nil {
			t.Logf("cleanup: open base postgres connection: %v", err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.Exec(`DROP SCHEMA IF EXISTS ` + quotedSchema + ` CASCADE`); err != nil {
			t.Logf("cleanup: drop schema %s: %v", schema, err)
		}
	})

	return db
}
