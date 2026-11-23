package domain

import "testing"

func TestUser_PasswordStamp(t *testing.T) {
	t.Run("is stable for one stored hash", func(t *testing.T) {
		user := User{Password: "$argon2id$v=19$m=65536,t=1,p=2$c2FsdA$a2V5"}

		first, second := user.PasswordStamp(), user.PasswordStamp()

		if first == "" || first != second {
			t.Errorf("PasswordStamp() = %q then %q, want the same non-empty value", first, second)
		}
	})

	t.Run("changes when the stored hash changes", func(t *testing.T) {
		before := User{Password: "$argon2id$v=19$m=65536,t=1,p=2$c2FsdA$a2V5"}
		after := User{Password: "$argon2id$v=19$m=65536,t=1,p=2$c2FsdA$b3RoZXI"}

		if before.PasswordStamp() == after.PasswordStamp() {
			t.Error("PasswordStamp() is equal for different hashes, want different")
		}
	})

	t.Run("is a short fixed length", func(t *testing.T) {
		user := User{Password: "$argon2id$v=19$m=65536,t=1,p=2$c2FsdA$a2V5"}

		if got := user.PasswordStamp(); len(got) != passwordStampLength {
			t.Errorf("PasswordStamp() = %q, want %d characters", got, passwordStampLength)
		}
	})
}

func TestUser_PasswordStampPrefersTheStoredStamp(t *testing.T) {
	t.Run("uses the stored stamp when there is one", func(t *testing.T) {
		user := User{Password: "$argon2id$one", Stamp: "stored-stamp"}

		if got := user.PasswordStamp(); got != "stored-stamp" {
			t.Errorf("PasswordStamp() = %q, want the stored stamp", got)
		}
	})

	t.Run("keeps the stamp when only the hash changes", func(t *testing.T) {
		before := User{Password: "$argon2id$one", Stamp: "stored-stamp"}
		after := User{Password: "$argon2id$two", Stamp: "stored-stamp"}

		if before.PasswordStamp() != after.PasswordStamp() {
			t.Error("PasswordStamp() changed with the hash, want it stable while the stored stamp is set")
		}
	})

	t.Run("falls back to a stamp derived from the hash for existing users", func(t *testing.T) {
		user := User{Password: "$argon2id$one"}

		if got, want := user.PasswordStamp(), user.LegacyPasswordStamp(); got != want || got == "" {
			t.Errorf("PasswordStamp() = %q, want the legacy stamp %q", got, want)
		}
	})

	t.Run("derives the legacy stamp from the hash only", func(t *testing.T) {
		a := User{Password: "$argon2id$one", Stamp: "x"}
		b := User{Password: "$argon2id$one", Stamp: "y"}
		other := User{Password: "$argon2id$two"}

		if a.LegacyPasswordStamp() != b.LegacyPasswordStamp() || a.LegacyPasswordStamp() == other.LegacyPasswordStamp() {
			t.Error("LegacyPasswordStamp() must depend on the hash and nothing else")
		}
	})
}
