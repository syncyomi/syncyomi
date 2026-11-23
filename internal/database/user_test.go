package database

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func newTestUserRepo(t *testing.T) (*UserRepo, *DB) {
	t.Helper()
	db := newTestDB(t)
	return &UserRepo{log: zerolog.Nop(), db: db}, db
}

func TestUserRepo_FindFirst(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil when there are no users", func(t *testing.T) {
		repo, _ := newTestUserRepo(t)

		got, err := repo.FindFirst(ctx)

		if err != nil {
			t.Fatalf("FindFirst() error = %v", err)
		}
		if got != nil {
			t.Errorf("FindFirst() = %+v, want nil", got)
		}
	})

	t.Run("returns the user with the lowest id", func(t *testing.T) {
		repo, db := newTestUserRepo(t)
		mustExec(t, db, `INSERT INTO users (username, password) VALUES ('first', 'h1'), ('second', 'h2')`)

		got, err := repo.FindFirst(ctx)

		if err != nil {
			t.Fatalf("FindFirst() error = %v", err)
		}
		if got == nil || got.Username != "first" || got.Password != "h1" {
			t.Errorf("FindFirst() = %+v, want user first/h1", got)
		}
	})
}

func TestUserRepo_UpdatePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("changes only the targeted user", func(t *testing.T) {
		repo, db := newTestUserRepo(t)
		mustExec(t, db, `INSERT INTO users (username, password) VALUES ('first', 'h1'), ('second', 'h2')`)
		first, err := repo.FindFirst(ctx)
		if err != nil || first == nil {
			t.Fatalf("FindFirst() = %+v, %v", first, err)
		}

		if err := repo.UpdatePassword(ctx, first.ID, "new-hash"); err != nil {
			t.Fatalf("UpdatePassword() error = %v", err)
		}

		updated, _ := repo.FindByUsername(ctx, "first")
		other, _ := repo.FindByUsername(ctx, "second")
		if updated == nil || updated.Password != "new-hash" {
			t.Errorf("first user = %+v, want password new-hash", updated)
		}
		if other == nil || other.Password != "h2" {
			t.Errorf("second user = %+v, want password h2 untouched", other)
		}
	})

	t.Run("errors for an unknown id", func(t *testing.T) {
		repo, _ := newTestUserRepo(t)

		if err := repo.UpdatePassword(ctx, 999, "new-hash"); err == nil {
			t.Error("UpdatePassword() error = nil, want an error for an unknown id")
		}
	})
}

func mustExec(t *testing.T, db *DB, query string) {
	t.Helper()
	if _, err := db.handler.Exec(query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
