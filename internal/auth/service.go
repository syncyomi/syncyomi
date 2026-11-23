package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/user"
	"github.com/SyncYomi/SyncYomi/pkg/argon2id"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/sync/semaphore"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPasswordTooShort   = errors.New("password is too short")
	ErrPasswordTooLong    = errors.New("password is too long")
	ErrNoUser             = errors.New("no user exists")
	ErrBusy               = errors.New("too many password checks in progress")
)

const (
	MinPasswordLength = 8
	MaxPasswordBytes  = 1024
	hashSlots         = 4
	hashSlotWait      = 10 * time.Second
	dummyPassword     = "syncyomi dummy password for unknown usernames"
	stampBytes        = 8
)

type Service interface {
	GetUserCount(ctx context.Context) (int, error)
	Login(ctx context.Context, username, password string) (*domain.User, error)
	CreateUser(ctx context.Context, username, password string) error
	ChangePassword(ctx context.Context, current, next string) error
	ResetPassword(ctx context.Context, next string) error
	CurrentPasswordStamp(ctx context.Context) (string, error)
	NotifyLoginBlocked(client string)
}

type notifier interface {
	Send(event domain.NotificationEvent, payload domain.NotificationPayload)
}

type service struct {
	log      zerolog.Logger
	userSvc  user.Service
	notifier notifier
	slots    *semaphore.Weighted
	slotWait time.Duration

	params       *argon2id.Params
	compareHash  func(password, hash string) (bool, error)
	legacyParams *argon2id.Params
	dummies      []dummyHash
}

type dummyHash struct {
	params *argon2id.Params
	hash   string
}

func NewService(log logger.Logger, userSvc user.Service, notifier notifier) Service {
	return newService(log, userSvc, notifier, hashSlots, hashSlotWait)
}

func newService(log logger.Logger, userSvc user.Service, notifier notifier, slots int, slotWait time.Duration) *service {
	s := &service{
		log:      log.With().Str("module", "auth").Logger(),
		userSvc:  userSvc,
		notifier: notifier,
		slots:    semaphore.NewWeighted(int64(slots)),
		slotWait: slotWait,

		params:       argon2id.DefaultParams,
		legacyParams: argon2id.LegacyParams,
		compareHash:  argon2id.ComparePasswordAndHash,
	}
	s.dummies = s.newDummyHashes()

	return s
}

func (s *service) newDummyHashes() []dummyHash {
	profiles := []*argon2id.Params{s.params}
	if !s.legacyParams.SameCost(s.params) {
		profiles = append(profiles, s.legacyParams)
	}

	dummies := make([]dummyHash, 0, len(profiles))
	for _, params := range profiles {
		hashed, err := argon2id.CreateHash(dummyPassword, params)
		if err != nil {
			panic(errors.Wrap(err, "prepare the hash used for unknown usernames"))
		}
		dummies = append(dummies, dummyHash{params: params, hash: hashed})
	}

	return dummies
}

func (s *service) dummyHashFor(ctx context.Context) string {
	if u, err := s.userSvc.FindFirst(ctx); err == nil && u != nil {
		if stored, _, _, err := argon2id.DecodeHash(u.Password); err == nil {
			for _, dummy := range s.dummies {
				if dummy.params.SameCost(stored) {
					return dummy.hash
				}
			}
		}
	}

	return s.dummies[0].hash
}

func (s *service) ChangePassword(ctx context.Context, current, next string) error {
	if err := validateNewPassword(next); err != nil {
		return err
	}

	u, err := s.onlyUser(ctx)
	if err != nil {
		return err
	}

	match, err := s.passwordMatches(ctx, current, u.Password)
	if err != nil {
		return err
	}
	if !match {
		return ErrInvalidCredentials
	}

	if err := s.storePassword(ctx, u.ID, next); err != nil {
		return err
	}

	s.notifier.Send(domain.NotificationEventPasswordChanged, domain.NotificationPayload{
		Subject:   "Password changed",
		Message:   "The web UI password was changed and all other sessions were signed out",
		Event:     domain.NotificationEventPasswordChanged,
		Timestamp: time.Now(),
	})

	return nil
}

func (s *service) ResetPassword(ctx context.Context, next string) error {
	if err := validateNewPassword(next); err != nil {
		return err
	}

	u, err := s.onlyUser(ctx)
	if err != nil {
		return err
	}

	return s.storePassword(ctx, u.ID, next)
}

func (s *service) CurrentPasswordStamp(ctx context.Context) (string, error) {
	u, err := s.onlyUser(ctx)
	if err != nil {
		return "", err
	}

	return u.PasswordStamp(), nil
}

func (s *service) NotifyLoginBlocked(client string) {
	s.log.Warn().Str("client", client).Msg("too many failed logins; client blocked")
	s.notifier.Send(domain.NotificationEventLoginBlocked, domain.NotificationPayload{
		Subject:   "Login blocked",
		Message:   fmt.Sprintf("Too many failed logins from %s; further attempts from it are blocked for a while", client),
		Event:     domain.NotificationEventLoginBlocked,
		Timestamp: time.Now(),
	})
}

