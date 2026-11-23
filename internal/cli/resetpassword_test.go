package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/auth"
)

type fakeResetter struct {
	err   error
	calls []string
}

func (f *fakeResetter) ResetPassword(_ context.Context, next string) error {
	f.calls = append(f.calls, next)
	return f.err
}

type fakeRevoker struct {
	removed int
	err     error
	calls   int
}

func (f *fakeRevoker) DeleteAll(context.Context) (int, error) {
	f.calls++
	return f.removed, f.err
}

type fixedSource struct {
	password string
	err      error
}

func (s fixedSource) NewPassword() (string, error) { return s.password, s.err }

func TestPipeSource(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "reads the first line", input: "correct horse battery\nignored\n", want: "correct horse battery"},
		{name: "accepts input without a trailing newline", input: "correct horse battery", want: "correct horse battery"},
		{name: "strips a carriage return", input: "correct horse battery\r\n", want: "correct horse battery"},
		{name: "keeps inner and leading spaces", input: "  spaced out  \n", want: "  spaced out  "},
		{name: "rejects empty input", input: "", wantErr: true},
		{name: "rejects an empty line", input: "\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pipeSource{in: strings.NewReader(tt.input)}.NewPassword()

			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("NewPassword() = %q, %v, want %q, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestTerminalSource(t *testing.T) {
	scripted := func(answers ...string) func(string) (string, error) {
		return func(string) (string, error) {
			answer := answers[0]
			answers = answers[1:]
			return answer, nil
		}
	}

	t.Run("returns the password when both entries match", func(t *testing.T) {
		got, err := terminalSource{readSecret: scripted("long-enough-1", "long-enough-1")}.NewPassword()

		if err != nil || got != "long-enough-1" {
			t.Errorf("NewPassword() = %q, %v, want long-enough-1", got, err)
		}
	})

	t.Run("rejects entries that differ", func(t *testing.T) {
		_, err := terminalSource{readSecret: scripted("long-enough-1", "long-enough-2")}.NewPassword()

		if err == nil {
			t.Error("NewPassword() error = nil, want a mismatch error")
		}
	})

	t.Run("passes read failures on", func(t *testing.T) {
		failing := func(string) (string, error) { return "", errors.New("no terminal") }

		_, err := terminalSource{readSecret: failing}.NewPassword()

		if err == nil {
			t.Error("NewPassword() error = nil, want the read error")
		}
	})
}

func TestResetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the password and says so without echoing it", func(t *testing.T) {
		resetter, out := &fakeResetter{}, &bytes.Buffer{}

		err := ResetPassword(ctx, Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, fixedSource{password: "long-enough-1"}, out)

		if err != nil {
			t.Fatalf("ResetPassword() error = %v", err)
		}
		if len(resetter.calls) != 1 || resetter.calls[0] != "long-enough-1" {
			t.Errorf("ResetPassword calls = %v, want one with the new password", resetter.calls)
		}
		if !strings.Contains(out.String(), "Password updated") || strings.Contains(out.String(), "long-enough-1") {
			t.Errorf("output = %q, want a confirmation that does not echo the password", out.String())
		}
	})

	t.Run("explains a password that is too short", func(t *testing.T) {
		resetter := &fakeResetter{err: auth.ErrPasswordTooShort}

		err := ResetPassword(ctx, Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, fixedSource{password: "short"}, &bytes.Buffer{})

		if err == nil || err.Error() != "password must be at least 8 characters" {
			t.Errorf("ResetPassword() error = %v, want a lower-case statement of the minimum length", err)
		}
	})

	t.Run("points at the web UI when there is no user yet", func(t *testing.T) {
		resetter := &fakeResetter{err: auth.ErrNoUser}

		err := ResetPassword(ctx, Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, fixedSource{password: "long-enough-1"}, &bytes.Buffer{})

		if err == nil || !strings.Contains(err.Error(), "web UI") {
			t.Errorf("ResetPassword() error = %v, want it to point at the web UI", err)
		}
	})

	t.Run("does not touch the password when reading it fails", func(t *testing.T) {
		resetter := &fakeResetter{}

		err := ResetPassword(ctx, Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, fixedSource{err: errors.New("empty")}, &bytes.Buffer{})

		if err == nil || len(resetter.calls) != 0 {
			t.Errorf("error = %v, calls = %v, want an error and no call", err, resetter.calls)
		}
	})
}

