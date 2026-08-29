package api

import (
	"context"
	"errors"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
)

type fakeAPIRepo struct {
	keys        map[string]domain.APIKey
	getKeysErr  error
	getKeyCalls int
}

func (f *fakeAPIRepo) Store(ctx context.Context, key *domain.APIKey) error {
	f.keys[key.Key] = *key
	return nil
}

func (f *fakeAPIRepo) Delete(ctx context.Context, key string) error {
	delete(f.keys, key)
	return nil
}

func (f *fakeAPIRepo) GetKeys(ctx context.Context) ([]domain.APIKey, error) {
	f.getKeyCalls++
	if f.getKeysErr != nil {
		return nil, f.getKeysErr
	}
	out := make([]domain.APIKey, 0, len(f.keys))
	for _, k := range f.keys {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeAPIRepo) Get(ctx context.Context, key string) (*domain.APIKey, error) {
	k, ok := f.keys[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &k, nil
}

func newTestService(repo domain.APIRepo) *service {
	return NewService(logger.Mock(), repo).(*service)
}

func TestValidateAPIKey(t *testing.T) {
	repo := &fakeAPIRepo{keys: map[string]domain.APIKey{
		"abc": {Name: "phone", Key: "abc"},
	}}
	svc := newTestService(repo)
	ctx := context.Background()

	if !svc.ValidateAPIKey(ctx, "abc") {
		t.Error("known key rejected")
	}
	if svc.ValidateAPIKey(ctx, "nope") {
		t.Error("unknown key accepted")
	}
	if svc.ValidateAPIKey(ctx, "") {
		t.Error("empty key accepted")
	}

	// many validations, one repo load
	for i := 0; i < 50; i++ {
		svc.ValidateAPIKey(ctx, "abc")
	}
	if repo.getKeyCalls != 1 {
		t.Errorf("GetKeys called %d times, want 1", repo.getKeyCalls)
	}
}

func TestValidateAPIKey_RepoError(t *testing.T) {
	repo := &fakeAPIRepo{keys: map[string]domain.APIKey{}, getKeysErr: errors.New("db down")}
	svc := newTestService(repo)

	if svc.ValidateAPIKey(context.Background(), "abc") {
		t.Error("key accepted while repo errors")
	}
}

func TestCacheFollowsStoreAndDelete(t *testing.T) {
	repo := &fakeAPIRepo{keys: map[string]domain.APIKey{}}
	svc := newTestService(repo)
	ctx := context.Background()

	// warm the cache while empty
	if svc.ValidateAPIKey(ctx, "anything") {
		t.Fatal("empty repo accepted a key")
	}

	key := &domain.APIKey{Name: "tablet"}
	if err := svc.Store(ctx, key); err != nil {
		t.Fatal(err)
	}
	if key.Key == "" {
		t.Fatal("Store did not generate a key")
	}
	if !svc.ValidateAPIKey(ctx, key.Key) {
		t.Error("freshly stored key rejected")
	}

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Key != key.Key {
		t.Errorf("List = %v, %v; want the stored key", list, err)
	}

	if err := svc.Delete(ctx, key.Key); err != nil {
		t.Fatal(err)
	}
	if svc.ValidateAPIKey(ctx, key.Key) {
		t.Error("deleted key still accepted")
	}

	if repo.getKeyCalls != 1 {
		t.Errorf("GetKeys called %d times, want 1 (cache should be maintained, not reloaded)", repo.getKeyCalls)
	}
}
