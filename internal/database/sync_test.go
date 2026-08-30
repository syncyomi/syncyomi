package database

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
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

	for _, table := range []string{"users", "api_key", "notification", "sync_data", "sync_data_history", "sync_device", "sync_status"} {
		var name string
		err := db.handler.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = $1`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestSyncRepo_GetUnknownKey(t *testing.T) {
	db := newTestDB(t)
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
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
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
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
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
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
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
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
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}

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
	syncRepo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
	apiRepo := APIRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	if _, err := syncRepo.SetSyncData(ctx, "key1", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncRepo.SetSyncData(ctx, "key2", []byte("b")); err != nil {
		t.Fatal(err)
	}

	if err := syncRepo.TouchDevice(ctx, "key1", domain.DeviceInfo{ID: "d"}, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := apiRepo.Delete(ctx, "key1"); err != nil {
		t.Fatal(err)
	}
	for _, table := range apiKeyDependentTables {
		if n := countRows(t, db, table, "key1"); n != 0 {
			t.Errorf("%s rows for deleted key = %d, want 0", table, n)
		}
	}
	if n := countRows(t, db, "sync_data", "key2"); n != 1 {
		t.Errorf("sync_data rows for other key = %d, want 1", n)
	}
	if _, err := apiRepo.Get(ctx, "key1"); err == nil {
		t.Error("deleted api key still readable")
	}
}

func TestSyncRepo_History(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
	ctx := context.Background()

	var etags []string
	for i := 1; i <= 5; i++ {
		etag, err := repo.SetSyncData(ctx, "key1", []byte{byte(i)})
		if err != nil {
			t.Fatal(err)
		}
		etags = append(etags, *etag)
	}

	history, err := repo.ListHistory(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history len = %d, want 3", len(history))
	}
	for i, want := range []string{etags[4], etags[3], etags[2]} {
		if history[i].ETag != want || history[i].Size != 1 {
			t.Errorf("history[%d] = %+v, want etag %q size 1", i, history[i], want)
		}
	}

	data, err := repo.GetHistoryData(ctx, "key1", history[2].ID)
	if err != nil || !bytes.Equal(data, []byte{3}) {
		t.Errorf("GetHistoryData = %v, %v", data, err)
	}
	if _, err := repo.GetHistoryData(ctx, "key1", 99999); !stderrors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown id err = %v, want ErrNotFound", err)
	}
	newEtag := &history[0].ETag

	// IfMatch writes are recorded too
	if _, err := repo.SetSyncDataIfMatch(ctx, "key1", *newEtag, []byte{9}); err != nil {
		t.Fatal(err)
	}
	history, _ = repo.ListHistory(ctx, "key1")
	if history[0].Size != 1 || history[0].ETag == *newEtag {
		t.Errorf("IfMatch write not recorded: %+v", history)
	}
}

func TestSyncRepo_HistoryDisabled(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 0}
	ctx := context.Background()

	if _, err := repo.SetSyncData(ctx, "key1", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "sync_data_history", "key1"); n != 0 {
		t.Errorf("history rows = %d, want 0", n)
	}
}

func TestSyncRepo_Devices(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	if err := repo.TouchDevice(ctx, "key1", domain.DeviceInfo{}, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "sync_device", "key1"); n != 0 {
		t.Errorf("anonymous touch created %d rows", n)
	}

	if err := repo.TouchDevice(ctx, "key1", domain.DeviceInfo{ID: "d1", Name: "Phone"}, "SYNC_SUCCESS", "success", "done"); err != nil {
		t.Fatal(err)
	}
	// second touch with no name/event keeps what we know
	if err := repo.TouchDevice(ctx, "key1", domain.DeviceInfo{ID: "d1"}, "", "", ""); err != nil {
		t.Fatal(err)
	}
	// name-only device falls back to name as id
	if err := repo.TouchDevice(ctx, "key1", domain.DeviceInfo{Name: "Tablet"}, "SYNC_STARTED", "running", ""); err != nil {
		t.Fatal(err)
	}

	devices, err := repo.ListDevices(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %+v, want 2", devices)
	}
	byID := map[string]domain.SyncDevice{}
	for _, d := range devices {
		byID[d.DeviceID] = d
	}
	d1 := byID["d1"]
	if d1.DeviceName != "Phone" || d1.LastEvent != "SYNC_SUCCESS" || d1.LastStatus != "success" || d1.LastMessage != "done" {
		t.Errorf("d1 = %+v", d1)
	}
	if tab := byID["Tablet"]; tab.DeviceName != "Tablet" || tab.LastStatus != "running" {
		t.Errorf("tablet = %+v", tab)
	}
}

func TestSyncRepo_Status(t *testing.T) {
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	repo := SyncRepo{log: zerolog.Nop(), db: db}
	ctx := context.Background()

	st, err := repo.GetStatus(ctx, "key1")
	if err != nil || st != nil {
		t.Fatalf("empty status = (%v, %v), want (nil, nil)", st, err)
	}

	eventAt := time.Now().Add(-time.Minute)
	if err := repo.UpsertStatus(ctx, "key1", domain.SyncStatus{LastEventAt: &eventAt, LastEvent: "SYNC_FAILED", LastStatus: "error", LastDevice: "Phone", LastMessage: "boom"}); err != nil {
		t.Fatal(err)
	}
	syncedAt := time.Now()
	if err := repo.UpsertStatus(ctx, "key1", domain.SyncStatus{LastSyncedAt: &syncedAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetSyncData(ctx, "key1", []byte("12345")); err != nil {
		t.Fatal(err)
	}

	st, err = repo.GetStatus(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastEvent != "SYNC_FAILED" || st.LastStatus != "error" || st.LastDevice != "Phone" || st.LastMessage != "boom" {
		t.Errorf("content write cleared event fields: %+v", st)
	}
	if st.LastSyncedAt == nil || st.LastEventAt == nil || st.DataUpdatedAt == nil {
		t.Errorf("timestamps missing: %+v", st)
	}
	if st.DataSize != 5 {
		t.Errorf("data size = %d, want 5", st.DataSize)
	}

	// data only, no status row
	insertTestAPIKey(t, db, "key2")
	if _, err := repo.SetSyncData(ctx, "key2", []byte("ab")); err != nil {
		t.Fatal(err)
	}
	st, err = repo.GetStatus(ctx, "key2")
	if err != nil || st == nil || st.DataSize != 2 {
		t.Errorf("data-only status = (%+v, %v)", st, err)
	}
}

// Upgrading a database that is already at the previous schema version must only run the new migration.
func TestSQLiteMigrationFromPreviousVersion(t *testing.T) {
	databaseDriver = "sqlite"
	db := &DB{
		squirrel: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		log:      zerolog.Nop(),
		Driver:   "sqlite",
		DSN:      filepath.Join(t.TempDir(), "old.db"),
	}
	db.ctx, db.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() { _ = db.Close() })

	if err := db.openSQLite(); err != nil {
		t.Fatal(err)
	}
	// rewind to the previous version by dropping what the last migration added
	previous := len(sqliteMigrations) - 1
	for _, stmt := range []string{
		"DROP TABLE sync_item", "DROP TABLE sync_state",
		"ALTER TABLE sync_device DROP COLUMN last_cursor", "ALTER TABLE sync_device DROP COLUMN protocol",
		"ALTER TABLE sync_data DROP COLUMN rendered_seq",
		fmt.Sprintf("PRAGMA user_version = %d", previous),
	} {
		if _, err := db.handler.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	insertTestAPIKey(t, db, "key1")
	if _, err := db.handler.Exec(`INSERT INTO sync_data (user_api_key, data, data_etag) VALUES ('key1', X'01', 'uuid=old')`); err != nil {
		t.Fatal(err)
	}

	if err := db.migrateSQLite(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var version int
	if err := db.handler.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(sqliteMigrations) {
		t.Errorf("user_version = %d, want %d", version, len(sqliteMigrations))
	}

	repo := SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3}
	ctx := context.Background()
	data, etag, err := repo.GetSyncDataAndETag(ctx, "key1")
	if err != nil || !bytes.Equal(data, []byte{1}) || *etag != "uuid=old" {
		t.Errorf("existing data lost across migration: %v %v %v", data, etag, err)
	}
	if err := repo.TouchDevice(ctx, "key1", domain.DeviceInfo{ID: "d"}, "", "", ""); err != nil {
		t.Errorf("new table unusable after migration: %v", err)
	}
	if _, err := repo.SetSyncData(ctx, "key1", []byte{2}); err != nil {
		t.Errorf("history write after migration: %v", err)
	}
}
