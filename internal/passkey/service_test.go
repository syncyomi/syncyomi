package passkey

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

const (
	testOrigin = "https://sync.example.com"
	testRPID   = "sync.example.com"
)

type fakeRepo struct {
	findErr   error
	listErr   error
	updateErr error
	mu        sync.Mutex
	passkeys  []domain.Passkey
	nextID    int
}

func (f *fakeRepo) List(_ context.Context, userID int) ([]domain.Passkey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []domain.Passkey
	for _, p := range f.passkeys {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepo) FindByCredentialID(_ context.Context, credentialID []byte) (*domain.Passkey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return nil, f.findErr
	}
	for _, p := range f.passkeys {
		if string(p.CredentialID) == string(credentialID) {
			found := p
			return &found, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) Store(_ context.Context, passkey domain.Passkey) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	passkey.ID = f.nextID
	passkey.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.passkeys = append(f.passkeys, passkey)
	return passkey.ID, nil
}

func (f *fakeRepo) UpdateCredential(_ context.Context, id int, credentialJSON string, usedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateErr != nil {
		return f.updateErr
	}
	for i := range f.passkeys {
		if f.passkeys[i].ID == id {
			f.passkeys[i].CredentialJSON = credentialJSON
			f.passkeys[i].LastUsedAt = usedAt
		}
	}
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, userID, id int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	before := len(f.passkeys)
	f.passkeys = slices.DeleteFunc(f.passkeys, func(p domain.Passkey) bool { return p.UserID == userID && p.ID == id })
	return len(f.passkeys) < before, nil
}

func (f *fakeRepo) DeleteAll(_ context.Context, userID int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	before := len(f.passkeys)
	f.passkeys = slices.DeleteFunc(f.passkeys, func(p domain.Passkey) bool { return p.UserID == userID })
	return before - len(f.passkeys), nil
}

type fakeUsers struct {
	user *domain.User
	err  error
}

func (f *fakeUsers) FindFirst(context.Context) (*domain.User, error) { return f.user, f.err }

type fakeNotifier struct {
	mu     sync.Mutex
	events []domain.NotificationPayload
}

func (n *fakeNotifier) Send(_ domain.NotificationEvent, payload domain.NotificationPayload) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, payload)
}

type fixture struct {
	svc      *Service
	repo     *fakeRepo
	users    *fakeUsers
	notifier *fakeNotifier
	now      *time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixtureWith(t, Config{Origin: testOrigin, RPName: "SyncYomi"}, &domain.User{ID: 1, Username: "admin", Password: "hash"})
}

func newFixtureWith(t *testing.T, cfg Config, user *domain.User) *fixture {
	t.Helper()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo, notifier, users := &fakeRepo{}, &fakeNotifier{}, &fakeUsers{user: user}
	svc, err := newService(cfg, repo, users, notifier, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}

	return &fixture{svc: svc, repo: repo, users: users, notifier: notifier, now: &now}
}

func (f *fixture) register(t *testing.T, name string) *softAuthenticator {
	t.Helper()
	ctx := context.Background()
	authenticator := newSoftAuthenticator(t, testRPID, testOrigin)

	id, creation, err := f.svc.BeginRegistration(ctx, name)
	if err != nil {
		t.Fatalf("BeginRegistration() error = %v", err)
	}
	if _, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation)); err != nil {
		t.Fatalf("FinishRegistration() error = %v", err)
	}

	return authenticator
}

func (f *fixture) login(t *testing.T, authenticator *softAuthenticator) (*domain.User, error) {
	t.Helper()

	id, assertion, err := f.svc.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin() error = %v", err)
	}

	return f.svc.FinishLogin(context.Background(), id, authenticator.assert(t, assertion))
}

