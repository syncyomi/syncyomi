package notification

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/pkg/errors"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"time"
)

type notifiarrMessage struct {
	Event string               `json:"event"`
	Data  notifiarrMessageData `json:"data"`
}

type notifiarrMessageData struct {
	Subject   string                   `json:"subject"`
	Message   string                   `json:"message"`
	Event     domain.NotificationEvent `json:"event"`
	Timestamp time.Time                `json:"timestamp"`
}

type notifiarrSender struct {
	log      zerolog.Logger
	Settings domain.Notification
	baseUrl  string
}

func NewNotifiarrSender(log zerolog.Logger, settings domain.Notification) domain.NotificationSender {
	return &notifiarrSender{
		log:      log.With().Str("sender", "notifiarr").Logger(),
		Settings: settings,
		baseUrl:  "https://notifiarr.com/api/v1/notification/tachi-sync-server",
	}
}

func (s *notifiarrSender) Send(event domain.NotificationEvent, payload domain.NotificationPayload) error {
	m := notifiarrMessage{
		Event: string(event),
		Data:  s.buildMessage(payload),
	}

	jsonData, err := json.Marshal(m)
	if err != nil {
		s.log.Error().Err(err).Msgf("notifiarr client could not marshal data: %v", m)
		return errors.Wrap(err, "could not marshal data: %+v", m)
	}

	req, err := http.NewRequest(http.MethodPost, s.baseUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		s.log.Error().Err(err).Msgf("notifiarr client request error: %v", event)
		return errors.Wrap(err, "could not create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tachisyncserver")
	req.Header.Set("X-API-Key", s.Settings.APIKey)

	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := http.Client{Transport: t, Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		s.log.Error().Err(err).Msgf("notifiarr client request error: %v", event)
		return errors.Wrap(err, "could not make request: %+v", req)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.log.Error().Err(err).Msgf("notifiarr client request error: %v", event)
		return errors.Wrap(err, "could not read data")
	}

	defer res.Body.Close()

	s.log.Trace().Msgf("notifiarr status: %v response: %v", res.StatusCode, string(body))

	if res.StatusCode != http.StatusOK {
		s.log.Error().Err(err).Msgf("notifiarr client request error: %v", string(body))
		return errors.New("bad status: %v body: %v", res.StatusCode, string(body))
	}

	s.log.Debug().Msg("notification successfully sent to notifiarr")

	return nil
}

func (s *notifiarrSender) CanSend(event domain.NotificationEvent) bool {
	if s.isEnabled() && s.isEnabledEvent(event) {
		return true
	}
	return false
}

func (s *notifiarrSender) isEnabled() bool {
	if s.Settings.Enabled {
		if s.Settings.APIKey == "" {
			s.log.Warn().Msg("notifiarr missing api key")
			return false
		}

		return true
	}
	return false
}

func (s *notifiarrSender) isEnabledEvent(event domain.NotificationEvent) bool {
	for _, e := range s.Settings.Events {
		if e == string(event) {
			return true
		}
	}

	return false
}

func (s *notifiarrSender) buildMessage(payload domain.NotificationPayload) notifiarrMessageData {
	m := notifiarrMessageData{
		Event:     payload.Event,
		Timestamp: payload.Timestamp,
	}

	if payload.Subject != "" && payload.Message != "" {
		m.Subject = payload.Subject
		m.Message = payload.Message
	}

	return m
}
