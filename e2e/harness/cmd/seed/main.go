// Command seed pushes a generated fixture backup to a running SyncYomi server,
// acting as a synthetic device. For manual testing. -protocol picks the v2 merge
// endpoint (default) or the legacy v1 upload, which sends no device headers just
// like real v1 clients.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:8790", "SyncYomi base URL")
	key := flag.String("key", "", "API key")
	prefix := flag.String("prefix", "E2E Alpha", "fixture title prefix")
	manga := flag.Int("manga", 5, "manga count")
	chapters := flag.Int("chapters", 3, "chapters per manga")
	protocol := flag.String("protocol", "v2", "sync protocol to use: v1 or v2")
	device := flag.String("device", "e2e-seed", "device id (v2 only)")
	flag.Parse()
	if *key == "" {
		fmt.Fprintln(os.Stderr, "-key required")
		os.Exit(1)
	}

	srv := &harness.SyncServer{BaseURL: *server, APIKey: *key}
	b := harness.FixtureBackup(*prefix, *manga, *chapters)

	switch *protocol {
	case "v2":
		c := harness.NewSyntheticClient(srv, *device)
		resp, err := c.Merge(context.Background(), b, harness.MergeOptions{Full: true})
		if err != nil {
			fmt.Fprintln(os.Stderr, "merge:", err)
			os.Exit(1)
		}
		fmt.Printf("seeded; server cursor=%d, response manga=%d\n", c.Cursor, len(resp.BackupManga))
	case "v1":
		raw, err := backup.Encode(b)
		if err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
			os.Exit(1)
		}
		c := harness.NewSyntheticClient(srv, "")
		etag, status, err := c.PutV1(context.Background(), raw, "", false)
		if err != nil || status != 200 {
			fmt.Fprintf(os.Stderr, "v1 put: status %d err %v\n", status, err)
			os.Exit(1)
		}
		fmt.Printf("seeded via v1; etag=%s size=%d\n", etag, len(raw))
	default:
		fmt.Fprintln(os.Stderr, "-protocol must be v1 or v2")
		os.Exit(1)
	}
}
