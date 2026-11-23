package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/user"
	"github.com/SyncYomi/SyncYomi/pkg/argon2id"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPasswordTooShort   = errors.New("password is too short")
	ErrPasswordTooLong    = errors.New("password is too long")
	ErrNoUser             = errors.New("no user exists")
)

const (
	MinPasswordLength = 8
	MaxPasswordBytes  = 1024
	hashSlots         = 4
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
	slots    chan struct{}
}

func NewService(log logger.Logger, userSvc user.Service, notifier notifier) Service {
	return newService(log, userSvc, notifier, hashSlots)
}

func newService(log logger.Logger, userSvc user.Service, notifier notifier, slots int) *service {
	return &service{
		log:      log.With().Str("module", "auth").Logger(),
		userSvc:  userSvc,
		notifier: notifier,
		slots:    make(chan struct{}, slots),
	}
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
		s.log.Warn().Msg("password change rejected: wrong current password")
		return ErrInvalidCredentials
	}

	if err := s.storePassword(ctx, u.ID, next); err != nil {
		return err
	}

	s.log.Info().Msg("password changed")
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

	if err := s.userSvc.UpdatePassword(ctx, userID, hashed); err != nil {
		return errors.Wrap(err, "update password")
	}

	return nil
}

func validateNewPassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}

	return nil
}

func (s *service) acquireSlot(ctx context.Context) (release func(), err error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *service) hashPassword(ctx context.Context, password string) (string, error) {
	release, err := s.acquireSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	hashed, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", errors.New("failed to hash password")
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

	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, errors.New("error checking credentials")
	}

	return match, nil
}

func (s *service) GetUserCount(ctx context.Context) (int, error) {
	return s.userSvc.GetUserCount(ctx)
}

func (s *service) Login(ctx context.Context, username, password string) (*domain.User, error) {
	if username == "" || password == "" {
		return nil, errors.New("empty credentials supplied")
	}

	// find user
	u, err := s.userSvc.FindByUsername(ctx, username)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not find user by username: %v", username)
		return nil, errors.Wrapf(err, "invalid login: %s", username)
	}

	if u == nil {
		return nil, errors.Errorf("invalid login: %s", username)
	}

	// compare password from request and the saved password
	match, err := s.passwordMatches(ctx, password, u.Password)
	if err != nil {
		return nil, err
	}

	if !match {
		s.log.Error().Msg("bad credentials")
		return nil, errors.Errorf("invalid login: %s", username)
	}

	return u, nil
}

func (s *service) CreateUser(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return errors.New("empty credentials supplied")
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

	newUser := domain.User{
		Username: username,
		Password: hashed,
	}
	if err := s.userSvc.CreateUser(context.Background(), newUser); err != nil {
		s.log.Error().Err(err).Msgf("could not create user: %v", username)
		return errors.New("failed to create new user")
	}

	return nil
}
