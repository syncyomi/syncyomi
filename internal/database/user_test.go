package database

import (
	"context"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"

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

		if err := repo.UpdatePassword(ctx, first.ID, "new-hash", "new-stamp"); err != nil {
			t.Fatalf("UpdatePassword() error = %v", err)
		}

		updated, _ := repo.FindByUsername(ctx, "first")
		other, _ := repo.FindByUsername(ctx, "second")
		if updated == nil || updated.Password != "new-hash" || updated.Stamp != "new-stamp" {
			t.Errorf("first user = %+v, want password new-hash and stamp new-stamp", updated)
		}
		if other == nil || other.Password != "h2" {
			t.Errorf("second user = %+v, want password h2 untouched", other)
		}
	})

	t.Run("errors for an unknown id", func(t *testing.T) {
		repo, _ := newTestUserRepo(t)

		if err := repo.UpdatePassword(ctx, 999, "new-hash", "new-stamp"); err == nil {
			t.Error("UpdatePassword() error = nil, want an error for an unknown id")
		}
	})
}

func TestUserRepo_StoresTheStamp(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestUserRepo(t)

	if err := repo.Store(ctx, domain.User{Username: "admin", Password: "h", Stamp: "s1"}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	byName, _ := repo.FindByUsername(ctx, "admin")
	first, _ := repo.FindFirst(ctx)
	if byName == nil || byName.Stamp != "s1" || first == nil || first.Stamp != "s1" {
		t.Errorf("FindByUsername() = %+v, FindFirst() = %+v, want stamp s1 on both", byName, first)
	}
}

func TestUserRepo_RehashPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces the hash and pins the legacy stamp so sessions survive", func(t *testing.T) {
		repo, db := newTestUserRepo(t)
		mustExec(t, db, `INSERT INTO users (username, password) VALUES ('admin', 'old-hash')`)
		legacy := domain.User{Password: "old-hash"}.LegacyPasswordStamp()

		applied, err := repo.RehashPassword(ctx, 1, "old-hash", "new-hash")
		if err != nil || !applied {
			t.Fatalf("RehashPassword() = %v, %v, want applied", applied, err)
		}

		got, _ := repo.FindFirst(ctx)
		if got.Password != "new-hash" || got.PasswordStamp() != legacy {
			t.Errorf("user = %+v with stamp %q, want hash new-hash and stamp %q", got, got.PasswordStamp(), legacy)
		}
	})

	t.Run("keeps an existing stored stamp", func(t *testing.T) {
		repo, db := newTestUserRepo(t)
		mustExec(t, db, `INSERT INTO users (username, password, password_stamp) VALUES ('admin', 'old-hash', 'kept')`)

		if applied, err := repo.RehashPassword(ctx, 1, "old-hash", "new-hash"); err != nil || !applied {
			t.Fatalf("RehashPassword() = %v, %v, want applied", applied, err)
		}

		got, _ := repo.FindFirst(ctx)
		if got.Password != "new-hash" || got.Stamp != "kept" {
			t.Errorf("user = %+v, want hash new-hash and stamp kept", got)
		}
	})

	t.Run("does nothing when the password changed in the meantime", func(t *testing.T) {
		repo, db := newTestUserRepo(t)
		mustExec(t, db, `INSERT INTO users (username, password, password_stamp) VALUES ('admin', 'changed-hash', 'fresh')`)

		applied, err := repo.RehashPassword(ctx, 1, "old-hash", "new-hash")

		got, _ := repo.FindFirst(ctx)
		if err != nil || applied || got.Password != "changed-hash" || got.Stamp != "fresh" {
			t.Errorf("applied = %v, error = %v, user = %+v, want the concurrent change left untouched", applied, err, got)
		}
	})
}

func mustExec(t *testing.T, db *DB, query string) {
	t.Helper()
	if _, err := db.handler.Exec(query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
