package harness

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// HoldWriteLock takes the server's SQLite write lock (BEGIN IMMEDIATE, the same mode the
// server uses for its own transactions) and keeps it until release is called or d elapses.
// It stands in for a long import: every write on the server queues behind it, and reads
// must not.
func HoldWriteLock(dbPath string, d time.Duration) (release func(), err error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout%3d5000&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("take write lock: %w", err)
	}
	done := make(chan struct{})
	timer := time.AfterFunc(d, func() { close(done) })
	go func() {
		<-done
		_ = tx.Rollback()
		_ = db.Close()
	}()
	return func() {
		if timer.Stop() {
			close(done)
		}
	}, nil
}
