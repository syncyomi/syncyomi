package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/pkg/argon2id"
	"github.com/r3labs/sse/v2"
	"github.com/rs/zerolog"
)

const (
	oldPassword = "old-password-1"
	newPassword = "brand-new-pass"
)

type fakeUsers struct {
	mu      sync.Mutex
	user    *domain.User
	updates int
	findErr error
}

func (f *fakeUsers) GetUserCount(context.Context) (int, error) {
	if f.user == nil {
		return 0, nil
	}
	return 1, nil
}

func (f *fakeUsers) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.user == nil || f.user.Username != username {
		return nil, nil
	}
	copied := *f.user
	return &copied, nil
}

func (f *fakeUsers) FindFirst(context.Context) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.user == nil {
		return nil, nil
	}
	copied := *f.user
	return &copied, nil
}

func (f *fakeUsers) UpdatePassword(_ context.Context, id int, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	f.user.Password = hash
	return nil
}

func (f *fakeUsers) CreateUser(_ context.Context, user domain.User) error {
	f.user = &user
	return nil
}

func (f *fakeUsers) storedHash() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.user.Password
}

type fakeNotifier struct {
	mu     sync.Mutex
	events []domain.NotificationPayload
}

func (n *fakeNotifier) Send(_ domain.NotificationEvent, payload domain.NotificationPayload) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, payload)
}

func (n *fakeNotifier) sent() []domain.NotificationPayload {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]domain.NotificationPayload(nil), n.events...)
}

type bufferLogger struct {
	zerolog.Logger
}

func (l bufferLogger) Log() *zerolog.Event   { return l.Logger.Log() }
func (l bufferLogger) Fatal() *zerolog.Event { return l.Logger.Fatal() }
func (l bufferLogger) Err(err error) *zerolog.Event {
	return l.Logger.Err(err)
}
func (l bufferLogger) Error() *zerolog.Event         { return l.Logger.Error() }
func (l bufferLogger) Warn() *zerolog.Event          { return l.Logger.Warn() }
func (l bufferLogger) Info() *zerolog.Event          { return l.Logger.Info() }
func (l bufferLogger) Trace() *zerolog.Event         { return l.Logger.Trace() }
func (l bufferLogger) Debug() *zerolog.Event         { return l.Logger.Debug() }
func (l bufferLogger) With() zerolog.Context         { return l.Logger.With() }
func (l bufferLogger) RegisterSSEWriter(*sse.Server) {}
func (l bufferLogger) SetLogLevel(string)            {}

type fixture struct {
	svc      *service
	users    *fakeUsers
	notifier *fakeNotifier
	logs     *bytes.Buffer
}

func newFixture(t *testing.T, withUser bool) *fixture {
	t.Helper()

	users := &fakeUsers{}
	if withUser {
		hash, err := argon2id.CreateHash(oldPassword, argon2id.DefaultParams)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		users.user = &domain.User{ID: 1, Username: "admin", Password: hash}
	}

	logs := &bytes.Buffer{}
	notifier := &fakeNotifier{}

	return &fixture{
		svc:      newService(bufferLogger{zerolog.New(logs)}, users, notifier, hashSlots, time.Second),
		users:    users,
		notifier: notifier,
		logs:     logs,
	}
}

func saturateHashSlots(t *testing.T, f *fixture) {
	t.Helper()
	if err := f.svc.slots.Acquire(context.Background(), hashSlots); err != nil {
		t.Fatalf("acquire hash slots: %v", err)
	}
}

func matches(t *testing.T, password, hash string) bool {
	t.Helper()
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	return ok
}

