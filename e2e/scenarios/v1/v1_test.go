//go:build e2e_v1

package v1

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

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
