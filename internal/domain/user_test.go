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
