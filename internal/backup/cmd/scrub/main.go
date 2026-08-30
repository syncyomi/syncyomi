// Command scrub turns a real Tachiyomi backup into a checked-in test fixture without personal data.
//
//	go run ./internal/backup/cmd/scrub -in internal/backup/testdata/private/x.tachibk -out internal/backup/testdata/backup_scrubbed.tachibk
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/SyncYomi/SyncYomi/internal/backup"
)

func main() {
	in := flag.String("in", "", "real .tachibk to scrub")
	out := flag.String("out", "internal/backup/testdata/backup_scrubbed.tachibk", "output path (gzipped)")
	maxManga := flag.Int("max-manga", 0, "keep only the first N manga (0 = all)")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	b, err := backup.Decode(raw)
	if err != nil {
		log.Fatal(err)
	}

	if *maxManga > 0 && len(b.BackupManga) > *maxManga {
		b.BackupManga = b.BackupManga[:*maxManga]
	}
	scrubbed := backup.Scrub(b)
	data, err := backup.EncodeGzip(scrubbed)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}

	chapters := 0
	for _, m := range scrubbed.BackupManga {
		chapters += len(m.Chapters)
	}
	fmt.Printf("wrote %s: %d bytes, manga=%d chapters=%d categories=%d sources=%d prefs=%d sourcePrefs=%d extStores=%d savedSearches=%d\n",
		*out, len(data), len(scrubbed.BackupManga), chapters, len(scrubbed.BackupCategories), len(scrubbed.BackupSources),
		len(scrubbed.BackupPreferences), len(scrubbed.BackupSourcePreferences), len(scrubbed.BackupExtensionStores), len(scrubbed.BackupSavedSearches))
}
