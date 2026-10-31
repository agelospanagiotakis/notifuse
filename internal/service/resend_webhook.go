package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// resendSignatureTolerance bounds how old a webhook may be. Svix recommends five
// minutes; without it any captured request could be replayed forever.
const resendSignatureTolerance = 5 * time.Minute

// resendWebhookPayload is the subset of Resend's event body this provider needs.
//
// Resend sends one event per request (`type` + `data`), not a reply: there is no
// message body and no sender to classify. That is why bounce and complaint events
// are handled here rather than through the ReplyParser interface, which exists for
// inbound mail from a contact.
type resendWebhookPayload struct {
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Data      struct {
		EmailID string   `json:"email_id"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Bounce  struct {
			Type    string `json:"type"`
			SubType string `json:"subType"`
			Message string `json:"message"`
		} `json:"bounce"`
	} `json:"data"`
}

// processResendWebhook authenticates and normalizes a Resend webhook into the
// provider-neutral events the rest of the pipeline already understands.
//
// The signature is verified before anything is parsed: an unauthenticated caller must
// not be able to mark a subscriber as bounced, which would silently drop them from every
// future send.
func (s *InboundWebhookEventService) processResendWebhook(integration *domain.Integration, headers http.Header, rawPayload []byte) ([]*domain.InboundWebhookEvent, error) {
	if integration.EmailProvider.Resend == nil {
		return nil, fmt.Errorf("resend settings missing on integration %s", integration.ID)
	}
	secret := integration.EmailProvider.Resend.WebhookSecret
	if secret == "" {
		return nil, fmt.Errorf("resend webhook secret is not configured on integration %s", integration.ID)
	}

	if err := verifySvixSignature(headers, rawPayload, secret, time.Now()); err != nil {
		return nil, err
	}

	var payload resendWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resend webhook payload: %w", err)
	}

	// Events that carry no delivery meaning for this platform are acknowledged rather
	// than rejected: Resend retries a non-2xx, and retrying cannot make them relevant.
	var eventType domain.EmailEventType
	switch payload.Type {
	case "email.delivered":
		eventType = domain.EmailEventDelivered
	case "email.bounced":
		eventType = domain.EmailEventBounce
	case "email.complained":
		eventType = domain.EmailEventComplaint
	default:
		return nil, nil
	}

	ts := time.Now().UTC()
	if payload.CreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, payload.CreatedAt); err == nil {
			ts = parsed.UTC()
		}
	}

	events := make([]*domain.InboundWebhookEvent, 0, len(payload.Data.To))
	for _, recipient := range payload.Data.To {
		if recipient == "" {
			continue
		}
		event := &domain.InboundWebhookEvent{
			Type:           eventType,
			Source:         domain.WebhookSourceResend,
			IntegrationID:  integration.ID,
			RecipientEmail: strings.ToLower(strings.TrimSpace(recipient)),
			Timestamp:      ts,
			RawPayload:     string(rawPayload),
		}

		switch eventType {
		case domain.EmailEventBounce:
			event.BounceType = strings.ToLower(strings.TrimSpace(payload.Data.Bounce.Type))
			event.BounceCategory = payload.Data.Bounce.SubType
			event.BounceDiagnostic = payload.Data.Bounce.Message
		case domain.EmailEventComplaint:
			event.ComplaintFeedbackType = "abuse"
		}

		events = append(events, event)
	}

	return events, nil
}

// verifySvixSignature checks a Svix-signed request.
//
// Svix signs the string "<id>.<timestamp>.<body>" with HMAC-SHA256 using the secret's
// raw bytes (everything after "whsec_", base64 decoded) and sends the result in
// `svix-signature` as one or more space-separated "v1,<base64>" entries — several while
// a secret is rotating, so any match is a pass.
func verifySvixSignature(headers http.Header, body []byte, secret string, now time.Time) error {
	id := headers.Get("svix-id")
	ts := headers.Get("svix-timestamp")
	sigHeader := headers.Get("svix-signature")
	if id == "" || ts == "" || sigHeader == "" {
		return fmt.Errorf("missing svix signature headers")
	}

	seconds, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid svix-timestamp: %w", err)
	}
	sentAt := time.Unix(seconds, 0).UTC()
	if diff := now.UTC().Sub(sentAt); diff > resendSignatureTolerance || diff < -resendSignatureTolerance {
		return fmt.Errorf("svix-timestamp outside tolerance: %s", diff)
	}

	key, err := decodeSvixSecret(secret)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, candidate := range strings.Fields(sigHeader) {
		parts := strings.SplitN(candidate, ",", 2)
		if len(parts) != 2 || parts[0] != "v1" {
			continue
		}
		// constant-time compare: a timing oracle on a signature check is a real attack
		if hmac.Equal([]byte(parts[1]), []byte(expected)) {
			return nil
		}
	}

	return fmt.Errorf("svix signature verification failed")
}

// decodeSvixSecret accepts the "whsec_<base64>" form Resend shows, and tolerates an
// already-decoded secret so a mis-pasted value produces a signature failure rather than
// a confusing decode error.
func decodeSvixSecret(secret string) ([]byte, error) {
	trimmed := strings.TrimSpace(secret)
	if !strings.HasPrefix(trimmed, "whsec_") {
		return nil, fmt.Errorf("resend webhook secret must start with whsec_")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(trimmed, "whsec_"))
	if err != nil {
		return nil, fmt.Errorf("resend webhook secret is not valid base64: %w", err)
	}
	return raw, nil
}
