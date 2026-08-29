package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"sync"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/rs/zerolog"
)

type Service interface {
	Get(ctx context.Context, key string) (*domain.APIKey, error)
	List(ctx context.Context) ([]domain.APIKey, error)
	Store(ctx context.Context, key *domain.APIKey) error
	Update(ctx context.Context, key *domain.APIKey) error
	Delete(ctx context.Context, key string) error
	ValidateAPIKey(ctx context.Context, token string) bool
}

type service struct {
	log  zerolog.Logger
	repo domain.APIRepo

	// Per-process cache, loaded lazily, maintained by Store/Delete.
	mu   sync.RWMutex
	keys map[string]domain.APIKey
}

func NewService(log logger.Logger, repo domain.APIRepo) Service {
	return &service{
		log:  log.With().Str("module", "api").Logger(),
		repo: repo,
	}
}

func (s *service) Get(ctx context.Context, key string) (*domain.APIKey, error) {
	return s.repo.Get(ctx, key)
}

func (s *service) List(ctx context.Context) ([]domain.APIKey, error) {
	keys, err := s.loadKeys(ctx)
	if err != nil {
		return nil, err
	}

	list := make([]domain.APIKey, 0, len(keys))
	for _, k := range keys {
		list = append(list, k)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].CreatedAt == nil || list[j].CreatedAt == nil {
			return list[i].Key < list[j].Key
		}
		if list[i].CreatedAt.Equal(*list[j].CreatedAt) {
			return list[i].Key < list[j].Key
		}
		return list[i].CreatedAt.Before(*list[j].CreatedAt)
	})

	return list, nil
}

func (s *service) Store(ctx context.Context, key *domain.APIKey) error {
	key.Key = GenerateSecureToken(16)

	if err := s.repo.Store(ctx, key); err != nil {
		return err
	}

	s.mu.Lock()
	if s.keys != nil {
		s.keys[key.Key] = *key
	}
	s.mu.Unlock()

	return nil
}

func (s *service) Update(ctx context.Context, key *domain.APIKey) error {
	return nil
}

func (s *service) Delete(ctx context.Context, key string) error {
	if err := s.repo.Delete(ctx, key); err != nil {
		return err
	}

	s.mu.Lock()
	if s.keys != nil {
		delete(s.keys, key)
	}
	s.mu.Unlock()

	return nil
}

func (s *service) ValidateAPIKey(ctx context.Context, key string) bool {
	if key == "" {
		return false
	}

	keys, err := s.loadKeys(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to load api keys")
		return false
	}

	k, ok := keys[key]
	if !ok {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(k.Key), []byte(key)) == 1
}

func (s *service) loadKeys(ctx context.Context) (map[string]domain.APIKey, error) {
	s.mu.RLock()
	keys := s.keys
	s.mu.RUnlock()
	if keys != nil {
		return keys, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keys != nil {
		return s.keys, nil
	}

	list, err := s.repo.GetKeys(ctx)
	if err != nil {
		return nil, err
	}

	keys = make(map[string]domain.APIKey, len(list))
	for _, k := range list {
		keys[k.Key] = k
	}
	s.keys = keys

	return keys, nil
}

func GenerateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
