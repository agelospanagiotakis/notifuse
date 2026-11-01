package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSvixSecret is a fixed whsec_ secret so signatures are reproducible.
const testSvixSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func svixSignature(t *testing.T, secret, id, timestamp string, body []byte) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	require.NoError(t, err)
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func svixHeaders(t *testing.T, secret string, body []byte) http.Header {
	t.Helper()
	id := "msg_test_123"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", svixSignature(t, secret, id, ts, body))
	return h
}

func resendIntegration(secret string) *domain.Integration {
	return &domain.Integration{
		ID: "resend-integration",
		EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindResend,
			Resend: &domain.ResendSettings{
				Host:          "smtp.resend.com",
				Port:          2587,
				Username:      "resend",
				Password:      "re_test",
				WebhookSecret: secret,
			},
		},
	}
}

func TestVerifySvixSignature_Valid(t *testing.T) {
	body := []byte(`{"type":"email.bounced"}`)
	id := "msg_1"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", svixSignature(t, testSvixSecret, id, ts, body))

	assert.NoError(t, verifySvixSignature(h, body, testSvixSecret, time.Now()))
}

func TestVerifySvixSignature_RejectsTamperedBody(t *testing.T) {
	body := []byte(`{"type":"email.bounced"}`)
	id := "msg_1"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", svixSignature(t, testSvixSecret, id, ts, body))

	// One byte changed: the signature must no longer match. This is the difference
	// between "a bounce was reported" and "anyone can mark a subscriber as bounced".
	tampered := []byte(`{"type":"email.bounced" }`)
	assert.Error(t, verifySvixSignature(h, tampered, testSvixSecret, time.Now()))
}

func TestVerifySvixSignature_RejectsStaleTimestamp(t *testing.T) {
	body := []byte(`{"type":"email.delivered"}`)
	id := "msg_2"
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", svixSignature(t, testSvixSecret, id, ts, body))

	err := verifySvixSignature(h, body, testSvixSecret, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tolerance")
}

func TestVerifySvixSignature_RejectsMissingHeaders(t *testing.T) {
	err := verifySvixSignature(http.Header{}, []byte(`{}`), testSvixSecret, time.Now())
	assert.Error(t, err)
}

func TestVerifySvixSignature_RejectsWrongSecret(t *testing.T) {
	body := []byte(`{"type":"email.bounced"}`)
	id := "msg_3"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", svixSignature(t, testSvixSecret, id, ts, body))

	other := "whsec_c2Vjb25kLXNlY3JldC1mb3ItdGVzdHM="
	assert.Error(t, verifySvixSignature(h, body, other, time.Now()))
}

func TestProcessResendWebhook_HardBounce(t *testing.T) {
	body := []byte(`{
		"type":"email.bounced",
		"created_at":"2026-10-31T18:04:37.000Z",
		"data":{"email_id":"em_1","to":["Bounced@Example.com"],
		        "bounce":{"type":"hard","subType":"General","message":"550 5.1.1 user unknown"}}
	}`)
	svc := &InboundWebhookEventService{}

	events, err := svc.processResendWebhook(resendIntegration(testSvixSecret), svixHeaders(t, testSvixSecret, body), body)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.NotEmpty(t, events[0].ID, "the event id is a uuid primary key; an empty one fails the insert")
	assert.Equal(t, domain.EmailEventBounce, events[0].Type)
	assert.Equal(t, domain.WebhookSourceResend, events[0].Source)
	assert.Equal(t, "bounced@example.com", events[0].RecipientEmail, "recipient must be normalised to lower case")
	assert.Equal(t, "hard", events[0].BounceType)
	assert.Contains(t, events[0].BounceDiagnostic, "550")

	// and it must classify as a hard bounce, which is what suppresses the address
	assert.Equal(t, domain.BounceClassificationHard, domain.ClassifyBounce(domain.BounceInput{
		Provider:   domain.EmailProviderKindResend,
		Type:       events[0].BounceType,
		Subtype:    events[0].BounceCategory,
		Diagnostic: events[0].BounceDiagnostic,
	}))
}

func TestProcessResendWebhook_SoftBounceIsNotSuppressed(t *testing.T) {
	body := []byte(`{"type":"email.bounced","data":{"to":["soft@example.com"],"bounce":{"type":"soft"}}}`)
	svc := &InboundWebhookEventService{}

	events, err := svc.processResendWebhook(resendIntegration(testSvixSecret), svixHeaders(t, testSvixSecret, body), body)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, domain.BounceClassificationSoftCount, domain.ClassifyBounce(domain.BounceInput{
		Provider: domain.EmailProviderKindResend,
		Type:     events[0].BounceType,
	}))
}

func TestProcessResendWebhook_Complaint(t *testing.T) {
	body := []byte(`{"type":"email.complained","data":{"to":["angry@example.com"]}}`)
	svc := &InboundWebhookEventService{}

	events, err := svc.processResendWebhook(resendIntegration(testSvixSecret), svixHeaders(t, testSvixSecret, body), body)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
	assert.Equal(t, "abuse", events[0].ComplaintFeedbackType)
}