func TestService_registration(t *testing.T) {
	ctx := context.Background()

	t.Run("stores a passkey and announces it", func(t *testing.T) {
		f := newFixture(t)

		f.register(t, "  Laptop  ")

		list, err := f.svc.List(ctx)
		if err != nil || len(list) != 1 || list[0].Name != "Laptop" || list[0].UserID != 1 {
			t.Fatalf("List() = %+v, %v, want one passkey named Laptop", list, err)
		}
		events := f.notifier.events
		if len(events) != 1 || events[0].Event != domain.NotificationEventPasskeyAdded || !strings.Contains(events[0].Message, "Laptop") {
			t.Errorf("notifications = %+v, want one PASSKEY_ADDED naming Laptop", events)
		}
	})

	t.Run("asks for a user-verified discoverable credential for this site only", func(t *testing.T) {
		f := newFixture(t)

		_, creation, err := f.svc.BeginRegistration(ctx, "Laptop")

		if err != nil {
			t.Fatalf("BeginRegistration() error = %v", err)
		}
		selection := creation.Response.AuthenticatorSelection
		if string(selection.UserVerification) != "required" || string(selection.ResidentKey) != "required" || creation.Response.RelyingParty.ID != testRPID {
			t.Errorf("selection = %+v, rp = %+v, want required user verification and resident key for %s", selection, creation.Response.RelyingParty, testRPID)
		}
		if string(creation.Response.Attestation) != "none" {
			t.Errorf("attestation = %q, want none", creation.Response.Attestation)
		}
	})

	t.Run("excludes passkeys that are already registered", func(t *testing.T) {
		f := newFixture(t)
		first := f.register(t, "Laptop")

		_, creation, _ := f.svc.BeginRegistration(ctx, "Phone")

		if len(creation.Response.CredentialExcludeList) != 1 || string(creation.Response.CredentialExcludeList[0].CredentialID) != string(first.credentialID) {
			t.Errorf("exclude list = %+v, want the Laptop credential", creation.Response.CredentialExcludeList)
		}
	})

	t.Run("rejects an empty or over-long name", func(t *testing.T) {
		f := newFixture(t)

		for _, name := range []string{"", "   ", strings.Repeat("a", MaxNameLength+1)} {
			if _, _, err := f.svc.BeginRegistration(ctx, name); !errors.Is(err, ErrInvalidName) {
				t.Errorf("BeginRegistration(%q) error = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("stops at the passkey limit", func(t *testing.T) {
		f := newFixture(t)
		for range MaxPasskeys {
			f.repo.passkeys = append(f.repo.passkeys, domain.Passkey{ID: len(f.repo.passkeys) + 1, UserID: 1})
		}

		_, _, err := f.svc.BeginRegistration(ctx, "One too many")

		if !errors.Is(err, ErrLimit) {
			t.Errorf("BeginRegistration() error = %v, want ErrLimit", err)
		}
	})

	t.Run("refuses a registration made for another origin", func(t *testing.T) {
		f := newFixture(t)
		attacker := newSoftAuthenticator(t, testRPID, "https://evil.example")
		id, creation, _ := f.svc.BeginRegistration(ctx, "Laptop")

		_, err := f.svc.FinishRegistration(ctx, id, attacker.register(t, creation))

		if !errors.Is(err, ErrRejected) || len(f.repo.passkeys) != 0 {
			t.Errorf("FinishRegistration() error = %v, stored %d, want ErrRejected and nothing stored", err, len(f.repo.passkeys))
		}
	})

	t.Run("accepts each registration ceremony once", func(t *testing.T) {
		f := newFixture(t)
		authenticator := newSoftAuthenticator(t, testRPID, testOrigin)
		id, creation, _ := f.svc.BeginRegistration(ctx, "Laptop")
		if _, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation)); err != nil {
			t.Fatal(err)
		}

		_, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation))

		if !errors.Is(err, ErrCeremonyNotFound) {
			t.Errorf("FinishRegistration() error = %v, want ErrCeremonyNotFound", err)
		}
	})
}

