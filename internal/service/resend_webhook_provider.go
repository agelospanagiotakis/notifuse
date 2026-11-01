package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// resendAPIBaseURL is a variable rather than a constant so tests can point it at a stub.
var resendAPIBaseURL = "https://api.resend.com"

// ResendWebhookProvider registers Notifuse's callback with Resend's API.
//
// Without this, creating a Resend integration in the console fails with
// "webhook registration not implemented for provider: resend" — the integration still
// works, but the operator is left to hand-craft the URL, and a typo there is invisible
// until a bounce never arrives.
type ResendWebhookProvider struct {
	logger logger.Logger
	client *http.Client
}

func NewResendWebhookProvider(l logger.Logger) *ResendWebhookProvider {
	return &ResendWebhookProvider{logger: l, client: &http.Client{Timeout: 20 * time.Second}}
}

type resendWebhookRecord struct {
	ID            string   `json:"id"`
	Endpoint      string   `json:"endpoint"`
	Events        []string `json:"events"`
	Status        string   `json:"status"`
	CreatedAt     string   `json:"created_at"`
	SigningSecret string   `json:"signing_secret,omitempty"`
}

type resendWebhookList struct {
	Data []resendWebhookRecord `json:"data"`
}

// resendEventNames maps our event types onto Resend's webhook event names.
func resendEventNames(eventTypes []domain.EmailEventType) []string {
	names := make([]string, 0, len(eventTypes))
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, e := range eventTypes {
		switch e {
		case domain.EmailEventDelivered:
			add("email.delivered")
		case domain.EmailEventBounce:
			add("email.bounced")
		case domain.EmailEventComplaint:
			add("email.complained")
		}
	}
	return names
}

func (p *ResendWebhookProvider) apiKey(providerConfig *domain.EmailProvider) (string, error) {
	if providerConfig == nil || providerConfig.Resend == nil {
		return "", fmt.Errorf("resend configuration is missing or invalid")
	}
	// The Resend API key is stored as the SMTP password: Resend uses one key for both.
	key := providerConfig.Resend.Password
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("resend API key is missing")
	}
	return key, nil
}

func (p *ResendWebhookProvider) do(ctx context.Context, method, path, key string, body interface{}) ([]byte, int, error) {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, resendAPIBaseURL+path, payload)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("resend API request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func (p *ResendWebhookProvider) list(ctx context.Context, key string) ([]resendWebhookRecord, error) {
	data, code, err := p.do(ctx, http.MethodGet, "/webhooks", key, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("resend list webhooks failed: %d %s", code, strings.TrimSpace(string(data)))
	}
	var list resendWebhookList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse resend webhook list: %w", err)
	}
	return list.Data, nil
}

// RegisterWebhooks points Resend at our callback. It is idempotent: an endpoint that
// already matches is reported rather than duplicated.
func (p *ResendWebhookProvider) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	key, err := p.apiKey(providerConfig)
	if err != nil {
		return nil, err
	}

	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindResend, workspaceID, integrationID)
	events := resendEventNames(eventTypes)
	if len(events) == 0 {
		events = []string{"email.delivered", "email.bounced", "email.complained"}
	}

	existing, err := p.list(ctx, key)
	if err != nil {
		return nil, err
	}
	for _, wh := range existing {
		if strings.TrimSuffix(wh.Endpoint, "/") == strings.TrimSuffix(webhookURL, "/") {
			return &domain.WebhookRegistrationStatus{
				EmailProviderKind: domain.EmailProviderKindResend,
				IsRegistered:      true,
				Endpoints: []domain.WebhookEndpointStatus{{
					WebhookID: wh.ID,
					URL:       wh.Endpoint,
					Active:    true,
				}},
				ProviderDetails: map[string]interface{}{
					"endpoint": webhookURL,
					"events":   wh.Events,
				},
			}, nil
		}
	}

	data, code, err := p.do(ctx, http.MethodPost, "/webhooks", key, map[string]interface{}{
		"endpoint": webhookURL,
		"events":   events,
	})
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return nil, fmt.Errorf("resend create webhook failed: %d %s", code, strings.TrimSpace(string(data)))
	}

	var created resendWebhookRecord
	_ = json.Unmarshal(data, &created)

	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindResend,
		IsRegistered:      true,
		Endpoints: []domain.WebhookEndpointStatus{{
			WebhookID: created.ID,
			URL:       webhookURL,
			Active:    true,
		}},
		ProviderDetails: map[string]interface{}{
			"endpoint": webhookURL,
			"events":   events,
		},
	}

	// Resend returns the signing secret once, at creation. The integration needs it to
	// verify events, and we cannot write it back from here without the caller's
	// credentials, so surface it rather than silently losing it.
	if created.SigningSecret != "" {
		status.ProviderDetails["signing_secret"] = created.SigningSecret
		status.ProviderDetails["signing_secret_note"] = "paste this into the integration's Webhook Signing Secret field; Resend shows it only once"
	}

	p.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		WithField("webhook_id", created.ID).
		Info("Registered Resend webhook")

	return status, nil
}

// GetWebhookStatus reports whether our callback is registered, without changing anything.
func (p *ResendWebhookProvider) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	key, err := p.apiKey(providerConfig)
	if err != nil {
		return nil, err
	}
	existing, err := p.list(ctx, key)
	if err != nil {
		return nil, err
	}

	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindResend,
		ProviderDetails:   map[string]interface{}{"webhook_count": len(existing)},
	}
	for _, wh := range existing {
		// The endpoint carries the integration id, so matching on that is stable across a
		// hostname change and does not need the base URL this method is not given.
		if strings.Contains(wh.Endpoint, integrationID) {
			status.IsRegistered = true
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				WebhookID: wh.ID,
				URL:       wh.Endpoint,
				Active:    strings.EqualFold(wh.Status, "enabled") || wh.Status == "",
			})
		}
	}
	return status, nil
}

// UnregisterWebhooks removes our callback from Resend. Deleting an integration must not
// leave a webhook firing at an endpoint that no longer resolves.
func (p *ResendWebhookProvider) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	key, err := p.apiKey(providerConfig)
	if err != nil {
		return err
	}
	existing, err := p.list(ctx, key)
	if err != nil {
		return err
	}

	// Match on the integration id: the endpoint carries it, and that survives a host rename.
	marker := integrationID
	var failures []string
	for _, wh := range existing {
		if !strings.Contains(wh.Endpoint, marker) {
			continue
		}
		data, code, err := p.do(ctx, http.MethodDelete, "/webhooks/"+wh.ID, key, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if code != http.StatusOK && code != http.StatusNoContent && code != http.StatusNotFound {
			failures = append(failures, fmt.Sprintf("%s: %d %s", wh.ID, code, strings.TrimSpace(string(data))))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed to delete resend webhook(s): %s", strings.Join(failures, "; "))
	}
	return nil
}
