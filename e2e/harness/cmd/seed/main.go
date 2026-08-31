// Command seed pushes a generated fixture backup to a running SyncYomi server
// via the v2 merge endpoint, acting as a synthetic device. For manual testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:8790", "SyncYomi base URL")
	key := flag.String("key", "", "API key")
	prefix := flag.String("prefix", "E2E Alpha", "fixture title prefix")
	manga := flag.Int("manga", 5, "manga count")
	chapters := flag.Int("chapters", 3, "chapters per manga")
	flag.Parse()
	if *key == "" {
		fmt.Fprintln(os.Stderr, "-key required")
		os.Exit(1)
	}

	srv := &harness.SyncServer{BaseURL: *server, APIKey: *key}
	c := harness.NewSyntheticClient(srv, "e2e-seed")
	resp, err := c.Merge(context.Background(),
		harness.FixtureBackup(*prefix, *manga, *chapters), harness.MergeOptions{Full: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "merge:", err)
		os.Exit(1)
	}
	fmt.Printf("seeded; server cursor=%d, response manga=%d\n", c.Cursor, len(resp.BackupManga))
}
