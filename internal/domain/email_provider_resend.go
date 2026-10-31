package domain

import (
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// Resend provider defaults. DigitalOcean blocks outbound 25/465/587 and Resend only
// offers implicit TLS on 465, so 2587 with STARTTLS is the port that works from these
// servers; 587 would be the obvious choice on an unblocked host.
const (
	ResendDefaultHost = "smtp.resend.com"
	ResendDefaultPort = 2587
	ResendDefaultUser = "resend"
)

// ResendSettings configures Resend (https://resend.com).
//
// Sending goes over Resend's SMTP endpoint rather than its HTTP API, so the transport
// fields are SMTP-shaped and are handed to the existing SMTP sender.
//
// Delivery feedback is the reason Resend is a provider kind of its own rather than just
// "some SMTP server". Resend signs its webhooks with Svix over the raw request, which
// carries the signature in headers rather than in the body like Mailgun or SES, so a
// provider that is only configured as generic SMTP cannot authenticate those events at
// all. Without the signing secret this provider refuses bounce and complaint events
// instead of trusting whatever arrives at the endpoint.
type ResendSettings struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`

	EncryptedPassword string `json:"encrypted_password,omitempty"`

	// WebhookSecret is the Svix signing secret Resend shows when the endpoint is
	// created, of the form whsec_<base64>.
	WebhookSecret          string `json:"webhook_secret,omitempty"`
	EncryptedWebhookSecret string `json:"encrypted_webhook_secret,omitempty"`
}

// Validate checks the transport configuration and requires a webhook secret, because a
// Resend provider that cannot verify events silently loses every bounce it is told about.
func (r *ResendSettings) Validate(passphrase string) error {
	if r == nil {
		return fmt.Errorf("resend settings required when email provider kind is resend")
	}

	if strings.TrimSpace(r.Host) == "" {
		r.Host = ResendDefaultHost
	}
	if r.Port <= 0 {
		r.Port = ResendDefaultPort
	}
	if strings.TrimSpace(r.Username) == "" {
		r.Username = ResendDefaultUser
	}

	if r.Password == "" && r.EncryptedPassword == "" {
		return fmt.Errorf("resend API key is required as the SMTP password")
	}

	if r.WebhookSecret == "" && r.EncryptedWebhookSecret == "" {
		return fmt.Errorf("resend webhook signing secret is required: without it bounce and complaint events cannot be verified")
	}
	if r.WebhookSecret != "" && !strings.HasPrefix(r.WebhookSecret, "whsec_") {
		return fmt.Errorf("resend webhook signing secret must start with whsec_")
	}

	return nil
}

// EncryptSecrets encrypts the SMTP password and the webhook secret for storage.
func (r *ResendSettings) EncryptSecrets(passphrase string) error {
	if r.Password != "" {
		enc, err := crypto.EncryptString(r.Password, passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt resend API key: %w", err)
		}
		r.EncryptedPassword = enc
	}
	if r.WebhookSecret != "" {
		enc, err := crypto.EncryptString(r.WebhookSecret, passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt resend webhook secret: %w", err)
		}
		r.EncryptedWebhookSecret = enc
	}
	return nil
}

// DecryptSecrets decrypts both secrets into the in-memory fields.
func (r *ResendSettings) DecryptSecrets(passphrase string) error {
	if r.EncryptedPassword != "" {
		pw, err := crypto.DecryptFromHexString(r.EncryptedPassword, passphrase)
		if err != nil {
			return fmt.Errorf("failed to decrypt resend API key: %w", err)
		}
		r.Password = pw
	}
	if r.EncryptedWebhookSecret != "" {
		secret, err := crypto.DecryptFromHexString(r.EncryptedWebhookSecret, passphrase)
		if err != nil {
			return fmt.Errorf("failed to decrypt resend webhook secret: %w", err)
		}
		r.WebhookSecret = secret
	}
	return nil
}

// ToSMTP renders the transport half as SMTPSettings so the existing SMTP sender can send
// it. A Resend provider is a Resend-shaped configuration over an SMTP transport; there is
// no reason to write a second sender.
func (r *ResendSettings) ToSMTP() *SMTPSettings {
	host := r.Host
	if strings.TrimSpace(host) == "" {
		host = ResendDefaultHost
	}
	port := r.Port
	if port <= 0 {
		port = ResendDefaultPort
	}
	user := r.Username
	if strings.TrimSpace(user) == "" {
		user = ResendDefaultUser
	}
	return &SMTPSettings{
		Host:     host,
		Port:     port,
		Username: user,
		Password: r.Password,
		UseTLS:   true,
		AuthType: "basic",
	}
}
