//go:build e2e_v1

package v1

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	_ "modernc.org/sqlite"
)

func encodeFixture(t *testing.T, prefix string, mangaCount, chapterCount int) []byte {
	t.Helper()
	raw, err := backup.Encode(harness.FixtureBackup(prefix, mangaCount, chapterCount))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// V1-S1: a v1 push comes back byte-identical with a uuid= etag, and If-None-Match 304s.
func TestV1_EchoRoundTrip(t *testing.T) {
	srv := startServer(t, 8795)
	ctx := context.Background()
	c := harness.NewSyntheticClient(srv, "")

	// empty key: nothing to fetch yet
	if _, _, status, err := c.GetV1(ctx, ""); err != nil || status != http.StatusNotFound {
		t.Fatalf("initial get = %d, %v; want 404", status, err)
	}

	raw := encodeFixture(t, "s1", 5, 3)
	etag, status, err := c.PutV1(ctx, raw, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("put = %d, %v", status, err)
	}
	if !strings.HasPrefix(etag, "uuid=") {
		t.Fatalf("etag = %q, want uuid= prefix", etag)
	}

	data, gotTag, status, err := c.GetV1(ctx, "")
	if err != nil || status != http.StatusOK {
		t.Fatalf("get = %d, %v", status, err)
	}
	if gotTag != etag {
		t.Errorf("get etag = %q, put etag = %q", gotTag, etag)
	}
	if !bytes.Equal(data, raw) {
		t.Fatal("v1 get is not byte-identical to the upload")
	}

	if _, _, status, err = c.GetV1(ctx, etag); err != nil || status != http.StatusNotModified {
		t.Errorf("If-None-Match get = %d, %v; want 304", status, err)
	}

	// gzip-encoded upload lands identically
	raw2 := encodeFixture(t, "s1b", 2, 1)
	etag2, status, err := c.PutV1(ctx, raw2, etag, true)
	if err != nil || status != http.StatusOK {
		t.Fatalf("gzip put = %d, %v", status, err)
	}
	data, _, _, _ = c.GetV1(ctx, "")
	if !bytes.Equal(data, raw2) || etag2 == etag {
		t.Error("gzip upload not echoed")
	}
}

// V1-S2: two v1 devices exchange state through the blob; If-Match protects against races.
func TestV1_TwoDevices(t *testing.T) {
	srv := startServer(t, 8796)
	ctx := context.Background()
	a := harness.NewSyntheticClient(srv, "")
	b := harness.NewSyntheticClient(srv, "")

	rawA := encodeFixture(t, "s2a", 4, 2)
	etagA, status, err := a.PutV1(ctx, rawA, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("A put = %d, %v", status, err)
	}

	// B pulls A's exact bytes, merges locally (simulated), pushes the union
	got, gotTag, _, err := b.GetV1(ctx, "")
	if err != nil || !bytes.Equal(got, rawA) || gotTag != etagA {
		t.Fatalf("B pull mismatch: etag=%q err=%v", gotTag, err)
	}
	merged := harness.FixtureBackup("s2a", 4, 2)
	merged.BackupManga = append(merged.BackupManga, harness.FixtureBackup("s2b", 3, 1).BackupManga...)
	rawB, _ := backup.Encode(merged)
	etagB, status, err := b.PutV1(ctx, rawB, gotTag, false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("B put = %d, %v", status, err)
	}

	// A pushing with its stale etag must 412, then pull B's bytes verbatim
	if _, status, err = a.PutV1(ctx, rawA, etagA, false); err != nil || status != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match = %d, %v; want 412", status, err)
	}
	got, gotTag, _, err = a.GetV1(ctx, "")
	if err != nil || !bytes.Equal(got, rawB) || gotTag != etagB {
		t.Fatal("A did not receive B's exact bytes")
	}
}

