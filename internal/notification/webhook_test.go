package notification

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/rs/zerolog"
)

type recordedRequest struct {
	method        string
	contentType   string
	userAgent     string
	authorization string
	body          []byte
}

func newRecordingServer(t *testing.T, status int, responseBody string) (*httptest.Server, *recordedRequest) {
	t.Helper()

	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		*rec = recordedRequest{
			method:        r.Method,
			contentType:   r.Header.Get("Content-Type"),
			userAgent:     r.Header.Get("User-Agent"),
			authorization: r.Header.Get("Authorization"),
			body:          body,
		}
		w.WriteHeader(status)
		io.WriteString(w, responseBody)
	}))
	t.Cleanup(srv.Close)

	return srv, rec
}

func webhookSettings(url, token string) domain.Notification {
	return domain.Notification{
		Name:    "hook",
		Type:    domain.NotificationTypeWebhook,
		Enabled: true,
		Events:  []string{string(domain.NotificationEventSyncSuccess)},
		Webhook: url,
		Token:   token,
	}
}

func TestWebhookSender_SendPostsJSON(t *testing.T) {
	srv, rec := newRecordingServer(t, http.StatusOK, "")
	sender := NewWebhookSender(zerolog.Nop(), webhookSettings(srv.URL, ""))
	timestamp := time.Date(2026, 10, 10, 12, 30, 0, 0, time.FixedZone("plus2", 2*60*60))
	payload := domain.NotificationPayload{
		Subject:   "Sync Completed Successfully!",
		Message:   "Sync Completed BETWEEN **DEVICE** AND SERVER",
		Event:     domain.NotificationEventSyncSuccess,
		Timestamp: timestamp,
	}

	err := sender.Send(domain.NotificationEventSyncSuccess, payload)

	if err != nil {
		t.Fatalf("Send: unexpected error %v", err)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method: got %q, want POST", rec.method)
	}
	if rec.contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", rec.contentType)
	}
	if rec.userAgent != "SyncYomi" {
		t.Errorf("User-Agent: got %q, want SyncYomi", rec.userAgent)
	}
	var got webhookPayload
	if err := json.Unmarshal(rec.body, &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.body, err)
	}
	want := webhookPayload{
		Event:     "SYNC_SUCCESS",
		Subject:   payload.Subject,
		Message:   payload.Message,
		Timestamp: timestamp.UTC().Format(time.RFC3339),
	}
	if got != want {
		t.Errorf("body: got %+v, want %+v", got, want)
	}
}

func TestWebhookSender_SendBearerToken(t *testing.T) {
	srv, rec := newRecordingServer(t, http.StatusOK, "")
	sender := NewWebhookSender(zerolog.Nop(), webhookSettings(srv.URL, "s3cret"))

	err := sender.Send(domain.NotificationEventSyncSuccess, domain.NotificationPayload{})

	if err != nil {
		t.Fatalf("Send: unexpected error %v", err)
	}
	if rec.authorization != "Bearer s3cret" {
		t.Errorf("Authorization: got %q, want %q", rec.authorization, "Bearer s3cret")
	}
}

func TestWebhookSender_SendOmitsAuthorizationWithoutToken(t *testing.T) {
	srv, rec := newRecordingServer(t, http.StatusOK, "")
	sender := NewWebhookSender(zerolog.Nop(), webhookSettings(srv.URL, ""))

	err := sender.Send(domain.NotificationEventSyncSuccess, domain.NotificationPayload{})

	if err != nil {
		t.Fatalf("Send: unexpected error %v", err)
	}
	if rec.authorization != "" {
		t.Errorf("Authorization: got %q, want empty", rec.authorization)
	}
}

func TestWebhookSender_SendRejectsNon2xx(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusInternalServerError, "boom")
	sender := NewWebhookSender(zerolog.Nop(), webhookSettings(srv.URL, ""))

	err := sender.Send(domain.NotificationEventSyncSuccess, domain.NotificationPayload{})

	if err == nil {
		t.Fatal("Send: got nil error, want one mentioning the status")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error: got %q, want it to contain 500 and boom", err.Error())
	}
}

func TestWebhookSender_SendAcceptsNoContent(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusNoContent, "")
	sender := NewWebhookSender(zerolog.Nop(), webhookSettings(srv.URL, ""))

	err := sender.Send(domain.NotificationEventSyncSuccess, domain.NotificationPayload{})

	if err != nil {
		t.Errorf("Send: got %v, want nil", err)
	}
}

func TestWebhookSender_CanSend(t *testing.T) {
	tests := []struct {
		name     string
		settings domain.Notification
		event    domain.NotificationEvent
		want     bool
	}{
		{
			name:     "disabled notification",
			settings: domain.Notification{Enabled: false, Webhook: "https://example.com/hook", Events: []string{"SYNC_SUCCESS"}},
			event:    domain.NotificationEventSyncSuccess,
			want:     false,
		},
		{
			name:     "event not selected",
			settings: domain.Notification{Enabled: true, Webhook: "https://example.com/hook", Events: []string{"SYNC_SUCCESS"}},
			event:    domain.NotificationEventSyncFailed,
			want:     false,
		},
		{
			name:     "empty url",
			settings: domain.Notification{Enabled: true, Webhook: "", Events: []string{"SYNC_SUCCESS"}},
			event:    domain.NotificationEventSyncSuccess,
			want:     false,
		},
		{
			name:     "enabled with url and selected event",
			settings: domain.Notification{Enabled: true, Webhook: "https://example.com/hook", Events: []string{"SYNC_SUCCESS"}},
			event:    domain.NotificationEventSyncSuccess,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sender := NewWebhookSender(zerolog.Nop(), tc.settings)

			got := sender.CanSend(tc.event)

			if got != tc.want {
				t.Errorf("CanSend: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "https url", raw: "https://example.com/hook", wantErr: false},
		{name: "http url to a private address", raw: "http://192.168.1.5:8123/api/webhook/x", wantErr: false},
		{name: "ftp scheme", raw: "ftp://x/y", wantErr: true},
		{name: "missing scheme", raw: "example.com/hook", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
		{name: "scheme without host", raw: "https://", wantErr: true},
		{name: "javascript scheme", raw: "javascript:alert(1)", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.raw)

			if (err != nil) != tc.wantErr {
				t.Errorf("validateWebhookURL(%q): got err %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
		})
	}
}