func TestService_ChangePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("stores a hash of the new password and notifies", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ChangePassword(ctx, oldPassword, newPassword)

		if err != nil {
			t.Fatalf("ChangePassword() error = %v", err)
		}
		if !matches(t, newPassword, f.users.storedHash()) {
			t.Error("stored hash does not verify the new password")
		}
		events := f.notifier.sent()
		if len(events) != 1 || events[0].Event != domain.NotificationEventPasswordChanged {
			t.Errorf("notifications = %+v, want one PASSWORD_CHANGED", events)
		}
	})

	t.Run("rejects a wrong current password and changes nothing", func(t *testing.T) {
		f := newFixture(t, true)
		before := f.users.storedHash()

		err := f.svc.ChangePassword(ctx, "not-the-password", newPassword)

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("ChangePassword() error = %v, want ErrInvalidCredentials", err)
		}
		if f.users.storedHash() != before || f.users.updates != 0 {
			t.Error("password was updated despite a wrong current password")
		}
		if got := f.notifier.sent(); len(got) != 0 {
			t.Errorf("notifications = %+v, want none", got)
		}
	})

	t.Run("rejects an empty current password", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ChangePassword(ctx, "", newPassword)

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("ChangePassword() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("rejects a new password that is too short", func(t *testing.T) {
		for _, next := range []string{"", "short", "1234567"} {
			f := newFixture(t, true)

			err := f.svc.ChangePassword(ctx, oldPassword, next)

			if !errors.Is(err, ErrPasswordTooShort) {
				t.Errorf("ChangePassword(%q) error = %v, want ErrPasswordTooShort", next, err)
			}
			if f.users.updates != 0 {
				t.Errorf("ChangePassword(%q) updated the password", next)
			}
		}
	})

	t.Run("counts characters, not bytes, for the minimum length", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ChangePassword(ctx, oldPassword, "密码很短")

		if !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("ChangePassword() error = %v, want ErrPasswordTooShort for 4 characters (12 bytes)", err)
		}
	})

	t.Run("rejects a new password that is too long", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ChangePassword(ctx, oldPassword, strings.Repeat("a", MaxPasswordBytes+1))

		if !errors.Is(err, ErrPasswordTooLong) {
			t.Errorf("ChangePassword() error = %v, want ErrPasswordTooLong", err)
		}
	})

	t.Run("fails when no user exists", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.svc.ChangePassword(ctx, oldPassword, newPassword)

		if !errors.Is(err, ErrNoUser) {
			t.Errorf("ChangePassword() error = %v, want ErrNoUser", err)
		}
	})

	t.Run("gives up when the request is cancelled while waiting to hash", func(t *testing.T) {
		f := newFixture(t, true)
		saturateHashSlots(t, f)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		err := f.svc.ChangePassword(cancelled, oldPassword, newPassword)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("ChangePassword() error = %v, want context.Canceled", err)
		}
		if f.users.updates != 0 {
			t.Error("password was updated by a cancelled request")
		}
	})

	t.Run("never writes a password or hash to the log", func(t *testing.T) {
		f := newFixture(t, true)
		original := f.users.storedHash()

		_ = f.svc.ChangePassword(ctx, "wrong-current-pw", newPassword)
		_ = f.svc.ChangePassword(ctx, oldPassword, newPassword)
		_, _ = f.svc.Login(ctx, "admin", "wrong-login-pw")

		for _, secret := range []string{"wrong-current-pw", oldPassword, newPassword, "wrong-login-pw", original, f.users.storedHash()} {
			if strings.Contains(f.logs.String(), secret) {
				t.Errorf("log output contains secret %q: %s", secret, f.logs.String())
			}
		}
	})
}

func TestService_ResetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the password without knowing the current one", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ResetPassword(ctx, newPassword)

		if err != nil {
			t.Fatalf("ResetPassword() error = %v", err)
		}
		if !matches(t, newPassword, f.users.storedHash()) {
			t.Error("stored hash does not verify the new password")
		}
	})

	t.Run("signs every session out by changing the stamp", func(t *testing.T) {
		f := newFixture(t, true)
		before, _ := f.svc.CurrentPasswordStamp(ctx)

		if err := f.svc.ResetPassword(ctx, newPassword); err != nil {
			t.Fatalf("ResetPassword() error = %v", err)
		}
		after, _ := f.svc.CurrentPasswordStamp(ctx)

		if before == after {
			t.Error("password stamp is unchanged after a reset, want a new stamp")
		}
	})

	t.Run("rejects a password that is too short", func(t *testing.T) {
		f := newFixture(t, true)

		err := f.svc.ResetPassword(ctx, "short")

		if !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("ResetPassword() error = %v, want ErrPasswordTooShort", err)
		}
	})

	t.Run("fails when no user exists", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.svc.ResetPassword(ctx, newPassword)

		if !errors.Is(err, ErrNoUser) {
			t.Errorf("ResetPassword() error = %v, want ErrNoUser", err)
		}
	})
}

func TestService_CurrentPasswordStamp(t *testing.T) {
	ctx := context.Background()

	t.Run("changes after the password changes", func(t *testing.T) {
		f := newFixture(t, true)
		before, err := f.svc.CurrentPasswordStamp(ctx)
		if err != nil {
			t.Fatalf("CurrentPasswordStamp() error = %v", err)
		}

		if err := f.svc.ChangePassword(ctx, oldPassword, newPassword); err != nil {
			t.Fatalf("ChangePassword() error = %v", err)
		}
		after, err := f.svc.CurrentPasswordStamp(ctx)

		if err != nil || before == "" || before == after {
			t.Errorf("stamps before=%q after=%q err=%v, want two different non-empty stamps", before, after, err)
		}
	})

	t.Run("fails when no user exists", func(t *testing.T) {
		f := newFixture(t, false)

		_, err := f.svc.CurrentPasswordStamp(ctx)

		if !errors.Is(err, ErrNoUser) {
			t.Errorf("CurrentPasswordStamp() error = %v, want ErrNoUser", err)
		}
	})
}

