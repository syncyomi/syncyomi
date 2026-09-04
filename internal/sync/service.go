package sync

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/notification"
	"github.com/rs/zerolog"
)

// ErrInvalidSyncEvent is returned by ReportSyncEvent when the event string is not a valid sync event type.
var ErrInvalidSyncEvent = errors.New("invalid sync event")

type Service interface {
	// v2 protocol
	Merge(ctx context.Context, req MergeRequest) (*MergeResponse, error)
	Snapshot(ctx context.Context, apiKey string, cursor int64) (*Snapshot, error)
	// v1 protocol, served from the same item store
	GetContent(ctx context.Context, apiKey string) (*Snapshot, error)
	PutContent(ctx context.Context, apiKey string, dev domain.DeviceInfo, ifMatch string, data []byte) (string, error)

	// ReportSyncEvent persists a device-reported sync event and sends it to the notification service.
	ReportSyncEvent(ctx context.Context, apiKey string, event string, dev domain.DeviceInfo, detailMessage string) error
	// RecordContentAccess updates device/status bookkeeping after a successful GET or PUT of the
	// content, tagging the key and device with the protocol used. Errors are logged, not returned.
	RecordContentAccess(ctx context.Context, apiKey string, dev domain.DeviceInfo, write bool, protocol string)

	ListHistory(ctx context.Context, apiKey string) ([]domain.SyncHistoryEntry, error)
	RestoreHistory(ctx context.Context, apiKey string, id int) (*string, error)
	// GetHistoryData returns the stored payload of a history entry. domain.ErrNotFound if id is unknown.
	GetHistoryData(ctx context.Context, apiKey string, id int) ([]byte, error)
	ListDevices(ctx context.Context, apiKey string) ([]domain.SyncDevice, error)
	// DeleteDevice removes a device row. domain.ErrNotFound if id is unknown for the key.
	DeleteDevice(ctx context.Context, apiKey string, id int) error
	GetStatus(ctx context.Context, apiKey string) (*domain.SyncStatus, error)
}

const (
	importDelay   = 20 * time.Second
	importTimeout = 10 * time.Minute
)

func NewService(log logger.Logger, repo domain.SyncRepo, store domain.SyncStore, notificationSvc notification.Service, apiRepo domain.APIRepo) Service {
	s := &service{
		log:                 log.With().Str("module", "sync").Logger(),
		repo:                repo,
		store:               store,
		notificationService: notificationSvc,
		apiRepo:             apiRepo,
		locks:               &keyLocks{},
	}
	s.scheduleImport = newImporter(importDelay, s.runImport).schedule
	return s
}

type service struct {
	log                 zerolog.Logger
	repo                domain.SyncRepo
	store               domain.SyncStore
	notificationService notification.Service
	apiRepo             domain.APIRepo
	locks               *keyLocks
	scheduleImport      func(apiKey string)
}

type importer struct {
	mu     gosync.Mutex
	timers map[string]*time.Timer
	delay  time.Duration
	run    func(apiKey string)
}

func newImporter(delay time.Duration, run func(apiKey string)) *importer {
	return &importer{timers: map[string]*time.Timer{}, delay: delay, run: run}
}

func (i *importer) schedule(apiKey string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if t, ok := i.timers[apiKey]; ok {
		t.Reset(i.delay)
		return
	}
	i.timers[apiKey] = time.AfterFunc(i.delay, func() {
		i.mu.Lock()
		delete(i.timers, apiKey)
		i.mu.Unlock()
		i.run(apiKey)
	})
}

func (s *service) runImport(apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
	defer cancel()
	if _, err := s.ImportPending(ctx, apiKey); err != nil {
		s.log.Error().Err(err).Msg("importing the v1 upload into the item store failed; retrying on the next sync")
	}
}

func (s *service) ImportPending(ctx context.Context, apiKey string) (bool, error) {
	unlock, err := s.locks.lock(ctx, apiKey)
	if err != nil {
		return false, err
	}
	defer unlock()

	var imported bool
	err = s.store.Tx(ctx, apiKey, func(tx domain.SyncStoreTx) error {
		var err error
		imported, err = s.importPending(ctx, tx)
		return err
	})
	return imported, err
}

func (s *service) ListHistory(ctx context.Context, apiKey string) ([]domain.SyncHistoryEntry, error) {
	return s.repo.ListHistory(ctx, apiKey)
}

func (s *service) ListDevices(ctx context.Context, apiKey string) ([]domain.SyncDevice, error) {
	return s.repo.ListDevices(ctx, apiKey)
}

func (s *service) DeleteDevice(ctx context.Context, apiKey string, id int) error {
	return s.repo.DeleteDevice(ctx, apiKey, id)
}

