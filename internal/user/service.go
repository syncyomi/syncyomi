package user

import (
	"context"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
)

type Service interface {
	GetUserCount(ctx context.Context) (int, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindFirst(ctx context.Context) (*domain.User, error)
	UpdatePassword(ctx context.Context, id int, hash, stamp string) error
	RehashPassword(ctx context.Context, id int, oldHash, newHash string) error
	CreateUser(ctx context.Context, user domain.User) error
}

type service struct {
	repo domain.UserRepo
}

func NewService(repo domain.UserRepo) Service {
	return &service{
		repo: repo,
	}
}

func (s *service) GetUserCount(ctx context.Context) (int, error) {
	return s.repo.GetUserCount(ctx)
}

func (s *service) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	user, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *service) FindFirst(ctx context.Context) (*domain.User, error) {
	return s.repo.FindFirst(ctx)
}

func (s *service) UpdatePassword(ctx context.Context, id int, hash, stamp string) error {
	return s.repo.UpdatePassword(ctx, id, hash, stamp)
}

func (s *service) RehashPassword(ctx context.Context, id int, oldHash, newHash string) error {
	return s.repo.RehashPassword(ctx, id, oldHash, newHash)
}

func (s *service) CreateUser(ctx context.Context, newUser domain.User) error {
	userCount, err := s.repo.GetUserCount(ctx)
	if err != nil {
		return err
	}

	if userCount > 0 {
		return errors.New("only 1 user account is supported at the moment")
	}

	return s.repo.Store(ctx, newUser)
}
