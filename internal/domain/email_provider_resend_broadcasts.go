package domain

import (
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// ResendBroadcastsSettings configures the beta Resend Broadcasts/Audiences sender.
//
// It is deliberately separate from ResendSettings: Resend's transactional SMTP quota
// and the Marketing Broadcasts quota are different products. The beta sender creates a
// Resend audience per send batch, uploads the recipients, creates one Broadcast, and
// sends it. It does not use Notifuse's per-recipient queue, so the usual per-contact
// message history and link/open tracking are not available for this provider.
type ResendBroadcastsSettings struct {
	APIKey          string `json:"api_key,omitempty"`
	EncryptedAPIKey string `json:"encrypted_api_key,omitempty"`
}

func (r *ResendBroadcastsSettings) Validate(passphrase string) error {
	if r == nil {
		return fmt.Errorf("resend broadcasts settings required when email provider kind is resend_broadcasts")
	}
	if strings.TrimSpace(r.APIKey) == "" && strings.TrimSpace(r.EncryptedAPIKey) == "" {
		return fmt.Errorf("resend API key is required for the Broadcasts provider")
	}
	return nil
}

func (r *ResendBroadcastsSettings) EncryptAPIKey(passphrase string) error {
	if r == nil || r.APIKey == "" {
		return nil
	}
	enc, err := crypto.EncryptString(r.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt resend broadcasts API key: %w", err)
	}
	r.EncryptedAPIKey = enc
	return nil
}

func (r *ResendBroadcastsSettings) DecryptAPIKey(passphrase string) error {
	if r == nil || r.EncryptedAPIKey == "" {
		return nil
	}
	key, err := crypto.DecryptFromHexString(r.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt resend broadcasts API key: %w", err)
	}
	r.APIKey = key
	return nil
}