// V1-S3: a database written by a pre-1.3 server keeps serving its blob after the upgrade,
// even when the blob cannot be decoded, and a later valid upload takes over cleanly.
func TestV1_LegacyUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name string
		port int
		blob []byte
	}{
		{name: "decodable", port: 8797},
		{name: "undecodable", port: 8798, blob: []byte{0xde, 0xad, 0xbe, 0xef, 0x02}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, tc.port)
			ctx := context.Background()
			c := harness.NewSyntheticClient(srv, "")

			blob := tc.blob
			if blob == nil {
				blob = encodeFixture(t, "s3", 6, 2)
			}

			// fabricate 1.1.14 state: a bare sync_data row, no rendered_seq, no item store
			srv.Stop()
			db, err := sql.Open("sqlite", "file:"+filepath.Join(srv.DataDir, "syncyomi.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(
				`INSERT INTO sync_data (user_api_key, data, data_etag) VALUES ($1, $2, 'uuid=legacy')`,
				srv.APIKey, blob); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := srv.Restart(ctx); err != nil {
				t.Fatal(err)
			}

			data, etag, status, err := c.GetV1(ctx, "")
			if err != nil || status != http.StatusOK {
				t.Fatalf("get after upgrade = %d, %v", status, err)
			}
			if !bytes.Equal(data, blob) || etag != "uuid=legacy" {
				t.Fatal("legacy blob not served verbatim with its original etag")
			}

			// a valid upload replaces it without ever having destroyed it
			raw := encodeFixture(t, "s3new", 2, 1)
			newTag, status, err := c.PutV1(ctx, raw, etag, false)
			if err != nil || status != http.StatusOK {
				t.Fatalf("put after upgrade = %d, %v", status, err)
			}
			data, etag, _, _ = c.GetV1(ctx, "")
			if !bytes.Equal(data, raw) || etag != newTag {
				t.Fatal("upload after upgrade not echoed")
			}
		})
	}
}

// V1-S4: a v2 write invalidates the raw blob (v1 falls back to a render containing both
// sides), and the next v1 upload resumes the echo.
func TestV1_MixedFleet(t *testing.T) {
	srv := startServer(t, 8799)
	ctx := context.Background()
	v1 := harness.NewSyntheticClient(srv, "")
	v2 := harness.NewSyntheticClient(srv, "e2e-v2-device")

	raw := encodeFixture(t, "s4v1", 3, 2)
	etag, status, err := v1.PutV1(ctx, raw, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("v1 put = %d, %v", status, err)
	}

	// v2 device merges its own library in
	if _, err := v2.Merge(ctx, harness.FixtureBackup("s4v2", 2, 1), harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("v2 merge: %v", err)
	}

	data, gotTag, status, err := v1.GetV1(ctx, "")
	if err != nil || status != http.StatusOK {
		t.Fatalf("v1 get after v2 write = %d, %v", status, err)
	}
	if gotTag == etag || !strings.HasPrefix(gotTag, "seq=") {
		t.Fatalf("etag after v2 write = %q, want a fresh seq= render", gotTag)
	}
	render, err := backup.Decode(data)
	if err != nil {
		t.Fatalf("render fallback does not decode: %v", err)
	}
	if len(render.BackupManga) != 5 {
		t.Errorf("render has %d manga, want 5 (3 v1 + 2 v2)", len(render.BackupManga))
	}

	// v1 pushes its client-merged state: echo resumes
	rawMerged, _ := backup.Encode(render)
	newTag, status, err := v1.PutV1(ctx, rawMerged, gotTag, false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("v1 re-put = %d, %v", status, err)
	}
	data, gotTag, _, _ = v1.GetV1(ctx, "")
	if !bytes.Equal(data, rawMerged) || gotTag != newTag {
		t.Fatal("echo did not resume after v1 upload")
	}

	// and the v2 device sees the v1 upload through the item store
	resp, err := v2.Merge(ctx, nil, harness.MergeOptions{Full: true})
	if err != nil {
		t.Fatalf("v2 refetch: %v", err)
	}
	if len(resp.BackupManga) != 5 {
		t.Errorf("v2 sees %d manga after v1 upload, want 5", len(resp.BackupManga))
	}
}

// V1-S5: the error surface v1 clients depend on — garbage is accepted and echoed
// (1.1.14 behaviour), never imported, and never served to v2.
func TestV1_GarbageTolerated(t *testing.T) {
	srv := startServer(t, 8800)
	ctx := context.Background()
	c := harness.NewSyntheticClient(srv, "")

	garbage := []byte("not a protobuf at all")
	etag, status, err := c.PutV1(ctx, garbage, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("garbage put = %d, %v; 1.1.14 accepted arbitrary bytes", status, err)
	}
	data, gotTag, _, err := c.GetV1(ctx, "")
	if err != nil || !bytes.Equal(data, garbage) || gotTag != etag {
		t.Fatal("garbage not echoed verbatim")
	}

	// a later valid upload recovers the key
	raw := encodeFixture(t, "s5", 2, 1)
	if _, status, err = c.PutV1(ctx, raw, gotTag, false); err != nil || status != http.StatusOK {
		t.Fatalf("recovery put = %d, %v", status, err)
	}
	data, _, _, _ = c.GetV1(ctx, "")
	if !bytes.Equal(data, raw) {
		t.Fatal("recovery upload not echoed")
	}
}

