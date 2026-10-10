package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/rs/zerolog"
)

type fakeNotificationRepo struct {
	listed []domain.Notification
	stored []domain.Notification
}

func (f *fakeNotificationRepo) List(context.Context) ([]domain.Notification, error) {
	return f.listed, nil
}

func (f *fakeNotificationRepo) Find(context.Context, domain.NotificationQueryParams) ([]domain.Notification, int, error) {
	return f.listed, len(f.listed), nil
}

func (f *fakeNotificationRepo) FindByID(context.Context, int) (*domain.Notification, error) {
	return nil, nil
}

func (f *fakeNotificationRepo) Store(_ context.Context, n domain.Notification) (*domain.Notification, error) {
	f.stored = append(f.stored, n)
	return &n, nil
}

func (f *fakeNotificationRepo) Update(_ context.Context, n domain.Notification) (*domain.Notification, error) {
	return &n, nil
}

func (f *fakeNotificationRepo) Delete(context.Context, int) error {
	return nil
}

func newTestService(repo *fakeNotificationRepo) *service {
	return &service{
		log:     zerolog.Nop(),
		repo:    repo,
		senders: []domain.NotificationSender{},
	}
}

func TestService_StoreRejectsInvalidWebhookURL(t *testing.T) {
	repo := &fakeNotificationRepo{}
	s := newTestService(repo)

	_, err := s.Store(context.Background(), domain.Notification{
		Name:    "bad",
		Type:    domain.NotificationTypeWebhook,
		Webhook: "foo",
	})

	if err == nil {
		t.Fatal("Store: got nil error, want a rejected url")
	}
	if len(repo.stored) != 0 {
		t.Errorf("repo.Store calls: got %d, want 0", len(repo.stored))
	}
}

func TestService_TestRejectsInvalidWebhookURL(t *testing.T) {
	s := newTestService(&fakeNotificationRepo{})

	err := s.Test(context.Background(), domain.Notification{
		Type:    domain.NotificationTypeWebhook,
		Webhook: "foo",
	})

	if err == nil || !strings.Contains(err.Error(), "webhook url") {
		t.Fatalf("Test: got %v, want a rejected webhook url", err)
	}
}

func TestService_registerSendersIncludesWebhook(t *testing.T) {
	s := newTestService(&fakeNotificationRepo{listed: []domain.Notification{{
		Name:    "hook",
		Type:    domain.NotificationTypeWebhook,
		Enabled: true,
		Webhook: "https://example.com/hook",
	}}})

	s.registerSenders()

	if len(s.senders) != 1 {
		t.Errorf("senders: got %d, want 1", len(s.senders))
	}
}
