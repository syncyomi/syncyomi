package harness

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"

	_ "modernc.org/sqlite"
)

// Snapshot fetches and decodes the server's rendered library for the test API key.
func (s *SyncServer) Snapshot(ctx context.Context) (*pb.Backup, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/api/sync/v2/snapshot", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Token", s.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return backup.Decode(data)
}

// Devices lists the devices the server has seen for the test API key.
func (s *SyncServer) Devices(ctx context.Context) ([]domain.SyncDevice, error) {
	var devices []domain.SyncDevice
	err := s.AdminGet(ctx, "/api/sync/admin/"+s.APIKey+"/devices", &devices)
	return devices, err
}

// WaitForDeviceSync polls until some device's last_seen passes since and its
// cursor is positive — i.e. a v2 merge completed after the trigger.
func (s *SyncServer) WaitForDeviceSync(ctx context.Context, since time.Time, timeout time.Duration) (*domain.SyncDevice, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		devices, err := s.Devices(ctx)
		if err == nil {
			for i := range devices {
				d := &devices[i]
				if d.LastSeen.After(since) && d.Cursor > 0 {
					return d, nil
				}
			}
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("no device completed a sync within %s", timeout)
}

// SnapshotTitles returns the sorted favorite manga titles in a backup.
func SnapshotTitles(b *pb.Backup) []string {
	var titles []string
	for _, m := range b.BackupManga {
		if backup.IsFavorite(m) {
			titles = append(titles, m.Title)
		}
	}
	sort.Strings(titles)
	return titles
}

// OpenAppDB opens a pulled tachiyomi.db read-only.
func OpenAppDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

// LibraryTitles returns the sorted titles of favorited manga in a pulled app DB.
func LibraryTitles(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT title FROM mangas WHERE favorite = 1 ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		titles = append(titles, t)
	}
	return titles, rows.Err()
}

// ReadChapterCount returns how many chapters of the given manga are marked read.
func ReadChapterCount(db *sql.DB, mangaTitle string) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM chapters c
		JOIN mangas m ON m._id = c.manga_id
		WHERE m.title = ? AND c.read = 1`, mangaTitle).Scan(&n)
	return n, err
}

// MangaCategoryNames returns the names of the categories a manga belongs to.
func MangaCategoryNames(db *sql.DB, mangaTitle string) ([]string, error) {
	rows, err := db.Query(`
		SELECT c.name FROM categories c
		JOIN mangas_categories mc ON mc.category_id = c._id
		JOIN mangas m ON m._id = mc.manga_id
		WHERE m.title = ? ORDER BY c.name`, mangaTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// TableCounts returns row counts for the tables sync writes to.
func TableCounts(db *sql.DB) (mangas, chapters, categories int, err error) {
	if err = db.QueryRow(`SELECT COUNT(*) FROM mangas`).Scan(&mangas); err != nil {
		return
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM chapters`).Scan(&chapters); err != nil {
		return
	}
	err = db.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&categories)
	return
}

// CategoryNames returns user categories (excluding the built-in default).
func CategoryNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM categories ORDER BY sort`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// CategorySort is one user category's position in the app DB.
type CategorySort struct {
	Name string
	Sort int64
}

// CategorySorts returns user categories (system category excluded) by position.
func CategorySorts(db *sql.DB) ([]CategorySort, error) {
	rows, err := db.Query(`SELECT name, sort FROM categories WHERE _id != 0 ORDER BY sort`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategorySort
	for rows.Next() {
		var c CategorySort
		if err := rows.Scan(&c.Name, &c.Sort); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
