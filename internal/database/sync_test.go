package database

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog"
)

// newTestDB opens a fresh sqlite database in a temp dir with the full schema applied.
func newTestDB(t *testing.T) *DB {
	t.Helper()

	databaseDriver = "sqlite"

	db := &DB{
		squirrel: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		log:      zerolog.Nop(),
		Driver:   "sqlite",
		DSN:      filepath.Join(t.TempDir(), "test.db"),
	}
	db.ctx, db.cancel = context.WithCancel(context.Background())

	if err := db.openSQLite(); err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func insertTestAPIKey(t *testing.T, db *DB, key string) {
	t.Helper()
	if _, err := db.handler.Exec(`INSERT INTO api_key (name, key) VALUES ('test', $1)`, key); err != nil {
		t.Fatalf("insert api key: %v", err)
	}
}

func countRows(t *testing.T, db *DB, table, apiKey string) int {
	t.Helper()
	var n int
	if err := db.handler.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_api_key = $1`, apiKey).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestSQLiteSchemaVersion(t *testing.T) {
	db := newTestDB(t)

	var version int
	if err := db.handler.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(sqliteMigrations) {
		t.Errorf("user_version = %d, want %d", version, len(sqliteMigrations))
	}

	for _, table := range []string{"users", "api_key", "notification", "sync_data"} {
		var name string
		err := db.handler.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = $1`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestSyncRepo_GetUnknownKey(t *testing.T) {
	db := newTestDB(t)
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	etag, err := repo.GetSyncDataETag(ctx, "missing")
	if err != nil || etag != nil {
		t.Errorf("GetSyncDataETag = (%v, %v), want (nil, nil)", etag, err)
	}

	data, etag, err := repo.GetSyncDataAndETag(ctx, "missing")
	if err != nil || etag != nil || data != nil {
		t.Errorf("GetSyncDataAndETag = (%v, %v, %v), want (nil, nil, nil)", data, etag, err)
	}
}

func TestSyncRepo_SetSyncDataUpserts(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	etag1, err := repo.SetSyncData(ctx, "key1", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	etag2, err := repo.SetSyncData(ctx, "key1", []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}
	if *etag1 == *etag2 {
		t.Errorf("etag did not change on overwrite: %q", *etag1)
	}

	if n := countRows(t, db, "sync_data", "key1"); n != 1 {
		t.Errorf("sync_data rows = %d, want 1", n)
	}

	data, etag, err := repo.GetSyncDataAndETag(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("v2")) {
		t.Errorf("data = %q, want %q", data, "v2")
	}
	if *etag != *etag2 {
		t.Errorf("etag = %q, want %q", *etag, *etag2)
	}
}

// Regression: the first write for a key from several devices at once used to
// race an UPDATE-then-INSERT into a UNIQUE violation.
func TestSyncRepo_SetSyncDataConcurrentFirstWrite(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	const writers = 10
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := repo.SetSyncData(ctx, "key1", []byte{byte(i)})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent SetSyncData: %v", err)
		}
	}

	if n := countRows(t, db, "sync_data", "key1"); n != 1 {
		t.Errorf("sync_data rows = %d, want 1", n)
	}
}

func TestSyncRepo_SetSyncDataIfMatch(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	// no data yet: If-Match can never match
	etag, err := repo.SetSyncDataIfMatch(ctx, "key1", "uuid=nope", []byte("x"))
	if err != nil || etag != nil {
		t.Errorf("IfMatch on empty = (%v, %v), want (nil, nil)", etag, err)
	}

	current, err := repo.SetSyncData(ctx, "key1", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}

	etag, err = repo.SetSyncDataIfMatch(ctx, "key1", "uuid=stale", []byte("v2"))
	if err != nil || etag != nil {
		t.Errorf("stale IfMatch = (%v, %v), want (nil, nil)", etag, err)
	}
	data, _, _ := repo.GetSyncDataAndETag(ctx, "key1")
	if !bytes.Equal(data, []byte("v1")) {
		t.Errorf("stale IfMatch changed data to %q", data)
	}

	etag, err = repo.SetSyncDataIfMatch(ctx, "key1", *current, []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}
	if etag == nil || *etag == *current {
		t.Errorf("matching IfMatch etag = %v, want new etag", etag)
	}
	data, got, _ := repo.GetSyncDataAndETag(ctx, "key1")
	if !bytes.Equal(data, []byte("v2")) || *got != *etag {
		t.Errorf("after IfMatch: data=%q etag=%q, want v2/%q", data, *got, *etag)
	}
}

func TestSyncRepo_ReadsHonourContext(t *testing.T) {
	db := newTestDB(t)
	repo := SyncRepo{log: zerolog.Nop(), db: db}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.GetSyncDataETag(ctx, "key1"); err == nil {
		t.Error("GetSyncDataETag with cancelled context returned no error")
	}
	if _, _, err := repo.GetSyncDataAndETag(ctx, "key1"); err == nil {
		t.Error("GetSyncDataAndETag with cancelled context returned no error")
	}
}

func TestAPIRepo_DeleteCascadesSyncData(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	insertTestAPIKey(t, db, "key2")
	syncRepo := SyncRepo{log: zerolog.Nop(), db: db}
	apiRepo := APIRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	if _, err := syncRepo.SetSyncData(ctx, "key1", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncRepo.SetSyncData(ctx, "key2", []byte("b")); err != nil {
		t.Fatal(err)
	}

	if err := apiRepo.Delete(ctx, "key1"); err != nil {
		t.Fatal(err)
	}

	if n := countRows(t, db, "sync_data", "key1"); n != 0 {
		t.Errorf("sync_data rows for deleted key = %d, want 0", n)
	}
	if n := countRows(t, db, "sync_data", "key2"); n != 1 {
		t.Errorf("sync_data rows for other key = %d, want 1", n)
	}
	if _, err := apiRepo.Get(ctx, "key1"); err == nil {
		t.Error("deleted api key still readable")
	}
}
