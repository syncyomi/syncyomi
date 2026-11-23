package passkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/rs/zerolog"
)

var (
	ErrDisabled         = errors.New("passkeys are not enabled")
	ErrNoUser           = errors.New("no user exists")
	ErrCeremonyNotFound = errors.New("the passkey request expired or was already used")
	ErrRejected         = errors.New("the passkey was rejected")
	ErrCloned           = errors.New("the authenticator looks cloned")
	ErrInvalidName      = errors.New("the passkey name is empty or too long")
	ErrLimit            = errors.New("the passkey limit is reached")

	errStorage = errors.New("storage failure")
)

const (
	MaxPasskeys   = 10
	MaxNameLength = 64
	CeremonyTTL   = 5 * time.Minute
	ceremonyBytes = 32
)

type Config struct {
	Origin string
	RPName string
}

type UserSource interface {
	FindFirst(ctx context.Context) (*domain.User, error)
}

type notifier interface {
	Send(event domain.NotificationEvent, payload domain.NotificationPayload)
}

type Service struct {
	log           zerolog.Logger
	wa            *webauthn.WebAuthn
	repo          domain.PasskeyRepo
	users         UserSource
	notifier      notifier
	registrations *ceremonyStore
	logins        *ceremonyStore
	now           func() time.Time
}

func NewService(log logger.Logger, cfg Config, repo domain.PasskeyRepo, users UserSource, notifier notifier) (*Service, error) {
	s, err := newService(cfg, repo, users, notifier, time.Now)
	if err != nil {
		return nil, err
	}
	s.log = log.With().Str("module", "passkey").Logger()

	return s, nil
}

func newService(cfg Config, repo domain.PasskeyRepo, users UserSource, notifier notifier, now func() time.Time) (*Service, error) {
	s := &Service{
		log:           zerolog.Nop(),
		repo:          repo,
		users:         users,
		notifier:      notifier,
		registrations: newCeremonyStore(now),
		logins:        newCeremonyStore(now),
		now:           now,
	}
	if cfg.Origin == "" {
		return s, nil
	}

	origin, err := url.Parse(cfg.Origin)
	if err != nil {
		return nil, fmt.Errorf("parse the passkey origin: %w", err)
	}

	s.wa, err = webauthn.New(&webauthn.Config{
		RPID:                  origin.Hostname(),
		RPDisplayName:         cfg.RPName,
		RPOrigins:             []string{cfg.Origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("set up passkeys: %w", err)
	}

	return s, nil
}

func (s *Service) Enabled() bool { return s.wa != nil }

func (s *Service) onlyUser(ctx context.Context) (*domain.User, error) {
	user, err := s.users.FindFirst(ctx)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil, ErrNoUser
	}

	return user, nil
}

func (s *Service) loadCredentials(ctx context.Context, userID int) ([]domain.Passkey, []webauthn.Credential, error) {
	passkeys, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("list passkeys: %w", err)
	}

	credentials := make([]webauthn.Credential, 0, len(passkeys))
	for _, passkey := range passkeys {
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(passkey.CredentialJSON), &credential); err != nil {
			s.log.Warn().Err(err).Int("id", passkey.ID).Msg("skipping a passkey with unreadable credential data")
			continue
		}
		credentials = append(credentials, credential)
	}

	return passkeys, credentials, nil
}

func newCeremonyID() (string, error) {
	random := make([]byte, ceremonyBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate ceremony id: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(random), nil
}

func (s *Service) BeginRegistration(ctx context.Context, name string) (string, *protocol.CredentialCreation, error) {
	if !s.Enabled() {
		return "", nil, ErrDisabled
	}

	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return "", nil, ErrInvalidName
	}

	user, err := s.onlyUser(ctx)
	if err != nil {
		return "", nil, err
	}

	passkeys, credentials, err := s.loadCredentials(ctx, user.ID)
	if err != nil {
		return "", nil, err
	}
	if len(passkeys) >= MaxPasskeys {
		return "", nil, ErrLimit
	}

	creation, session, err := s.wa.BeginRegistration(
		webauthnUser{user: *user, credentials: credentials},
		webauthn.WithExclusions(webauthn.Credentials(credentials).CredentialDescriptors()),
	)
	if err != nil {
		return "", nil, fmt.Errorf("begin registration: %w", err)
	}

	id, err := newCeremonyID()
	if err != nil {
		return "", nil, err
	}
	s.registrations.put(id, *session, name)

	return id, creation, nil
}

