package database

import (
	"database/sql"
	"fmt"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	_ "modernc.org/sqlite"
)

func (db *DB) openSQLite() error {
	if db.DSN == "" {
		return errors.New("DSN required")
	}

	var err error

	// open database connection
	if db.handler, err = sql.Open("sqlite", db.DSN+"?_pragma=busy_timeout%3d1000"); err != nil {
		db.log.Fatal().Err(err).Msg("could not open db connection")
		return err
	}

	// Set busy timeout
	//if _, err = db.handler.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
	//	return errors.New("busy timeout pragma: %w", err)
	//}

	// Enable WAL. SQLite performs better with the WAL  because it allows
	// multiple readers to operate while data is being written.
	if _, err = db.handler.Exec(`PRAGMA journal_mode = wal;`); err != nil {
		return errors.Wrap(err, "enable wal")
	}

	// When tachi-desk-server does not cleanly shut down, the WAL will still be present and not committed.
	// This is a no-op if the WAL is empty, and a commit when the WAL is not to start fresh.
	// When commits hit 1000, PRAGMA wal_checkpoint(PASSIVE); is invoked which tries its best
	// to commit from the WAL (and can fail to commit all pending operations).
	// Forcing a PRAGMA wal_checkpoint(RESTART); in the future on a "quiet period" could be
	// considered.
	if _, err = db.handler.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`); err != nil {
		return errors.Wrap(err, "commit wal")
	}

	// Enable foreign key checks. For historical reasons, SQLite does not check
	// foreign key constraints by default. There's some overhead on inserts to
	// verify foreign key integrity, but it's definitely worth it.
	//if _, err = db.handler.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
	//	return errors.New("foreign keys pragma: %w", err)
	//}

	// migrate db
	if err = db.migrateSQLite(); err != nil {
		db.log.Fatal().Err(err).Msg("could not migrate db")
		return err
	}

	return nil
}

func (db *DB) migrateSQLite() error {
	db.lock.Lock()
	defer db.lock.Unlock()

	var version int
	if err := db.handler.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return errors.Wrap(err, "failed to query schema version")
	}

	if version == len(sqliteMigrations) {
		return nil
	} else if version > len(sqliteMigrations) {
		return errors.New("SyncYomi (version %d) older than schema (version: %d)", len(sqliteMigrations), version)
	}

	db.log.Info().Msgf("Beginning database schema upgrade from version %v to version: %v", version, len(sqliteMigrations))

	tx, err := db.handler.Begin()
	if err != nil {
		return err
	}
	defer func(tx *sql.Tx) {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			db.log.Error().Msgf("Failed to rollback DB: %v", err)
		}
	}(tx)

	if version == 0 {
		if _, err := tx.Exec(sqliteSchema); err != nil {
			return errors.Wrap(err, "failed to initialize schema")
		}
	} else {
		for i := version; i < len(sqliteMigrations); i++ {
			db.log.Info().Msgf("Upgrading Database schema to version: %v", i)
			if _, err := tx.Exec(sqliteMigrations[i]); err != nil {
				return errors.Wrap(err, "failed to execute migration #%v", i)
			}
		}
	}

	_, err = tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", len(sqliteMigrations)))
	if err != nil {
		return errors.Wrap(err, "failed to bump schema version")
	}

	db.log.Info().Msgf("Database schema upgraded to version: %v", len(sqliteMigrations))

	return tx.Commit()
}
