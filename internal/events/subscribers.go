package events

import (
	"github.com/asaskevich/EventBus"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/notification"
	"github.com/rs/zerolog"
)

type Subscriber struct {
	log             zerolog.Logger
	eventbus        EventBus.Bus
	notificationSvc notification.Service
}

func NewSubscribers(log logger.Logger, eventbus EventBus.Bus, notificationSvc notification.Service) Subscriber {
	s := Subscriber{
		log:             log.With().Str("module", "events").Logger(),
		eventbus:        eventbus,
		notificationSvc: notificationSvc,
	}

	s.Register()

	return s
}

func (s Subscriber) Register() {
	err := s.eventbus.Subscribe("events:notification", s.sendNotification)
	if err != nil {
		s.log.Error().Msgf("failed to subscribe to events:notification: %v", err)
		return
	}
}

func (s Subscriber) sendNotification(event *domain.NotificationEvent, payload *domain.NotificationPayload) {
	s.log.Trace().Msgf("events: '%v' '%+v'", *event, payload)

	s.notificationSvc.Send(*event, *payload)
}
