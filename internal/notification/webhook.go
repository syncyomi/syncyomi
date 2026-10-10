package notification

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
)

type webhookPayload struct {
	Event     string `json:"event"`
	Subject   string `json:"subject"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type webhookSender struct {
	log      zerolog.Logger
	client   *http.Client
	Settings domain.Notification
}

func NewWebhookSender(log zerolog.Logger, settings domain.Notification) domain.NotificationSender {
	return &webhookSender{
		log:      log.With().Str("sender", "webhook").Logger(),
		client:   &http.Client{Timeout: 30 * time.Second},
		Settings: settings,
	}
}

func (s *webhookSender) Send(event domain.NotificationEvent, payload domain.NotificationPayload) error {
	body, err := json.Marshal(webhookPayload{
		Event:     string(event),
		Subject:   payload.Subject,
		Message:   payload.Message,
		Timestamp: payload.Timestamp.UTC().Format(time.RFC3339),
	})
	if err != nil {
		s.log.Error().Err(err).Msgf("webhook client could not marshal payload for event: %v", event)
		return errors.Wrap(err, "could not marshal payload")
	}

	req, err := http.NewRequest(http.MethodPost, s.Settings.Webhook, bytes.NewReader(body))
	if err != nil {
		s.log.Error().Err(err).Msgf("webhook client request error: %v", event)
		return errors.Wrap(err, "could not create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SyncYomi")
	if s.Settings.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Settings.Token)
	}

	res, err := s.client.Do(req)
	if err != nil {
		s.log.Error().Err(err).Msgf("webhook client request error: %v", event)
		return errors.Wrap(err, "could not make request to %s", s.Settings.Webhook)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		s.log.Error().Msgf("webhook client bad status %d: %s", res.StatusCode, responseBody)
		return errors.New("bad status: %d body: %s", res.StatusCode, responseBody)
	}

	s.log.Debug().Msg("notification successfully sent to webhook")

	return nil
}

func (s *webhookSender) CanSend(event domain.NotificationEvent) bool {
	return s.Settings.Enabled && s.Settings.Webhook != "" && slices.Contains(s.Settings.Events, string(event))
}

func validateWebhookURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(err, "invalid webhook url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("webhook url must start with http:// or https://")
	}
	if parsed.Host == "" {
		return errors.New("webhook url must include a host")
	}

	return nil
}