func TestService_login(t *testing.T) {
	t.Run("signs in with a registered passkey and records the use", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.signCount = 1

		user, err := f.login(t, authenticator)

		if err != nil || user == nil || user.ID != 1 {
			t.Fatalf("FinishLogin() = %+v, %v, want the user", user, err)
		}
		stored := f.repo.passkeys[0]
		if !stored.LastUsedAt.Equal(*f.now) || !strings.Contains(stored.CredentialJSON, `"signCount":1`) {
			t.Errorf("stored passkey = %+v, want last use %v and sign count 1", stored, *f.now)
		}
	})

	t.Run("accepts each login ceremony once", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.signCount = 1
		id, assertion, _ := f.svc.BeginLogin()
		if _, err := f.svc.FinishLogin(context.Background(), id, authenticator.assert(t, assertion)); err != nil {
			t.Fatal(err)
		}

		authenticator.signCount = 2
		_, err := f.svc.FinishLogin(context.Background(), id, authenticator.assert(t, assertion))

		if !errors.Is(err, ErrCeremonyNotFound) {
			t.Errorf("FinishLogin() error = %v, want ErrCeremonyNotFound", err)
		}
	})

	t.Run("expires a login ceremony", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		id, assertion, _ := f.svc.BeginLogin()
		*f.now = f.now.Add(CeremonyTTL + time.Second)

		_, err := f.svc.FinishLogin(context.Background(), id, authenticator.assert(t, assertion))

		if !errors.Is(err, ErrCeremonyNotFound) {
			t.Errorf("FinishLogin() error = %v, want ErrCeremonyNotFound", err)
		}
	})

	t.Run("rejects a credential the server does not know", func(t *testing.T) {
		f := newFixture(t)
		f.register(t, "Laptop")
		stranger := newSoftAuthenticator(t, testRPID, testOrigin)
		stranger.userHandle = []byte("whatever")

		_, err := f.login(t, stranger)

		if !errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want ErrRejected", err)
		}
	})

	t.Run("rejects an assertion made for another origin", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.origin = "https://evil.example"

		_, err := f.login(t, authenticator)

		if !errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want ErrRejected", err)
		}
	})

	t.Run("rejects an authenticator whose counter went backwards", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.signCount = 10
		if _, err := f.login(t, authenticator); err != nil {
			t.Fatal(err)
		}

		authenticator.signCount = 3
		_, err := f.login(t, authenticator)

		if !errors.Is(err, ErrCloned) {
			t.Errorf("FinishLogin() error = %v, want ErrCloned", err)
		}
	})
}

func TestService_userVerificationIsRequired(t *testing.T) {
	ctx := context.Background()

	t.Run("asks for it when signing in", func(t *testing.T) {
		f := newFixture(t)

		_, assertion, err := f.svc.BeginLogin()

		if err != nil || string(assertion.Response.UserVerification) != "required" {
			t.Errorf("BeginLogin() = %+v, %v, want user verification required", assertion, err)
		}
	})

	t.Run("refuses a registration made without it", func(t *testing.T) {
		f := newFixture(t)
		authenticator := newSoftAuthenticator(t, testRPID, testOrigin)
		authenticator.skipVerified = true
		id, creation, _ := f.svc.BeginRegistration(ctx, "Laptop")

		_, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation))

		if !errors.Is(err, ErrRejected) {
			t.Errorf("FinishRegistration() error = %v, want ErrRejected", err)
		}
	})

	t.Run("refuses a sign-in made without it", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.skipVerified = true

		_, err := f.login(t, authenticator)

		if !errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want ErrRejected", err)
		}
	})
}

func TestService_ceremoniesStayApart(t *testing.T) {
	ctx := context.Background()

	t.Run("a registration ceremony cannot finish a login", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		id, _, _ := f.svc.BeginRegistration(ctx, "Phone")
		_, assertion, _ := f.svc.BeginLogin()

		_, err := f.svc.FinishLogin(ctx, id, authenticator.assert(t, assertion))

		if !errors.Is(err, ErrCeremonyNotFound) {
			t.Errorf("FinishLogin() error = %v, want ErrCeremonyNotFound", err)
		}
	})

	t.Run("a login ceremony cannot finish a registration", func(t *testing.T) {
		f := newFixture(t)
		id, _, _ := f.svc.BeginLogin()
		_, creation, _ := f.svc.BeginRegistration(ctx, "Phone")
		authenticator := newSoftAuthenticator(t, testRPID, testOrigin)

		_, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation))

		if !errors.Is(err, ErrCeremonyNotFound) {
			t.Errorf("FinishRegistration() error = %v, want ErrCeremonyNotFound", err)
		}
	})

	t.Run("a flood of sign-in ceremonies cannot push out a registration in progress", func(t *testing.T) {
		f := newFixture(t)
		authenticator := newSoftAuthenticator(t, testRPID, testOrigin)
		id, creation, _ := f.svc.BeginRegistration(ctx, "Laptop")
		for range maxCeremonies + 50 {
			if _, _, err := f.svc.BeginLogin(); err != nil {
				t.Fatal(err)
			}
		}

		_, err := f.svc.FinishRegistration(ctx, id, authenticator.register(t, creation))

		if err != nil {
			t.Errorf("FinishRegistration() error = %v, want it to survive the flood", err)
		}
	})
}

