package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestDeepSeekSettings_Validate(t *testing.T) {
	t.Run("accepts a key and a model, and encrypts the key", func(t *testing.T) {
		ds := &domain.DeepSeekSettings{}
		ds.Model = "deepseek-chat"
		ds.APIKey = "sk-test-key"

		require.NoError(t, ds.Validate("passphrase"))
		assert.NotEmpty(t, ds.EncryptedAPIKey, "the key must be stored encrypted")
		assert.NotContains(t, ds.EncryptedAPIKey, "sk-test-key")
	})

	t.Run("names DeepSeek in its errors, not OpenAI", func(t *testing.T) {
		// The fields are shared with OpenAI; the message must not be, or the operator goes
		// looking in the wrong integration.
		ds := &domain.DeepSeekSettings{}
		err := ds.Validate("passphrase")
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "DeepSeek"), "got %q", err.Error())
		assert.False(t, strings.Contains(err.Error(), "OpenAI"), "got %q", err.Error())
	})

	t.Run("rejects an unknown reasoning effort", func(t *testing.T) {
		ds := &domain.DeepSeekSettings{}
		ds.Model = "deepseek-reasoner"
		ds.ReasoningEffort = "extreme"
		require.Error(t, ds.Validate("passphrase"))
	})
}

func TestDeepSeekSettings_WithDefaults(t *testing.T) {
	ds := &domain.DeepSeekSettings{}
	ds.WithDefaults()

	assert.Equal(t, "https://api.deepseek.com", ds.BaseURL)
	assert.Equal(t, "deepseek-chat", ds.Model)

	// An explicit choice is never overwritten.
	ds = &domain.DeepSeekSettings{}
	ds.BaseURL = "https://proxy.internal/v1"
	ds.Model = "deepseek-reasoner"
	ds.WithDefaults()
	assert.Equal(t, "https://proxy.internal/v1", ds.BaseURL)
	assert.Equal(t, "deepseek-reasoner", ds.Model)
}

func TestLLMProvider_DeepSeekRoundTrip(t *testing.T) {
	p := &domain.LLMProvider{
		Kind:     domain.LLMProviderKindDeepSeek,
		DeepSeek: &domain.DeepSeekSettings{},
	}
	p.DeepSeek.Model = "deepseek-chat"
	p.DeepSeek.APIKey = "sk-test-key"

	require.NoError(t, p.EncryptSecretKeys("passphrase"))
	assert.Empty(t, p.DeepSeek.APIKey, "the plaintext key must not survive encryption")

	require.NoError(t, p.DecryptSecretKeys("passphrase"))
	assert.Equal(t, "sk-test-key", p.DeepSeek.APIKey)
}

func TestLLMProvider_DeepSeekKindRequiresItsSettings(t *testing.T) {
	p := &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek}
	err := p.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DeepSeek settings required")

	// The OpenAI block is not a stand-in for it.
	p = &domain.LLMProvider{
		Kind:   domain.LLMProviderKindDeepSeek,
		OpenAI: &domain.OpenAISettings{Model: "gpt-4.1"},
	}
	require.Error(t, p.Validate("passphrase"))
}