func TestProcessResendWebhook_MultipleRecipients(t *testing.T) {
	body := []byte(`{"type":"email.delivered","data":{"to":["a@example.com","b@example.com"]}}`)
	svc := &InboundWebhookEventService{}

	events, err := svc.processResendWebhook(resendIntegration(testSvixSecret), svixHeaders(t, testSvixSecret, body), body)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// Resend retries anything that is not a 2xx, so an event we cannot use must be
// acknowledged rather than rejected.
func TestProcessResendWebhook_IgnoresIrrelevantTypes(t *testing.T) {
	body := []byte(`{"type":"email.opened","data":{"to":["x@example.com"]}}`)
	svc := &InboundWebhookEventService{}

	events, err := svc.processResendWebhook(resendIntegration(testSvixSecret), svixHeaders(t, testSvixSecret, body), body)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestProcessResendWebhook_RequiresSecret(t *testing.T) {
	body := []byte(`{"type":"email.bounced","data":{"to":["x@example.com"]}}`)
	svc := &InboundWebhookEventService{}

	_, err := svc.processResendWebhook(resendIntegration(""), http.Header{}, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook secret")
}

func TestProcessResendWebhook_RejectsBadSignature(t *testing.T) {
	body := []byte(`{"type":"email.bounced","data":{"to":["x@example.com"]}}`)
	h := http.Header{}
	h.Set("svix-id", "msg")
	h.Set("svix-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	h.Set("svix-signature", "v1,bm90LWEtcmVhbC1zaWduYXR1cmU=")
	svc := &InboundWebhookEventService{}

	_, err := svc.processResendWebhook(resendIntegration(testSvixSecret), h, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

// The secret Resend displays must be accepted in its documented whsec_ form, and a
// malformed one must fail loudly rather than silently verifying nothing.
func TestDecodeSvixSecret(t *testing.T) {
	raw, err := decodeSvixSecret(testSvixSecret)
	assert.NoError(t, err)
	assert.NotEmpty(t, raw)

	_, err = decodeSvixSecret("not-a-secret")
	assert.Error(t, err)

	_, err = decodeSvixSecret("whsec_!!!notbase64")
	assert.Error(t, err)
}

func TestResendSettings_Validate(t *testing.T) {
	valid := &domain.ResendSettings{Password: "re_x", WebhookSecret: testSvixSecret}
	require.NoError(t, valid.Validate(""))
	assert.Equal(t, "smtp.resend.com", valid.Host, "host defaults")
	assert.Equal(t, 2587, valid.Port, "2587 is the port that survives a DigitalOcean block")
	assert.Equal(t, "resend", valid.Username)

	noSecret := &domain.ResendSettings{Password: "re_x"}
	assert.Error(t, noSecret.Validate(""), "a Resend provider without a webhook secret cannot verify events")

	badSecret := &domain.ResendSettings{Password: "re_x", WebhookSecret: "nope"}
	assert.Error(t, badSecret.Validate(""))

	noKey := &domain.ResendSettings{WebhookSecret: testSvixSecret}
	assert.Error(t, noKey.Validate(""))
}

func TestResendSettings_ToSMTP(t *testing.T) {
	r := &domain.ResendSettings{Host: "smtp.resend.com", Port: 2587, Username: "resend", Password: "re_key"}
	smtp := r.ToSMTP()
	assert.Equal(t, "smtp.resend.com", smtp.Host)
	assert.Equal(t, 2587, smtp.Port)
	assert.Equal(t, "resend", smtp.Username)
	assert.Equal(t, "re_key", smtp.Password)
	assert.True(t, smtp.UseTLS)
}

func TestResendSettings_EncryptDecryptRoundTrip(t *testing.T) {
	passphrase := "test-passphrase"
	r := &domain.ResendSettings{Password: "re_secret_key", WebhookSecret: testSvixSecret}

	require.NoError(t, r.EncryptSecrets(passphrase))
	assert.NotEmpty(t, r.EncryptedPassword)
	assert.NotEmpty(t, r.EncryptedWebhookSecret)
	assert.NotContains(t, r.EncryptedPassword, "re_secret_key", "the key must not survive encryption in plaintext")

	r2 := &domain.ResendSettings{
		EncryptedPassword:      r.EncryptedPassword,
		EncryptedWebhookSecret: r.EncryptedWebhookSecret,
	}
	require.NoError(t, r2.DecryptSecrets(passphrase))
	assert.Equal(t, "re_secret_key", r2.Password)
	assert.Equal(t, testSvixSecret, r2.WebhookSecret)
}

// A single signature entry that does not parse must not panic or accept.
func TestVerifySvixSignature_MalformedSignatureHeader(t *testing.T) {
	body := []byte(`{}`)
	h := http.Header{}
	h.Set("svix-id", "id")
	h.Set("svix-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	h.Set("svix-signature", fmt.Sprintf("v2,abc %s", "garbage"))
	assert.Error(t, verifySvixSignature(h, body, testSvixSecret, time.Now()))
}