func (s *Service) FinishRegistration(ctx context.Context, ceremonyID string, r *http.Request) (*domain.Passkey, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}

	started, ok := s.registrations.take(ceremonyID)
	if !ok {
		return nil, ErrCeremonyNotFound
	}

	user, err := s.onlyUser(ctx)
	if err != nil {
		return nil, err
	}

	passkeys, credentials, err := s.loadCredentials(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if len(passkeys) >= MaxPasskeys {
		return nil, ErrLimit
	}

	credential, err := s.wa.FinishRegistration(webauthnUser{user: *user, credentials: credentials}, started.session, r)
	if err != nil {
		s.log.Warn().Err(err).Msg("passkey registration rejected")
		return nil, ErrRejected
	}

	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("encode credential: %w", err)
	}

	passkey := domain.Passkey{UserID: user.ID, CredentialID: credential.ID, Name: started.name, CredentialJSON: string(encoded)}
	id, err := s.repo.Store(ctx, passkey)
	if err != nil {
		return nil, fmt.Errorf("store passkey: %w", err)
	}
	passkey.ID = id
	passkey.CreatedAt = s.now()

	s.notifier.Send(domain.NotificationEventPasskeyAdded, domain.NotificationPayload{
		Subject:   "Passkey added",
		Message:   fmt.Sprintf("A passkey named %s was added for the web UI", started.name),
		Event:     domain.NotificationEventPasskeyAdded,
		Timestamp: s.now(),
	})

	return &passkey, nil
}

func (s *Service) BeginLogin() (string, *protocol.CredentialAssertion, error) {
	if !s.Enabled() {
		return "", nil, ErrDisabled
	}

	assertion, session, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, fmt.Errorf("begin login: %w", err)
	}

	id, err := newCeremonyID()
	if err != nil {
		return "", nil, err
	}
	s.logins.put(id, *session, "")

	return id, assertion, nil
}

func (s *Service) FinishLogin(ctx context.Context, ceremonyID string, r *http.Request) (*domain.User, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}

	started, ok := s.logins.take(ceremonyID)
	if !ok {
		return nil, ErrCeremonyNotFound
	}

	var owner *domain.User
	var known []domain.Passkey
	lookup := func(rawID, handle []byte) (webauthn.User, error) {
		passkey, err := s.repo.FindByCredentialID(ctx, rawID)
		if err != nil {
			return nil, storageFailure(err)
		}
		if passkey == nil {
			return nil, errors.New("unknown credential")
		}

		user, err := s.onlyUser(ctx)
		if err != nil {
			if errors.Is(err, ErrNoUser) {
				return nil, err
			}
			return nil, storageFailure(err)
		}
		if user.ID != passkey.UserID || !bytes.Equal(handle, userHandle(*user)) {
			return nil, errors.New("credential does not belong to the user")
		}

		passkeys, credentials, err := s.loadCredentials(ctx, user.ID)
		if err != nil {
			return nil, storageFailure(err)
		}
		owner, known = user, passkeys

		return webauthnUser{user: *user, credentials: credentials}, nil
	}

	_, credential, err := s.wa.FinishPasskeyLogin(lookup, started.session, r)
	if errors.Is(err, errStorage) {
		return nil, fmt.Errorf("look up passkey: %w", err)
	}
	if err != nil {
		s.log.Warn().Err(err).Msg("passkey login rejected")
		return nil, ErrRejected
	}
	if credential.Authenticator.CloneWarning {
		s.log.Warn().Msg("passkey login refused: the sign count went backwards")
		return nil, ErrCloned
	}

	if err := s.recordUse(ctx, known, credential); err != nil {
		return nil, err
	}

	return owner, nil
}

func (s *Service) recordUse(ctx context.Context, known []domain.Passkey, credential *webauthn.Credential) error {
	for _, passkey := range known {
		if !bytes.Equal(passkey.CredentialID, credential.ID) {
			continue
		}

		encoded, err := json.Marshal(credential)
		if err != nil {
			return fmt.Errorf("encode credential: %w", err)
		}
		if err := s.repo.UpdateCredential(ctx, passkey.ID, string(encoded), s.now()); err != nil {
			return fmt.Errorf("record passkey use: %w", err)
		}
	}

	return nil
}

func (s *Service) List(ctx context.Context) ([]domain.Passkey, error) {
	user, err := s.onlyUser(ctx)
	if err != nil {
		return nil, err
	}

	return s.repo.List(ctx, user.ID)
}

func (s *Service) Delete(ctx context.Context, id int) (bool, error) {
	user, err := s.onlyUser(ctx)
	if err != nil {
		return false, err
	}

	return s.repo.Delete(ctx, user.ID, id)
}

func (s *Service) DeleteAll(ctx context.Context) (int, error) {
	user, err := s.onlyUser(ctx)
	if err != nil {
		return 0, err
	}

	return s.repo.DeleteAll(ctx, user.ID)
}

func storageFailure(err error) error {
	return fmt.Errorf("%w: %w", errStorage, err)
}