// within runs fn with the forks' 10 s client deadline and fails the test when it takes
// longer than budget.
func within(t *testing.T, name string, budget time.Duration, fn func(ctx context.Context)) time.Duration {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), harness.LegacyClientTimeout)
	defer cancel()
	start := time.Now()
	fn(ctx)
	took := time.Since(start)
	if took > budget {
		t.Errorf("%s took %s, budget %s", name, took, budget)
	} else {
		t.Logf("%s: %s", name, took)
	}
	return took
}

// importTook reads the duration of the last v1 import from the server log, "" if none ran.
func importTook(t *testing.T, srv *harness.SyncServer) string {
	t.Helper()
	log, err := os.ReadFile(srv.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	matches := importTookRe.FindAllSubmatch(log, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1][1]) + "ms"
}

// the console logger colours the field name, so an escape sequence may sit before the value
var importTookRe = regexp.MustCompile(`imported v1 upload into the item store.*took=(?:\x1b\[[0-9;]*m)?([0-9.]+)`)

func TestV1_LargeLibraryUnderTimeout(t *testing.T) {
	srv := startServer(t, 8801)
	v1 := harness.NewSyntheticClient(srv, "")
	v2 := harness.NewSyntheticClient(srv, "e2e-v2-device")
	raw := encodeFixture(t, "s6", 3600, 60)
	t.Logf("payload %d bytes", len(raw))

	var etag string
	within(t, "first put", harness.LegacyClientTimeout, func(ctx context.Context) {
		tag, status, err := v1.PutV1(ctx, raw, "", false)
		if err != nil || status != http.StatusOK {
			t.Fatalf("put = %d, %v", status, err)
		}
		etag = tag
	})
	within(t, "get", harness.LegacyClientTimeout, func(ctx context.Context) {
		data, tag, status, err := v1.GetV1(ctx, "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("get = %d, %v", status, err)
		}
		if tag != etag || !bytes.Equal(data, raw) {
			t.Fatal("large upload not echoed")
		}
	})
	within(t, "second put", harness.LegacyClientTimeout, func(ctx context.Context) {
		tag, status, err := v1.PutV1(ctx, raw, etag, false)
		if err != nil || status != http.StatusOK {
			t.Fatalf("second put = %d, %v", status, err)
		}
		etag = tag
	})

	resp, err := v2.Merge(context.Background(), harness.FixtureBackup("s6v2", 1, 1), harness.MergeOptions{Full: true})
	if err != nil {
		t.Fatalf("v2 full merge: %v", err)
	}
	if len(resp.BackupManga) != 3601 {
		t.Fatalf("v2 sees %d manga, want 3601", len(resp.BackupManga))
	}
	within(t, "get after v2 write", harness.LegacyClientTimeout, func(ctx context.Context) {
		data, tag, status, err := v1.GetV1(ctx, etag)
		if err != nil || status != http.StatusOK || !strings.HasPrefix(tag, "seq=") {
			t.Fatalf("get after v2 write = %d %q, %v", status, tag, err)
		}
		if render, err := backup.Decode(data); err != nil || len(render.BackupManga) != 3601 {
			t.Fatalf("render fallback = %d manga, %v", len(render.BackupManga), err)
		}
	})
}

