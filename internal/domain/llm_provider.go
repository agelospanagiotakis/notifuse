package domain

import "fmt"

// LLMProviderKind defines the type of LLM provider
type LLMProviderKind string

const (
	LLMProviderKindAnthropic LLMProviderKind = "anthropic"
	LLMProviderKindOpenAI    LLMProviderKind = "openai"
	LLMProviderKindGemini    LLMProviderKind = "gemini"
	// DeepSeek speaks the OpenAI wire format but is a separate provider in the UI: it has its
	// own host, its own model names and its own key, and folding it into "OpenAI with a custom
	// base URL" made every operator read release notes to discover that.
	LLMProviderKindDeepSeek LLMProviderKind = "deepseek"
)

// LLMProvider contains configuration for an LLM service provider
type LLMProvider struct {
	Kind      LLMProviderKind    `json:"kind"`
	Anthropic *AnthropicSettings `json:"anthropic,omitempty"`
	OpenAI    *OpenAISettings    `json:"openai,omitempty"`
	Gemini    *GeminiSettings    `json:"gemini,omitempty"`
	DeepSeek  *DeepSeekSettings  `json:"deepseek,omitempty"`
}

// Validate validates the LLM provider settings
func (l *LLMProvider) Validate(passphrase string) error {
	if l.Kind == "" {
		return fmt.Errorf("LLM provider kind is required")
	}

	switch l.Kind {
	case LLMProviderKindAnthropic:
		if l.Anthropic == nil {
			return fmt.Errorf("Anthropic settings required when LLM provider kind is anthropic")
		}
		return l.Anthropic.Validate(passphrase)
	case LLMProviderKindOpenAI:
		if l.OpenAI == nil {
			return fmt.Errorf("OpenAI settings required when LLM provider kind is openai")
		}
		return l.OpenAI.Validate(passphrase)
	case LLMProviderKindGemini:
		if l.Gemini == nil {
			return fmt.Errorf("Gemini settings required when LLM provider kind is gemini")
		}
		return l.Gemini.Validate(passphrase)
	case LLMProviderKindDeepSeek:
		if l.DeepSeek == nil {
			return fmt.Errorf("DeepSeek settings required when LLM provider kind is deepseek")
		}
		return l.DeepSeek.Validate(passphrase)
	default:
		return fmt.Errorf("invalid LLM provider kind: %s", l.Kind)
	}
}

// EncryptSecretKeys encrypts all secret keys in the LLM provider
func (l *LLMProvider) EncryptSecretKeys(passphrase string) error {
	if l.Kind == LLMProviderKindAnthropic && l.Anthropic != nil && l.Anthropic.APIKey != "" {
		if err := l.Anthropic.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		l.Anthropic.APIKey = ""
	}

	if l.Kind == LLMProviderKindOpenAI && l.OpenAI != nil && l.OpenAI.APIKey != "" {
		if err := l.OpenAI.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		l.OpenAI.APIKey = ""
	}

	if l.Kind == LLMProviderKindGemini && l.Gemini != nil && l.Gemini.APIKey != "" {
		if err := l.Gemini.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		l.Gemini.APIKey = ""
	}

	if l.Kind == LLMProviderKindDeepSeek && l.DeepSeek != nil && l.DeepSeek.APIKey != "" {
		if err := l.DeepSeek.EncryptAPIKey(passphrase); err != nil {
			return err
		}
		l.DeepSeek.APIKey = ""
	}

	return nil
}

// DecryptSecretKeys decrypts all encrypted secret keys in the LLM provider
func (l *LLMProvider) DecryptSecretKeys(passphrase string) error {
	if l.Kind == LLMProviderKindAnthropic && l.Anthropic != nil && l.Anthropic.EncryptedAPIKey != "" {
		if err := l.Anthropic.DecryptAPIKey(passphrase); err != nil {
			return err
		}
	}

	if l.Kind == LLMProviderKindOpenAI && l.OpenAI != nil && l.OpenAI.EncryptedAPIKey != "" {
		if err := l.OpenAI.DecryptAPIKey(passphrase); err != nil {
			return err
		}
	}

	if l.Kind == LLMProviderKindGemini && l.Gemini != nil && l.Gemini.EncryptedAPIKey != "" {
		if err := l.Gemini.DecryptAPIKey(passphrase); err != nil {
			return err
		}
	}

	if l.Kind == LLMProviderKindDeepSeek && l.DeepSeek != nil && l.DeepSeek.EncryptedAPIKey != "" {
		if err := l.DeepSeek.DecryptAPIKey(passphrase); err != nil {
			return err
		}
	}

	return nil
}

// PreserveSecretsFrom keeps the stored secret when an update arrives without one.
//
// UpdateIntegration replaces the whole provider block from the request, and the console cannot
// resend a key it never receives — so every save that is not "I typed a new key" would otherwise
// wipe the credential and leave an integration that validates, saves, and then fails at the first
// message with "API key is not configured". That is exactly what happened to the DeepSeek
// integration: the preserve logic lived in the service, enumerating providers by hand, and the
// fourth one was forgotten.
//
// It lives here, beside the Validate/Encrypt/Decrypt switches, so a new provider has one obvious
// place to add its branch instead of a second, secret copy of the list in the service.
//
// Only the secret is preserved. Non-secret fields are taken verbatim from the request, which is
// why the console always resends model/base_url — an omitted one resets to its zero value.
func (l *LLMProvider) PreserveSecretsFrom(existing *LLMProvider) {
	if l == nil || existing == nil || l.Kind != existing.Kind {
		// A kind change is a different provider: its old secret does not belong to it.
		return
	}
	switch l.Kind {
	case LLMProviderKindAnthropic:
		if l.Anthropic != nil && existing.Anthropic != nil &&
			l.Anthropic.APIKey == "" && l.Anthropic.EncryptedAPIKey == "" {
			l.Anthropic.EncryptedAPIKey = existing.Anthropic.EncryptedAPIKey
		}
	case LLMProviderKindOpenAI:
		if l.OpenAI != nil && existing.OpenAI != nil &&
			l.OpenAI.APIKey == "" && l.OpenAI.EncryptedAPIKey == "" {
			l.OpenAI.EncryptedAPIKey = existing.OpenAI.EncryptedAPIKey
		}
	case LLMProviderKindGemini:
		if l.Gemini != nil && existing.Gemini != nil &&
			l.Gemini.APIKey == "" && l.Gemini.EncryptedAPIKey == "" {
			l.Gemini.EncryptedAPIKey = existing.Gemini.EncryptedAPIKey
		}
	case LLMProviderKindDeepSeek:
		if l.DeepSeek != nil && existing.DeepSeek != nil &&
			l.DeepSeek.APIKey == "" && l.DeepSeek.EncryptedAPIKey == "" {
			l.DeepSeek.EncryptedAPIKey = existing.DeepSeek.EncryptedAPIKey
		}
	}
}
