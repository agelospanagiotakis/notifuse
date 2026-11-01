package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resendProviderForTest(apiKey string) *domain.EmailProvider {
	return &domain.EmailProvider{
		Kind: domain.EmailProviderKindResend,
		Resend: &domain.ResendSettings{
			Host:          "smtp.resend.com",
			Port:          2587,
			Username:      "resend",
			Password:      apiKey,
			WebhookSecret: testSvixSecret,
		},
	}
}

// withStubResend points the provider at a stub API for the duration of a test.
func withStubResend(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := resendAPIBaseURL
	resendAPIBaseURL = srv.URL
	t.Cleanup(func() { resendAPIBaseURL = prev; srv.Close() })
	return srv
}

func TestResendWebhookProvider_RegistersMissingEndpoint(t *testing.T) {
	var created map[string]interface{}
	withStubResend(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer re_test", r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/webhooks":
			json.NewEncoder(w).Encode(resendWebhookList{Data: nil})
		case r.Method == http.MethodPost && r.URL.Path == "/webhooks":
			json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resendWebhookRecord{ID: "wh_1", SigningSecret: "whsec_new"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	p := NewResendWebhookProvider(logger.NewLogger())
	status, err := p.RegisterWebhooks(context.Background(), "epragma", "int-1", "https://newsletters.e-pragma.gr",
		[]domain.EmailEventType{domain.EmailEventBounce, domain.EmailEventComplaint}, resendProviderForTest("re_test"))
	require.NoError(t, err)

	assert.True(t, status.IsRegistered)
	assert.Equal(t, "wh_1", status.Endpoints[0].WebhookID)

	endpoint, _ := created["endpoint"].(string)
	assert.Contains(t, endpoint, "/webhooks/email?provider=resend")
	assert.Contains(t, endpoint, "workspace_id=epragma")
	assert.Contains(t, endpoint, "integration_id=int-1",
		"the integration id is what makes the handler able to find a verifier")

	events, _ := created["events"].([]interface{})
	assert.Len(t, events, 2)
	assert.Contains(t, events, "email.bounced")
	assert.Contains(t, events, "email.complained")

	// The signing secret is only returned at creation; losing it would leave every event
	// unverifiable, so it must be surfaced.
	assert.Equal(t, "whsec_new", status.ProviderDetails["signing_secret"])
}

func TestResendWebhookProvider_IsIdempotent(t *testing.T) {
	posts := 0
	withStubResend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		json.NewEncoder(w).Encode(resendWebhookList{Data: []resendWebhookRecord{
			{ID: "wh_existing", Endpoint: "https://newsletters.e-pragma.gr/webhooks/email?provider=resend&workspace_id=epragma&integration_id=int-1", Status: "enabled"},
		}})
	})

	p := NewResendWebhookProvider(logger.NewLogger())
	status, err := p.RegisterWebhooks(context.Background(), "epragma", "int-1", "https://newsletters.e-pragma.gr",
		[]domain.EmailEventType{domain.EmailEventBounce}, resendProviderForTest("re_test"))
	require.NoError(t, err)

	assert.True(t, status.IsRegistered)
	assert.Equal(t, "wh_existing", status.Endpoints[0].WebhookID)
	assert.Equal(t, 0, posts, "an existing endpoint must not be duplicated")
}

func TestResendWebhookProvider_StatusReportsRegistered(t *testing.T) {
	withStubResend(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resendWebhookList{Data: []resendWebhookRecord{
			{ID: "wh_1", Endpoint: "https://x/webhooks/email?integration_id=int-9", Status: "enabled"},
			{ID: "wh_2", Endpoint: "https://x/webhooks/email?integration_id=other", Status: "enabled"},
		}})
	})

	p := NewResendWebhookProvider(logger.NewLogger())
	status, err := p.GetWebhookStatus(context.Background(), "epragma", "int-9", resendProviderForTest("re_test"))
	require.NoError(t, err)
	assert.True(t, status.IsRegistered)
	assert.Len(t, status.Endpoints, 1, "only this integration's endpoint counts")
}

func TestResendWebhookProvider_UnregisterDeletesOnlyOurs(t *testing.T) {
	deleted := []string{}
	withStubResend(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(resendWebhookList{Data: []resendWebhookRecord{
				{ID: "wh_1", Endpoint: "https://x/webhooks/email?integration_id=int-9"},
				{ID: "wh_2", Endpoint: "https://x/webhooks/email?integration_id=other"},
			}})
		case http.MethodDelete:
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/webhooks/"))
			w.WriteHeader(http.StatusOK)
		}
	})

	p := NewResendWebhookProvider(logger.NewLogger())
	require.NoError(t, p.UnregisterWebhooks(context.Background(), "epragma", "int-9", resendProviderForTest("re_test")))
	assert.Equal(t, []string{"wh_1"}, deleted, "deleting an integration must not remove another one's webhook")
}

func TestResendWebhookProvider_RequiresAPIKey(t *testing.T) {
	p := NewResendWebhookProvider(logger.NewLogger())
	_, err := p.RegisterWebhooks(context.Background(), "ws", "int", "https://x",
		[]domain.EmailEventType{domain.EmailEventBounce},
		&domain.EmailProvider{Kind: domain.EmailProviderKindResend, Resend: &domain.ResendSettings{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestResendWebhookProvider_SurfacesAPIErrors(t *testing.T) {
	withStubResend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(resendWebhookList{})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Invalid API key"}`))
	})

	p := NewResendWebhookProvider(logger.NewLogger())
	_, err := p.RegisterWebhooks(context.Background(), "ws", "int", "https://x",
		[]domain.EmailEventType{domain.EmailEventBounce}, resendProviderForTest("re_bad"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "Invalid API key")
}