// V1-S7: while something holds the database write lock for longer than a phone waits
// (a large import does), v1 reads and event reports still answer at once; the device and
// status bookkeeping they trigger lands once the lock is free.
func TestV1_ResponsiveWhileStoreLocked(t *testing.T) {
	srv := startServer(t, 8802)
	ctx := context.Background()
	c := harness.NewSyntheticClient(srv, "e2e-v1-phone")
	raw := encodeFixture(t, "s7", 3600, 60)

	etag, status, err := c.PutV1(ctx, raw, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("put = %d, %v", status, err)
	}
	if status, err := c.ReportEvent(ctx, "SYNC_STARTED", ""); err != nil || status != http.StatusNoContent {
		t.Fatalf("event = %d, %v", status, err)
	}

	const hold = 8 * time.Second
	release, err := harness.HoldWriteLock(filepath.Join(srv.DataDir, "syncyomi.db"), hold)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	lockedAt := time.Now()

	// a phone hangs up at 10 s; two bookkeeping writes waiting for the lock used to eat it all
	const budget = 2 * time.Second
	within(t, "get while locked", budget, func(ctx context.Context) {
		data, tag, status, err := c.GetV1(ctx, "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("get = %d, %v", status, err)
		}
		if tag != etag || !bytes.Equal(data, raw) {
			t.Fatal("upload not echoed")
		}
	})
	within(t, "304 while locked", budget, func(ctx context.Context) {
		if _, _, status, err := c.GetV1(ctx, etag); err != nil || status != http.StatusNotModified {
			t.Fatalf("If-None-Match get = %d, %v", status, err)
		}
	})
	within(t, "event while locked", budget, func(ctx context.Context) {
		if status, err := c.ReportEvent(ctx, "SYNC_SUCCESS", ""); err != nil || status != http.StatusNoContent {
			t.Fatalf("event = %d, %v", status, err)
		}
	})
	if time.Since(lockedAt) >= hold {
		t.Fatalf("the lock expired before the requests finished; raise hold")
	}
	release()

	// writers legitimately waited; now they go through
	within(t, "put after release", harness.LegacyClientTimeout, func(ctx context.Context) {
		if _, status, err := c.PutV1(ctx, raw, etag, false); err != nil || status != http.StatusOK {
			t.Fatalf("put after release = %d, %v", status, err)
		}
	})

	// the bookkeeping the locked requests queued lands once the lock is free
	err = harness.WaitFor(ctx, 15*time.Second, func() bool {
		st, err := srv.Status(ctx)
		if err != nil || st.LastProtocol != "v1" || st.LastEvent != "SYNC_SUCCESS" {
			return false
		}
		devices, err := srv.Devices(ctx)
		if err != nil {
			return false
		}
		for _, d := range devices {
			if d.DeviceID == c.DeviceID && d.LastEvent == "SYNC_SUCCESS" {
				return true
			}
		}
		return false
	})
	if err != nil {
		st, _ := srv.Status(ctx)
		devices, _ := srv.Devices(ctx)
		t.Fatalf("bookkeeping never landed: %v (status %+v, devices %+v)", err, st, devices)
	}
}

// V1-S8: the real thing — a v2 device's full merge imports the pending v1 upload under
// the write lock while a v1 phone keeps polling and reporting; every one of its requests
// answers well inside the phone's 10 s.
func TestV1_ResponsiveDuringImport(t *testing.T) {
	srv := startServer(t, 8803)
	ctx := context.Background()
	v1 := harness.NewSyntheticClient(srv, "e2e-v1-phone")
	v2 := harness.NewSyntheticClient(srv, "e2e-v2-device")
	raw := encodeFixture(t, "s8", 3600, 60)

	etag, status, err := v1.PutV1(ctx, raw, "", false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("put = %d, %v", status, err)
	}

	merged := make(chan error, 1)
	go func() {
		resp, err := v2.Merge(ctx, harness.FixtureBackup("s8v2", 1, 1), harness.MergeOptions{Full: true})
		if err == nil && len(resp.BackupManga) != 3601 {
			err = fmt.Errorf("v2 sees %d manga, want 3601", len(resp.BackupManga))
		}
		merged <- err
	}()

	const budget = 3 * time.Second
	var rounds int
	var maxGet, maxEvent time.Duration
	for done := false; !done; {
		select {
		case err := <-merged:
			if err != nil {
				t.Fatalf("v2 full merge: %v", err)
			}
			done = true
		default:
		}
		maxGet = max(maxGet, within(t, "get during import", budget, func(ctx context.Context) {
			data, tag, status, err := v1.GetV1(ctx, "")
			if err != nil || status != http.StatusOK {
				t.Fatalf("get = %d, %v", status, err)
			}
			// the echo until the import commits, the render afterwards
			if tag == etag && !bytes.Equal(data, raw) {
				t.Fatal("upload not echoed")
			}
		}))
		maxEvent = max(maxEvent, within(t, "event during import", budget, func(ctx context.Context) {
			if status, err := v1.ReportEvent(ctx, "SYNC_STARTED", ""); err != nil || status != http.StatusNoContent {
				t.Fatalf("event = %d, %v", status, err)
			}
		}))
		rounds++
		if !done {
			time.Sleep(200 * time.Millisecond)
		}
	}
	t.Logf("%d rounds while the import ran %s: max get %s, max event %s", rounds, importTook(t, srv), maxGet, maxEvent)

	within(t, "get after import", harness.LegacyClientTimeout, func(ctx context.Context) {
		data, tag, status, err := v1.GetV1(ctx, etag)
		if err != nil || status != http.StatusOK || !strings.HasPrefix(tag, "seq=") {
			t.Fatalf("get after import = %d %q, %v", status, tag, err)
		}
		if render, err := backup.Decode(data); err != nil || len(render.BackupManga) != 3601 {
			t.Fatalf("render = %d manga, %v", len(render.BackupManga), err)
		}
	})
}
