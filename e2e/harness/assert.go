package harness

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"

	_ "modernc.org/sqlite"
)

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

func (s *SyncServer) Seq(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/api/sync/v2/snapshot", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-API-Token", s.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return 0, nil
	case http.StatusOK, http.StatusNotModified:
		return strconv.ParseInt(resp.Header.Get("X-Sync-Cursor"), 10, 64)
	default:
		return 0, fmt.Errorf("snapshot: status %d", resp.StatusCode)
	}
}

func (s *SyncServer) Devices(ctx context.Context) ([]domain.SyncDevice, error) {
	var devices []domain.SyncDevice
	err := s.AdminGet(ctx, "/api/sync/admin/"+s.APIKey+"/devices", &devices)
	return devices, err
}

func (s *SyncServer) Status(ctx context.Context) (*domain.SyncStatus, error) {
	var st domain.SyncStatus
	if err := s.AdminGet(ctx, "/api/sync/admin/"+s.APIKey+"/status", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func WaitFor(ctx context.Context, timeout time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cond() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within %s", timeout)
}

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

// renamedSchemaViews maps the tables TachiyomiSY renamed in its schema rewrite (manga, chapter, category,
// manga_category) back to the names these assertions query, so they read either client generation.
var renamedSchemaViews = []string{
	`CREATE TEMP VIEW mangas AS SELECT id AS _id, coalesce(user_title, remote_title) AS title,
		user_favorite_at IS NOT NULL AS favorite, state_last_modified_at AS last_modified_at FROM main.manga`,
	`CREATE TEMP VIEW chapters AS SELECT id AS _id, manga_id, remote_url AS url, user_read AS read,
		state_last_modified_at AS last_modified_at FROM main.chapter`,
	`CREATE TEMP VIEW categories AS SELECT id AS _id, name, "order" AS sort FROM main.category`,
	`CREATE TEMP VIEW mangas_categories AS SELECT manga_id, category_id FROM main.manga_category`,
	`CREATE TEMP VIEW e2e_history AS SELECT chapter_id, read_at FROM main.history`,
}

var legacySchemaViews = []string{
	`CREATE TEMP VIEW e2e_history AS SELECT chapter_id, last_read AS read_at FROM main.history`,
}

func OpenAppDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := createSchemaViews(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("map app schema of %s: %w", path, err)
	}
	return db, nil
}

func createSchemaViews(db *sql.DB) error {
	var renamed bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'manga')`).Scan(&renamed)
	if err != nil {
		return err
	}
	views := legacySchemaViews
	if renamed {
		views = renamedSchemaViews
	}
	for _, v := range views {
		if _, err := db.Exec(v); err != nil {
			return err
		}
	}
	return nil
}

func LastReadAt(db *sql.DB, mangaTitle, chapterURL string) (int64, error) {
	var readAt sql.NullInt64
	err := db.QueryRow(`
		SELECT h.read_at FROM e2e_history h
		JOIN chapters c ON c._id = h.chapter_id
		JOIN mangas m ON m._id = c.manga_id
		WHERE m.title = ? AND c.url = ?`, mangaTitle, chapterURL).Scan(&readAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return readAt.Int64, err
}

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

func ReadChapterCount(db *sql.DB, mangaTitle string) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM chapters c
		JOIN mangas m ON m._id = c.manga_id
		WHERE m.title = ? AND c.read = 1`, mangaTitle).Scan(&n)
	return n, err
}

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

func MaxLastModifiedAt(db *sql.DB) (int64, error) {
	var v int64
	err := db.QueryRow(`SELECT max(
		coalesce((SELECT max(last_modified_at) FROM mangas), 0),
		coalesce((SELECT max(last_modified_at) FROM chapters), 0))`).Scan(&v)
	return v, err
}

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

type CategorySort struct {
	Name string
	Sort int64
}

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
