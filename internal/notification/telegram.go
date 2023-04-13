package notification

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
	"html"
	"io"
	"net/http"
	"time"
)

type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type telegramSender struct {
	log      zerolog.Logger
	Settings domain.Notification
}

func NewTelegramSender(log zerolog.Logger, settings domain.Notification) domain.NotificationSender {
	return &telegramSender{
		log:      log.With().Str("sender", "telegram").Logger(),
		Settings: settings,
	}
}

func (s *telegramSender) Send(event domain.NotificationEvent, payload domain.NotificationPayload) error {
	m := TelegramMessage{
		ChatID:    s.Settings.Channel,
		Text:      s.buildMessage(event, payload),
		ParseMode: "HTML",
		//ParseMode: "MarkdownV2",
	}

	jsonData, err := json.Marshal(m)
	if err != nil {
		s.log.Error().Err(err).Msgf("telegram client could not marshal data: %v", m)
		return errors.Wrap(err, "could not marshal data: %+v", m)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%v/sendMessage", s.Settings.Token)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		s.log.Error().Err(err).Msgf("telegram client request error: %v", event)
		return errors.Wrap(err, "could not create request")
	}

	req.Header.Set("Content-Type", "application/json")

	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := http.Client{Transport: t, Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		s.log.Error().Err(err).Msgf("telegram client request error: %v", event)
		return errors.Wrap(err, "could not make request: %+v", req)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.log.Error().Err(err).Msgf("telegram client request error: %v", event)
		return errors.Wrap(err, "could not read data")
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.log.Error().Msgf("telegram client could not close body: %v", err)
		}
	}(res.Body)

	s.log.Trace().Msgf("telegram status: %v response: %v", res.StatusCode, string(body))

	if res.StatusCode != http.StatusOK {
		s.log.Error().Err(err).Msgf("telegram client request error: %v", string(body))
		return errors.New("bad status: %v body: %v", res.StatusCode, string(body))
	}

	s.log.Debug().Msg("notification successfully sent to telegram")
	return nil
}

func (s *telegramSender) CanSend(event domain.NotificationEvent) bool {
	if s.isEnabled() && s.isEnabledEvent(event) {
		return true
	}
	return false
}

func (s *telegramSender) isEnabled() bool {
	if s.Settings.Enabled && s.Settings.Token != "" && s.Settings.Channel != "" {
		return true
	}
	return false
}

func (s *telegramSender) isEnabledEvent(event domain.NotificationEvent) bool {
	for _, e := range s.Settings.Events {
		if e == string(event) {
			return true
		}
	}

	return false
}

func (s *telegramSender) buildMessage(event domain.NotificationEvent, payload domain.NotificationPayload) string {
	msg := ""

	if payload.Subject != "" && payload.Message != "" {
		msg += fmt.Sprintf("%v\n<b>%v</b>", payload.Subject, html.EscapeString(payload.Message))
	}

	return msg
}
