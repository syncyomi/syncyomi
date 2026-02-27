package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/notification"
	"github.com/rs/zerolog"
)

// ErrInvalidSyncEvent is returned by ReportSyncEvent when the event string is not a valid sync event type.
var ErrInvalidSyncEvent = errors.New("invalid sync event")

type Service interface {
	// Get etag of sync data.
	// For avoid memory usage, only the etag will be returnedj
	GetSyncDataETag(ctx context.Context, apiKey string) (*string, error)
	// Get sync data and etag
	GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error)
	// Create or replace sync data, returns the new etag.
	SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error)
	// Replace sync data only if the etag matches,
	// returns the new etag if updated, or nil if not.
	SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error)
	// ReportSyncEvent sends a device-reported sync event to the notification service.
	ReportSyncEvent(ctx context.Context, apiKey string, event string, deviceName string, detailMessage string) error
}

func NewService(log logger.Logger, repo domain.SyncRepo, notificationSvc notification.Service, apiRepo domain.APIRepo) Service {
	return &service{
		log:                 log.With().Str("module", "sync").Logger(),
		repo:                repo,
		notificationService: notificationSvc,
		apiRepo:             apiRepo,
	}
}

type service struct {
	log                 zerolog.Logger
	repo                domain.SyncRepo
	notificationService notification.Service
	apiRepo             domain.APIRepo
}

// Get etag of sync data.
// For avoid memory usage, only the etag will be returned.
func (s service) GetSyncDataETag(ctx context.Context, apiKey string) (*string, error) {
	return s.repo.GetSyncDataETag(ctx, apiKey)
}

// Get sync data and etag
func (s service) GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error) {
	return s.repo.GetSyncDataAndETag(ctx, apiKey)
}

// Create or replace sync data, returns the new etag.
func (s service) SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error) {
	return s.repo.SetSyncData(ctx, apiKey, data)
}

// Replace sync data only if the etag matches,
// returns the new etag if updated, or nil if not.
func (s service) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	return s.repo.SetSyncDataIfMatch(ctx, apiKey, etag, data)
}

func (s service) ReportSyncEvent(ctx context.Context, apiKey string, event string, deviceName string, detailMessage string) error {
	ev, err := parseSyncEvent(event)
	if err != nil {
		return err
	}
	keyName := "Unknown"
	if key, err := s.apiRepo.Get(ctx, apiKey); err == nil && key != nil && key.Name != "" {
		keyName = key.Name
	}
	payload := s.buildSyncPayload(ev, keyName, deviceName, detailMessage)
	s.notificationService.Send(ev, payload)
	return nil
}

func parseSyncEvent(event string) (domain.NotificationEvent, error) {
	switch event {
	case string(domain.NotificationEventSyncStarted):
		return domain.NotificationEventSyncStarted, nil
	case string(domain.NotificationEventSyncSuccess):
		return domain.NotificationEventSyncSuccess, nil
	case string(domain.NotificationEventSyncFailed):
		return domain.NotificationEventSyncFailed, nil
	case string(domain.NotificationEventSyncError):
		return domain.NotificationEventSyncError, nil
	case string(domain.NotificationEventSyncCancelled):
		return domain.NotificationEventSyncCancelled, nil
	default:
		return "", ErrInvalidSyncEvent
	}
}

func (s service) buildSyncPayload(event domain.NotificationEvent, keyName string, deviceName string, detailMessage string) domain.NotificationPayload {
	devicePart := ""
	if deviceName != "" {
		devicePart = fmt.Sprintf(" from device **%s**", deviceName)
	}
	userPart := fmt.Sprintf(" (user **%s**)", keyName)
	ts := time.Now()

	switch event {
	case domain.NotificationEventSyncStarted:
		return domain.NotificationPayload{
			Subject:   "Data Transmission Initiated",
			Message:   fmt.Sprintf("A data transmission between your Tachiyomi library and user **%s** has been initiated%s. Please wait for the process to complete.", keyName, devicePart),
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncSuccess:
		return domain.NotificationPayload{
			Subject:   "Data Send Successful",
			Message:   fmt.Sprintf("Your Tachiyomi library data has been successfully sent%s. User: **%s**.", devicePart, keyName),
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncFailed:
		msg := fmt.Sprintf("The synchronization with Tachiyomi failed for user **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " Error: " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Sync Operation Failed",
			Message:   msg,
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncError:
		msg := fmt.Sprintf("An error occurred during synchronization with Tachiyomi for user **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " Error: " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Error During Sync",
			Message:   msg,
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncCancelled:
		msg := fmt.Sprintf("Synchronization was cancelled for user **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Sync Cancelled",
			Message:   msg,
			Event:     event,
			Timestamp: ts,
		}
	default:
		return domain.NotificationPayload{
			Subject:   "Sync Event",
			Message:   fmt.Sprintf("Sync event %s%s%s.", event, devicePart, userPart),
			Event:     event,
			Timestamp: ts,
		}
	}
}

func (s service) notifySyncStarted(apiKeyName string) {
	s.notificationService.Send(domain.NotificationEventSyncStarted, s.buildSyncPayload(domain.NotificationEventSyncStarted, apiKeyName, "", ""))
}

func (s service) notifySyncSuccess(apiKeyName string) {
	s.notificationService.Send(domain.NotificationEventSyncSuccess, s.buildSyncPayload(domain.NotificationEventSyncSuccess, apiKeyName, "", ""))
}

func (s service) notifySyncFailed(apiKeyName string, errMsg string) {
	s.notificationService.Send(domain.NotificationEventSyncFailed, s.buildSyncPayload(domain.NotificationEventSyncFailed, apiKeyName, "", errMsg))
}

func (s service) notifySyncError(apiKeyName string, errMsg string) {
	s.notificationService.Send(domain.NotificationEventSyncError, s.buildSyncPayload(domain.NotificationEventSyncError, apiKeyName, "", errMsg))
}