func (s *service) GetHistoryData(ctx context.Context, apiKey string, id int) ([]byte, error) {
	return s.repo.GetHistoryData(ctx, apiKey, id)
}

func (s *service) GetStatus(ctx context.Context, apiKey string) (*domain.SyncStatus, error) {
	return s.repo.GetStatus(ctx, apiKey)
}

func (s *service) RecordContentAccess(ctx context.Context, apiKey string, dev domain.DeviceInfo, write bool, protocol string) {
	if err := s.repo.TouchDevice(ctx, apiKey, dev, "", "", "", protocol); err != nil {
		s.log.Warn().Err(err).Msg("failed to record device")
	}

	// reads record the protocol too: a v1-only fleet must be flagged even if it never PUTs
	if !write {
		if err := s.repo.UpsertStatus(ctx, apiKey, domain.SyncStatus{LastProtocol: protocol}); err != nil {
			s.log.Warn().Err(err).Msg("failed to record sync status")
		}
		return
	}

	now := time.Now().UTC()
	if err := s.repo.UpsertStatus(ctx, apiKey, domain.SyncStatus{LastSyncedAt: &now, LastDevice: dev.Name, LastProtocol: protocol}); err != nil {
		s.log.Warn().Err(err).Msg("failed to record sync status")
	}
}

func (s *service) ReportSyncEvent(ctx context.Context, apiKey string, event string, dev domain.DeviceInfo, detailMessage string) error {
	ev, err := parseSyncEvent(event)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	status := statusFromEvent(ev)
	// v1 requests carry no device identity, so the anonymous upload and the named event
	// rows for the same phone cannot be joined directly. When the key's last sync was v1,
	// tag the event's device row (only if it has no protocol yet) so the UI can show one
	// device instead of a named row plus the "legacy" aggregate.
	protocolHint := ""
	if st, err := s.repo.GetStatus(ctx, apiKey); err == nil && st != nil && st.LastProtocol == ProtocolV1 {
		protocolHint = ProtocolV1
	}
	if err := s.repo.TouchDevice(ctx, apiKey, dev, event, status, detailMessage, protocolHint); err != nil {
		s.log.Warn().Err(err).Msg("failed to record device")
	}
	if err := s.repo.UpsertStatus(ctx, apiKey, domain.SyncStatus{
		LastEventAt: &now,
		LastEvent:   event,
		LastStatus:  status,
		LastDevice:  dev.Name,
		LastMessage: detailMessage,
	}); err != nil {
		s.log.Warn().Err(err).Msg("failed to record sync status")
	}

	keyName := "Unknown"
	if key, err := s.apiRepo.Get(ctx, apiKey); err == nil && key != nil && key.Name != "" {
		keyName = key.Name
	}
	payload := s.buildSyncPayload(ev, keyName, dev.Name, detailMessage)
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

func statusFromEvent(ev domain.NotificationEvent) string {
	switch ev {
	case domain.NotificationEventSyncStarted:
		return "running"
	case domain.NotificationEventSyncSuccess:
		return "success"
	case domain.NotificationEventSyncFailed, domain.NotificationEventSyncError:
		return "error"
	case domain.NotificationEventSyncCancelled:
		return "cancelled"
	default:
		return ""
	}
}

func (s *service) buildSyncPayload(event domain.NotificationEvent, keyName string, deviceName string, detailMessage string) domain.NotificationPayload {
	devicePart := ""
	if deviceName != "" {
		devicePart = fmt.Sprintf(" from device **%s**", deviceName)
	}
	userPart := fmt.Sprintf(" (user **%s**)", keyName)
	ts := time.Now()

	switch event {
	case domain.NotificationEventSyncStarted:
		return domain.NotificationPayload{
			Subject:   "Sync started",
			Message:   fmt.Sprintf("Your library is syncing%s with **%s**. Give it a moment to finish.", devicePart, keyName),
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncSuccess:
		return domain.NotificationPayload{
			Subject:   "Sync completed",
			Message:   fmt.Sprintf("Your library finished syncing%s. All set with **%s**.", devicePart, keyName),
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncFailed:
		msg := fmt.Sprintf("Sync didn’t complete for **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Sync failed",
			Message:   msg,
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncError:
		msg := fmt.Sprintf("Something went wrong while syncing with **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Sync error",
			Message:   msg,
			Event:     event,
			Timestamp: ts,
		}
	case domain.NotificationEventSyncCancelled:
		msg := fmt.Sprintf("Sync was cancelled for **%s**%s.", keyName, devicePart)
		if detailMessage != "" {
			msg += " " + detailMessage
		}
		return domain.NotificationPayload{
			Subject:   "Sync cancelled",
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