func (s *service) onlyUser(ctx context.Context) (*domain.User, error) {
	u, err := s.userSvc.FindFirst(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "find user")
	}
	if u == nil {
		return nil, ErrNoUser
	}

	return u, nil
}

func (s *service) storePassword(ctx context.Context, userID int, password string) error {
	hashed, err := s.hashPassword(ctx, password)
	if err != nil {
		return err
	}

	stamp, err := newStamp()
	if err != nil {
		return err
	}

	if err := s.userSvc.UpdatePassword(ctx, userID, hashed, stamp); err != nil {
		return errors.Wrap(err, "update password")
	}

	return nil
}

func newStamp() (string, error) {
	random := make([]byte, stampBytes)
	if _, err := rand.Read(random); err != nil {
		return "", errors.Wrap(err, "generate session stamp")
	}

	return hex.EncodeToString(random), nil
}

func (s *service) upgradeHash(ctx context.Context, u *domain.User, password string) {
	stored, _, _, err := argon2id.DecodeHash(u.Password)
	if err != nil || stored.SameCost(s.params) {
		return
	}

	upgraded, err := s.hashPassword(ctx, password)
	if err != nil {
		s.log.Warn().Err(err).Msg("could not upgrade the stored password hash")
		return
	}

	if err := s.userSvc.RehashPassword(ctx, u.ID, u.Password, upgraded); err != nil {
		s.log.Warn().Err(err).Msg("could not store the upgraded password hash")
		return
	}

	u.Stamp = u.PasswordStamp()
	u.Password = upgraded
}

func validateNewPassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}

	return nil
}

func (s *service) acquireSlot(ctx context.Context) (release func(), err error) {
	waitCtx, cancel := context.WithTimeout(ctx, s.slotWait)
	defer cancel()

	if err := s.slots.Acquire(waitCtx, 1); err != nil {
		if ctx.Err() == nil {
			return nil, ErrBusy
		}
		return nil, ctx.Err()
	}

	return func() { s.slots.Release(1) }, nil
}

func (s *service) hashPassword(ctx context.Context, password string) (string, error) {
	release, err := s.acquireSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	hashed, err := argon2id.CreateHash(password, s.params)
	if err != nil {
		return "", errors.Wrap(err, "hash password")
	}

	return hashed, nil
}

func (s *service) passwordMatches(ctx context.Context, password, hash string) (bool, error) {
	if password == "" || len(password) > MaxPasswordBytes {
		return false, nil
	}

	release, err := s.acquireSlot(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	match, err := s.compareHash(password, hash)
	if err != nil {
		return false, errors.Wrap(err, "compare password")
	}

	return match, nil
}

func (s *service) GetUserCount(ctx context.Context) (int, error) {
	return s.userSvc.GetUserCount(ctx)
}

func (s *service) Login(ctx context.Context, username, password string) (*domain.User, error) {
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	u, err := s.userSvc.FindByUsername(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err, "find user")
	}

	if u == nil {
		return nil, s.rejectUnknownUser(ctx, password)
	}

	match, err := s.passwordMatches(ctx, password, u.Password)
	if err != nil {
		return nil, err
	}

	if !match {
		return nil, ErrInvalidCredentials
	}

	s.upgradeHash(ctx, u, password)

	return u, nil
}

func (s *service) rejectUnknownUser(ctx context.Context, password string) error {
	if _, err := s.passwordMatches(ctx, password, s.dummyHashFor(ctx)); err != nil {
		return err
	}

	return ErrInvalidCredentials
}

func (s *service) CreateUser(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return errors.New("empty credentials supplied")
	}

	if err := validateNewPassword(password); err != nil {
		return err
	}

	userCount, err := s.userSvc.GetUserCount(ctx)
	if err != nil {
		return err
	}

	if userCount > 0 {
		return errors.New("only 1 user account is supported at the moment")
	}

	hashed, err := s.hashPassword(ctx, password)
	if err != nil {
		return err
	}

	stamp, err := newStamp()
	if err != nil {
		return err
	}

	newUser := domain.User{
		Username: username,
		Password: hashed,
		Stamp:    stamp,
	}
	if err := s.userSvc.CreateUser(context.Background(), newUser); err != nil {
		s.log.Error().Err(err).Msgf("could not create user: %v", username)
		return errors.New("failed to create new user")
	}

	return nil
}

func PolicyMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrPasswordTooShort):
		return fmt.Sprintf("Password must be at least %d characters", MinPasswordLength), true
	case errors.Is(err, ErrPasswordTooLong):
		return fmt.Sprintf("Password must be at most %d bytes", MaxPasswordBytes), true
	}

	return "", false
}