func TestService_NotifyLoginBlocked(t *testing.T) {
	f := newFixture(t, true)

	f.svc.NotifyLoginBlocked("203.0.113.7")

	events := f.notifier.sent()
	if len(events) != 1 || events[0].Event != domain.NotificationEventLoginBlocked {
		t.Fatalf("notifications = %+v, want one LOGIN_BLOCKED", events)
	}
	if !strings.Contains(events[0].Message, "203.0.113.7") {
		t.Errorf("message = %q, want it to name the blocked address", events[0].Message)
	}
}

func TestService_Login(t *testing.T) {
	ctx := context.Background()

	t.Run("accepts the right password", func(t *testing.T) {
		f := newFixture(t, true)

		if _, err := f.svc.Login(ctx, "admin", oldPassword); err != nil {
			t.Errorf("Login() error = %v", err)
		}
	})

	t.Run("reports bad credentials as ErrInvalidCredentials", func(t *testing.T) {
		tests := []struct{ name, username, password string }{
			{"wrong password", "admin", "wrong-login-pw"},
			{"unknown user", "nobody", oldPassword},
			{"empty username", "", oldPassword},
			{"empty password", "admin", ""},
			{"over-long password", "admin", strings.Repeat("a", MaxPasswordBytes+1)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f := newFixture(t, true)

				_, err := f.svc.Login(ctx, tt.username, tt.password)

				if !errors.Is(err, ErrInvalidCredentials) {
					t.Errorf("Login() error = %v, want ErrInvalidCredentials", err)
				}
			})
		}
	})

	t.Run("spends one password check whether or not the username exists", func(t *testing.T) {
		for _, username := range []string{"admin", "nobody"} {
			f := newFixture(t, true)
			calls := 0
			real := f.svc.compareHash
			f.svc.compareHash = func(password, hash string) (bool, error) {
				calls++
				return real(password, hash)
			}

			_, err := f.svc.Login(ctx, username, "wrong-login-pw")

			if calls != 1 || !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("Login(%q) ran %d password checks and returned %v, want 1 and ErrInvalidCredentials", username, calls, err)
			}
		}
	})

	t.Run("reports a busy server the same way for known and unknown usernames", func(t *testing.T) {
		for _, username := range []string{"admin", "nobody"} {
			f := newFixture(t, true)
			f.svc.slotWait = 10 * time.Millisecond
			saturateHashSlots(t, f)

			_, err := f.svc.Login(ctx, username, "wrong-login-pw")

			if !errors.Is(err, ErrBusy) {
				t.Errorf("Login(%q) error = %v, want ErrBusy", username, err)
			}
		}
	})

	t.Run("does not report a storage failure as bad credentials", func(t *testing.T) {
		f := newFixture(t, true)
		f.users.findErr = errors.New("database is down")

		_, err := f.svc.Login(ctx, "admin", oldPassword)

		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want a non-credential error", err)
		}
	})

	t.Run("reports ErrBusy instead of queuing without limit", func(t *testing.T) {
		f := newFixture(t, true)
		f.svc.slotWait = 10 * time.Millisecond
		saturateHashSlots(t, f)

		_, err := f.svc.Login(ctx, "admin", oldPassword)

		if !errors.Is(err, ErrBusy) {
			t.Errorf("Login() error = %v, want ErrBusy", err)
		}
	})
}

func TestService_CreateUser(t *testing.T) {
	ctx := context.Background()

	t.Run("stores a hash for a valid password", func(t *testing.T) {
		f := newFixture(t, false)

		err := f.svc.CreateUser(ctx, "admin", newPassword)

		if err != nil || !matches(t, newPassword, f.users.storedHash()) {
			t.Errorf("CreateUser() error = %v, want a stored hash of the password", err)
		}
	})

	t.Run("applies the same password policy as a change", func(t *testing.T) {
		tests := []struct {
			name     string
			password string
			want     error
		}{
			{"too short", "short", ErrPasswordTooShort},
			{"too long", strings.Repeat("a", MaxPasswordBytes+1), ErrPasswordTooLong},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f := newFixture(t, false)

				err := f.svc.CreateUser(ctx, "admin", tt.password)

				if !errors.Is(err, tt.want) || f.users.user != nil {
					t.Errorf("CreateUser() error = %v, user stored = %v, want %v and no user", err, f.users.user != nil, tt.want)
				}
			})
		}
	})
}

func TestPolicyMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
		ok   bool
	}{
		{"too short", ErrPasswordTooShort, "Password must be at least 8 characters", true},
		{"too long", ErrPasswordTooLong, "Password must be at most 1024 bytes", true},
		{"wrapped", fmt.Errorf("ctx: %w", ErrPasswordTooShort), "Password must be at least 8 characters", true},
		{"unrelated", errors.New("boom"), "", false},
		{"nil", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PolicyMessage(tt.err)

			if got != tt.want || ok != tt.ok {
				t.Errorf("PolicyMessage() = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
