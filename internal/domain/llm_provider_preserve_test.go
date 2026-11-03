package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Every provider that stores a secret must survive an update that does not carry one. This is
// the bug that broke DeepSeek: the update path replaced the provider block and the preserve list
// named three providers, not four.
func TestLLMProvider_PreserveSecretsFrom(t *testing.T) {
	const stored = "deadbeef" // stands in for the encrypted form

	cases := []struct {
		name     string
		provided func() *domain.LLMProvider
		existing func() *domain.LLMProvider
		got      func(*domain.LLMProvider) string
	}{
		{
			name: "anthropic",
			provided: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindAnthropic, Anthropic: &domain.AnthropicSettings{Model: "m"}}
			},
			existing: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindAnthropic, Anthropic: &domain.AnthropicSettings{EncryptedAPIKey: stored}}
			},
			got: func(p *domain.LLMProvider) string { return p.Anthropic.EncryptedAPIKey },
		},
		{
			name: "openai",
			provided: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindOpenAI, OpenAI: &domain.OpenAISettings{Model: "m"}}
			},
			existing: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindOpenAI, OpenAI: &domain.OpenAISettings{EncryptedAPIKey: stored}}
			},
			got: func(p *domain.LLMProvider) string { return p.OpenAI.EncryptedAPIKey },
		},
		{
			name: "gemini",
			provided: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindGemini, Gemini: &domain.GeminiSettings{Model: "m"}}
			},
			existing: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindGemini, Gemini: &domain.GeminiSettings{EncryptedAPIKey: stored}}
			},
			got: func(p *domain.LLMProvider) string { return p.Gemini.EncryptedAPIKey },
		},
		{
			name: "deepseek",
			provided: func() *domain.LLMProvider {
				return &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: &domain.DeepSeekSettings{}}
			},
			existing: func() *domain.LLMProvider {
				d := &domain.DeepSeekSettings{}
				d.EncryptedAPIKey = stored
				return &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: d}
			},
			got: func(p *domain.LLMProvider) string { return p.DeepSeek.EncryptedAPIKey },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provided := tc.provided()
			provided.PreserveSecretsFrom(tc.existing())
			assert.Equal(t, stored, tc.got(provided), "an update without a key must keep the stored one")
		})
	}

	t.Run("a new key replaces the stored one", func(t *testing.T) {
		provided := &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: &domain.DeepSeekSettings{}}
		provided.DeepSeek.EncryptedAPIKey = "freshly-encrypted"
		existing := &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: &domain.DeepSeekSettings{}}
		existing.DeepSeek.EncryptedAPIKey = stored

		provided.PreserveSecretsFrom(existing)
		assert.Equal(t, "freshly-encrypted", provided.DeepSeek.EncryptedAPIKey)
	})

	t.Run("switching provider kind does not carry the old secret", func(t *testing.T) {
		provided := &domain.LLMProvider{Kind: domain.LLMProviderKindDeepSeek, DeepSeek: &domain.DeepSeekSettings{}}
		existing := &domain.LLMProvider{Kind: domain.LLMProviderKindOpenAI, OpenAI: &domain.OpenAISettings{EncryptedAPIKey: stored}}

		provided.PreserveSecretsFrom(existing)
		assert.Empty(t, provided.DeepSeek.EncryptedAPIKey)
	})
}