func TestService_storageFailures(t *testing.T) {
	t.Run("a failing credential lookup is a server error, not a rejected passkey", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		f.repo.findErr = errors.New("database is down")

		_, err := f.login(t, authenticator)

		if err == nil || errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want a non-rejection error", err)
		}
	})

	t.Run("a failing user lookup is a server error, not a rejected passkey", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		f.users.err = errors.New("database is down")

		_, err := f.login(t, authenticator)

		if err == nil || errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want a non-rejection error", err)
		}
	})

	t.Run("a failing passkey listing is a server error, not a rejected passkey", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		f.repo.listErr = errors.New("database is down")

		_, err := f.login(t, authenticator)

		if err == nil || errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want a non-rejection error", err)
		}
	})

	t.Run("rejects the passkey when no user exists any more", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		f.users.user = nil

		_, err := f.login(t, authenticator)

		if !errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want ErrRejected", err)
		}
	})

	t.Run("fails the sign-in when the new sign count cannot be stored", func(t *testing.T) {
		f := newFixture(t)
		authenticator := f.register(t, "Laptop")
		authenticator.signCount = 1
		f.repo.updateErr = errors.New("database is read-only")

		_, err := f.login(t, authenticator)

		if err == nil || errors.Is(err, ErrRejected) {
			t.Errorf("FinishLogin() error = %v, want a server error so the counter is never left stale", err)
		}
	})
}

func TestService_management(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes one passkey of the user", func(t *testing.T) {
		f := newFixture(t)
		f.register(t, "Laptop")
		f.register(t, "Phone")
		list, _ := f.svc.List(ctx)

		deleted, err := f.svc.Delete(ctx, list[0].ID)

		left, _ := f.svc.List(ctx)
		if err != nil || !deleted || len(left) != 1 || left[0].Name != "Phone" {
			t.Errorf("Delete() = %v, %v, left %+v, want only Phone", deleted, err, left)
		}
	})

	t.Run("reports an unknown id as not deleted", func(t *testing.T) {
		f := newFixture(t)

		deleted, err := f.svc.Delete(ctx, 99)

		if err != nil || deleted {
			t.Errorf("Delete() = %v, %v, want false", deleted, err)
		}
	})

	t.Run("revokes every passkey", func(t *testing.T) {
		f := newFixture(t)
		f.register(t, "Laptop")
		f.register(t, "Phone")

		removed, err := f.svc.DeleteAll(ctx)

		left, _ := f.svc.List(ctx)
		if err != nil || removed != 2 || len(left) != 0 {
			t.Errorf("DeleteAll() = %d, %v, left %d, want 2 removed", removed, err, len(left))
		}
	})
}

func TestService_disabled(t *testing.T) {
	ctx := context.Background()
	f := newFixtureWith(t, Config{}, &domain.User{ID: 1, Username: "admin"})
	f.repo.passkeys = []domain.Passkey{{ID: 1, UserID: 1, Name: "Laptop"}}

	t.Run("reports itself disabled and refuses ceremonies", func(t *testing.T) {
		if f.svc.Enabled() {
			t.Error("Enabled() = true without an origin")
		}
		if _, _, err := f.svc.BeginRegistration(ctx, "Laptop"); !errors.Is(err, ErrDisabled) {
			t.Errorf("BeginRegistration() error = %v, want ErrDisabled", err)
		}
		if _, _, err := f.svc.BeginLogin(); !errors.Is(err, ErrDisabled) {
			t.Errorf("BeginLogin() error = %v, want ErrDisabled", err)
		}
	})

	t.Run("can still revoke passkeys so a reset works without the setting", func(t *testing.T) {
		removed, err := f.svc.DeleteAll(ctx)

		if err != nil || removed != 1 {
			t.Errorf("DeleteAll() = %d, %v, want 1 removed", removed, err)
		}
	})
}

func TestService_withoutAUser(t *testing.T) {
	f := newFixtureWith(t, Config{Origin: testOrigin, RPName: "SyncYomi"}, nil)

	if _, _, err := f.svc.BeginRegistration(context.Background(), "Laptop"); !errors.Is(err, ErrNoUser) {
		t.Errorf("BeginRegistration() error = %v, want ErrNoUser", err)
	}
}
