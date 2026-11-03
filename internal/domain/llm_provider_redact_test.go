package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The credential must never leave the process, for ANY provider. This is the leak that shipped:
// Redact emptied three providers by hand and left DeepSeek's plaintext key in the struct, so
// workspaces.get handed it to the browser.
func TestIntegration_Redact_ClearsEveryLLMKey(t *testing.T) {
	ds := &domain.DeepSeekSettings{}
	ds.APIKey = "sk-deepseek-secret"
	ds.EncryptedAPIKey = "ciphertext"

	i := &domain.Integration{
		ID:   "i1",
		Type: domain.IntegrationTypeLLM,
		LLMProvider: &domain.LLMProvider{
			Kind:     domain.LLMProviderKindDeepSeek,
			DeepSeek: ds,
		},
	}

	i.Redact()

	assert.Empty(t, i.LLMProvider.DeepSeek.APIKey, "the plaintext key must not survive redaction")
	// The ciphertext may stay: the console needs it to know a key exists, and it is not usable.
	assert.Equal(t, "ciphertext", i.LLMProvider.DeepSeek.EncryptedAPIKey)

	hint, ok := i.CredentialHints["deepseek.api_key"]
	assert.True(t, ok, "the console needs to know a key is configured")
	assert.NotContains(t, hint, "sk-deepseek-secret")
}

func TestLLMProvider_RedactSecrets_CoversEveryProvider(t *testing.T) {
	d := &domain.DeepSeekSettings{}
	d.APIKey = "ds-key"
	p := &domain.LLMProvider{
		Kind:      domain.LLMProviderKindDeepSeek,
		Anthropic: &domain.AnthropicSettings{APIKey: "a-key"},
		OpenAI:    &domain.OpenAISettings{APIKey: "o-key"},
		Gemini:    &domain.GeminiSettings{APIKey: "g-key"},
		DeepSeek:  d,
	}

	p.RedactSecrets()

	assert.Empty(t, p.Anthropic.APIKey)
	assert.Empty(t, p.OpenAI.APIKey)
	assert.Empty(t, p.Gemini.APIKey)
	assert.Empty(t, p.DeepSeek.APIKey)
}

func TestLLMProvider_SecretHints_NeverCarryTheSecret(t *testing.T) {
	d := &domain.DeepSeekSettings{}
	d.APIKey = "sk-abcdef123456"
	p := &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: d}

	hints := p.SecretHints()
	require.Contains(t, hints, "deepseek.api_key")
	assert.NotEqual(t, "sk-abcdef123456", hints["deepseek.api_key"])
	assert.Contains(t, hints["deepseek.api_key"], "6") // a suffix, not the value
}
