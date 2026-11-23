package database

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/rs/zerolog"
)

func newTestPasskeyRepo(t *testing.T) (*PasskeyRepo, *DB) {
	t.Helper()
	db := newTestDB(t)
	mustExec(t, db, `INSERT INTO users (username, password) VALUES ('admin', 'hash'), ('other', 'hash')`)
	return &PasskeyRepo{log: zerolog.Nop(), db: db}, db
}

func samplePasskey(userID int, credentialID, name string) domain.Passkey {
	return domain.Passkey{UserID: userID, CredentialID: []byte(credentialID), Name: name, CredentialJSON: `{"id":"` + credentialID + `"}`}
}

func TestPasskeyRepo_StoreAndList(t *testing.T) {
	ctx := context.Background()

	t.Run("stores a passkey and lists it for its user only, oldest first", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)
		first, err := repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop"))
		if err != nil || first == 0 {
			t.Fatalf("Store() = %d, %v", first, err)
		}
		if _, err := repo.Store(ctx, samplePasskey(1, "cred-b", "Phone")); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Store(ctx, samplePasskey(2, "cred-c", "Other user")); err != nil {
			t.Fatal(err)
		}

		got, err := repo.List(ctx, 1)

		if err != nil || len(got) != 2 || got[0].Name != "Laptop" || got[1].Name != "Phone" {
			t.Fatalf("List() = %+v, %v, want Laptop then Phone", got, err)
		}
		if got[0].CreatedAt.IsZero() || !got[0].LastUsedAt.IsZero() || got[0].CredentialJSON != `{"id":"cred-a"}` || string(got[0].CredentialID) != "cred-a" {
			t.Errorf("List()[0] = %+v, want the stored fields, a creation time and no last use", got[0])
		}
	})

	t.Run("lists nothing for a user without passkeys", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)

		got, err := repo.List(ctx, 1)

		if err != nil || len(got) != 0 {
			t.Errorf("List() = %+v, %v, want an empty list", got, err)
		}
	})

	t.Run("rejects a second passkey with the same credential id", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)
		if _, err := repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop")); err != nil {
			t.Fatal(err)
		}

		if _, err := repo.Store(ctx, samplePasskey(2, "cred-a", "Copy")); err == nil {
			t.Error("Store() error = nil, want a uniqueness error")
		}
	})
}

func TestPasskeyRepo_FindByCredentialID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestPasskeyRepo(t)
	if _, err := repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop")); err != nil {
		t.Fatal(err)
	}

	t.Run("finds the passkey by its credential id", func(t *testing.T) {
		got, err := repo.FindByCredentialID(ctx, []byte("cred-a"))

		if err != nil || got == nil || got.Name != "Laptop" || got.UserID != 1 {
			t.Errorf("FindByCredentialID() = %+v, %v, want the Laptop passkey of user 1", got, err)
		}
	})

	t.Run("returns nil for an unknown credential", func(t *testing.T) {
		got, err := repo.FindByCredentialID(ctx, []byte("unknown"))

		if err != nil || got != nil {
			t.Errorf("FindByCredentialID() = %+v, %v, want nil", got, err)
		}
	})
}

func TestPasskeyRepo_UpdateCredential(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestPasskeyRepo(t)
	id, _ := repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop"))
	usedAt := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)

	if err := repo.UpdateCredential(ctx, id, `{"signCount":5}`, usedAt); err != nil {
		t.Fatalf("UpdateCredential() error = %v", err)
	}

	got, _ := repo.FindByCredentialID(ctx, []byte("cred-a"))
	if got.CredentialJSON != `{"signCount":5}` || !got.LastUsedAt.Equal(usedAt) {
		t.Errorf("passkey = %+v, want the new credential and last use %v", got, usedAt)
	}
}

func TestPasskeyRepo_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("removes one passkey of the user", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)
		id, _ := repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop"))
		_, _ = repo.Store(ctx, samplePasskey(1, "cred-b", "Phone"))

		deleted, err := repo.Delete(ctx, 1, id)

		left, _ := repo.List(ctx, 1)
		if err != nil || !deleted || len(left) != 1 || left[0].Name != "Phone" {
			t.Errorf("Delete() = %v, %v, left %+v, want only Phone left", deleted, err, left)
		}
	})

	t.Run("does not remove another user's passkey", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)
		id, _ := repo.Store(ctx, samplePasskey(2, "cred-c", "Other user"))

		deleted, err := repo.Delete(ctx, 1, id)

		if err != nil || deleted {
			t.Errorf("Delete() = %v, %v, want false", deleted, err)
		}
	})

	t.Run("reports an unknown id as not deleted", func(t *testing.T) {
		repo, _ := newTestPasskeyRepo(t)

		deleted, err := repo.Delete(ctx, 1, 999)

		if err != nil || deleted {
			t.Errorf("Delete() = %v, %v, want false", deleted, err)
		}
	})
}

func TestPasskeyRepo_DeleteAll(t *testing.T) {
	ctx := context.Background()
	repo, _ := newTestPasskeyRepo(t)
	_, _ = repo.Store(ctx, samplePasskey(1, "cred-a", "Laptop"))
	_, _ = repo.Store(ctx, samplePasskey(1, "cred-b", "Phone"))
	_, _ = repo.Store(ctx, samplePasskey(2, "cred-c", "Other user"))

	removed, err := repo.DeleteAll(ctx, 1)

	mine, _ := repo.List(ctx, 1)
	theirs, _ := repo.List(ctx, 2)
	if err != nil || removed != 2 || len(mine) != 0 || len(theirs) != 1 {
		t.Errorf("DeleteAll() = %d, %v, mine %d, theirs %d, want 2 removed and the other user's kept", removed, err, len(mine), len(theirs))
	}
}

func TestSQLiteMigrationCreatesThePasskeysTable(t *testing.T) {
	db := newTestDB(t)
	mustExec(t, db, `DROP TABLE passkeys`)
	if _, err := db.handler.Exec(`PRAGMA user_version = ` + strconv.Itoa(len(sqliteMigrations)-1)); err != nil {
		t.Fatal(err)
	}

	if err := db.migrateSQLite(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mustExec(t, db, `INSERT INTO users (username, password) VALUES ('admin', 'hash')`)
	repo := &PasskeyRepo{log: zerolog.Nop(), db: db}
	if _, err := repo.Store(context.Background(), samplePasskey(1, "cred-a", "Laptop")); err != nil {
		t.Errorf("Store() after migrating = %v", err)
	}
}
