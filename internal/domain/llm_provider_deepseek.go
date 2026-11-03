package domain

import (
	"fmt"
)

// DeepSeek defaults. The API is OpenAI-compatible — same request shape, same streaming
// chunks, a different host — so it is configured and called through the OpenAI path rather
// than growing a fourth client.
const (
	// DeepSeekDefaultBaseURL is DeepSeek's OpenAI-compatible endpoint. Their docs accept
	// both https://api.deepseek.com and https://api.deepseek.com/v1; the shorter form is
	// what the SDK appends /chat/completions to correctly.
	DeepSeekDefaultBaseURL = "https://api.deepseek.com"
	// DeepSeekDefaultModel is the general chat model. deepseek-reasoner is the other one:
	// same endpoint, thinking tokens surfaced as reasoning_content (which streamChatOpenAI
	// already recognises).
	DeepSeekDefaultModel = "deepseek-chat"
)

// DeepSeekSettings configures a DeepSeek integration.
//
// The fields are embedded rather than duplicated: the wire format IS OpenAI's, and two
// copies of the same five fields would drift the first time one of them gains a flag. The
// methods below exist only to name DeepSeek in the errors — "model is required for OpenAI
// configuration" on a DeepSeek integration sends the operator looking in the wrong place.
type DeepSeekSettings struct {
	OpenAISettings
}

// DecryptAPIKey decrypts the stored API key.
func (d *DeepSeekSettings) DecryptAPIKey(passphrase string) error {
	if err := d.OpenAISettings.DecryptAPIKey(passphrase); err != nil {
		return fmt.Errorf("failed to decrypt DeepSeek API key: %w", err)
	}
	return nil
}

// EncryptAPIKey encrypts the API key before it is stored.
func (d *DeepSeekSettings) EncryptAPIKey(passphrase string) error {
	if err := d.OpenAISettings.EncryptAPIKey(passphrase); err != nil {
		return fmt.Errorf("failed to encrypt DeepSeek API key: %w", err)
	}
	return nil
}

// Validate validates the DeepSeek settings and encrypts the key when one was supplied.
func (d *DeepSeekSettings) Validate(passphrase string) error {
	if d.Model == "" {
		return fmt.Errorf("model is required for DeepSeek configuration")
	}
	if !IsValidReasoningEffort(d.ReasoningEffort) {
		return fmt.Errorf("invalid reasoning_effort %q for DeepSeek configuration", d.ReasoningEffort)
	}
	if d.APIKey != "" {
		if err := d.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt DeepSeek API key: %w", err)
		}
	}
	return nil
}

// WithDefaults fills in the host and model when the operator left them out, so an
// integration that was saved with only an API key still calls the right endpoint.
func (d *DeepSeekSettings) WithDefaults() {
	if d.BaseURL == "" {
		d.BaseURL = DeepSeekDefaultBaseURL
	}
	if d.Model == "" {
		d.Model = DeepSeekDefaultModel
	}
}