func TestRunResetPassword(t *testing.T) {
	stdinWith := func(t *testing.T, content string) *os.File {
		t.Helper()
		file, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(content); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { file.Close() })
		return file
	}

	t.Run("reads a piped password and exits 0", func(t *testing.T) {
		resetter, stdout, stderr := &fakeResetter{}, &bytes.Buffer{}, &bytes.Buffer{}

		code := RunResetPassword(context.Background(), Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, stdinWith(t, "long-enough-1\n"), stdout, stderr)

		if code != 0 || len(resetter.calls) != 1 || resetter.calls[0] != "long-enough-1" {
			t.Errorf("exit = %d, calls = %v, stderr = %q, want 0 and the piped password", code, resetter.calls, stderr.String())
		}
	})

	t.Run("prints the reason and exits 1 when the reset fails", func(t *testing.T) {
		resetter, stdout, stderr := &fakeResetter{err: auth.ErrNoUser}, &bytes.Buffer{}, &bytes.Buffer{}

		code := RunResetPassword(context.Background(), Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, stdinWith(t, "long-enough-1\n"), stdout, stderr)

		if code != 1 || !strings.Contains(stderr.String(), "web UI") {
			t.Errorf("exit = %d, stderr = %q, want 1 and a hint about the web UI", code, stderr.String())
		}
	})

	t.Run("exits 1 without calling the service on empty stdin", func(t *testing.T) {
		resetter, stderr := &fakeResetter{}, &bytes.Buffer{}

		code := RunResetPassword(context.Background(), Reset{Passwords: resetter, Passkeys: &fakeRevoker{}}, stdinWith(t, ""), &bytes.Buffer{}, stderr)

		if code != 1 || len(resetter.calls) != 0 {
			t.Errorf("exit = %d, calls = %v, want 1 and no call", code, resetter.calls)
		}
	})
}

func TestResetPassword_passkeys(t *testing.T) {
	ctx := context.Background()
	source := fixedSource{password: "long-enough-1"}

	t.Run("removes the passkeys after resetting and says how many", func(t *testing.T) {
		revoker, out := &fakeRevoker{removed: 2}, &bytes.Buffer{}

		err := ResetPassword(ctx, Reset{Passwords: &fakeResetter{}, Passkeys: revoker}, source, out)

		if err != nil || revoker.calls != 1 || !strings.Contains(out.String(), "Removed 2 passkeys") {
			t.Errorf("error = %v, revoke calls = %d, output = %q, want the passkeys removed and reported", err, revoker.calls, out.String())
		}
	})

	t.Run("says nothing about passkeys when there were none", func(t *testing.T) {
		out := &bytes.Buffer{}

		_ = ResetPassword(ctx, Reset{Passwords: &fakeResetter{}, Passkeys: &fakeRevoker{}}, source, out)

		if strings.Contains(out.String(), "passkey") {
			t.Errorf("output = %q, want no passkey line", out.String())
		}
	})

	t.Run("keeps the passkeys when asked to", func(t *testing.T) {
		revoker := &fakeRevoker{removed: 2}

		err := ResetPassword(ctx, Reset{Passwords: &fakeResetter{}, Passkeys: revoker, KeepPasskeys: true}, source, &bytes.Buffer{})

		if err != nil || revoker.calls != 0 {
			t.Errorf("error = %v, revoke calls = %d, want the passkeys left alone", err, revoker.calls)
		}
	})

	t.Run("leaves the passkeys alone when the password could not be reset", func(t *testing.T) {
		revoker := &fakeRevoker{removed: 2}

		err := ResetPassword(ctx, Reset{Passwords: &fakeResetter{err: auth.ErrPasswordTooShort}, Passkeys: revoker}, source, &bytes.Buffer{})

		if err == nil || revoker.calls != 0 {
			t.Errorf("error = %v, revoke calls = %d, want a failure before any passkey is touched", err, revoker.calls)
		}
	})

	t.Run("reports a failed revocation after the password changed", func(t *testing.T) {
		revoker := &fakeRevoker{err: errors.New("database is read-only")}
		out := &bytes.Buffer{}

		err := ResetPassword(ctx, Reset{Passwords: &fakeResetter{}, Passkeys: revoker}, source, out)

		if err == nil || !strings.Contains(err.Error(), "passkeys") || !strings.Contains(out.String(), "Password updated") {
			t.Errorf("error = %v, output = %q, want the password update confirmed and an error about the passkeys", err, out.String())
		}
	})
}
