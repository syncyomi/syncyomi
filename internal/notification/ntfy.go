package notification

import (
	"net/http"
	"bytes"
	"crypto/tls"
	"time"
	"io"
	
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
)

type ntfySender struct {
	log      zerolog.Logger
	Settings domain.Notification
}

func NewNtfySender(log zerolog.Logger, settings domain.Notification) domain.NotificationSender {
	return &ntfySender{
		log:      log.With().Str("sender", "ntfy").Logger(),
		Settings: settings,
	}
}

func (s *ntfySender) Send(event domain.NotificationEvent, payload domain.NotificationPayload) error {
	req, err := http.NewRequest(http.MethodPost, s.Settings.Webhook, bytes.NewBufferString(payload.Message))
	if err != nil {
		s.log.Error().Err(err).Msgf("telegram client request error: %v", event)
		return errors.Wrap(err, "could not create request")
	}

	req.Header.Set("X-Title", payload.Subject)

	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := http.Client{Transport: t, Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		s.log.Error().Err(err).Msgf("ntfy client request error: %v", event)
		return errors.Wrap(err, "could not make request: %+v", req)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.log.Error().Err(err).Msgf("ntfy client request error: %v", event)
		return errors.Wrap(err, "could not read data")
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.log.Error().Msgf("ntfy client could not close body: %v", err)
		}
	}(res.Body)

	s.log.Trace().Msgf("ntfy status: %v response: %v", res.StatusCode, string(body))

	if res.StatusCode != http.StatusOK {
		s.log.Error().Err(err).Msgf("ntfy client request error: %v", string(body))
		return errors.New("bad status: %v body: %v", res.StatusCode, string(body))
	}

	s.log.Debug().Msg("notification successfully sent to ntfy")
	return nil
}

func (s *ntfySender) CanSend(event domain.NotificationEvent) bool {
	if s.isEnabled() && s.isEnabledEvent(event) {
		return true
	}
	return false
}

func (s *ntfySender) isEnabled() bool {
	if s.Settings.Enabled && s.Settings.Webhook != "" {
		return true
	}
	return false
}

func (s *ntfySender) isEnabledEvent(event domain.NotificationEvent) bool {
	for _, e := range s.Settings.Events {
		if e == string(event) {
			return true
		}
	}

	return false
}
