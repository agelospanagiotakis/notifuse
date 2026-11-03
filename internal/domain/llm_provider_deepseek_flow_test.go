package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The console sends exactly this shape when a DeepSeek integration is created.
const deepSeekCreatePayload = `{
  "workspace_id": "koisoptics",
  "name": "DeepSeek",
  "type": "llm",
  "llm_provider": {
    "kind": "deepseek",
    "deepseek": { "api_key": "sk-test-key", "model": "deepseek-chat" }
  }
}`

func TestCreateIntegration_DeepSeekStoresTheKey(t *testing.T) {
	var req domain.CreateIntegrationRequest
	require.NoError(t, json.Unmarshal([]byte(deepSeekCreatePayload), &req))

	// What the HTTP handler does.
	require.NoError(t, req.Validate("passphrase"))

	require.NotNil(t, req.LLMProvider.DeepSeek)
	require.NotEmpty(t, req.LLMProvider.DeepSeek.EncryptedAPIKey,
		"the API key must be encrypted and stored at create time")

	// What addIntegration then does with the same object.
	integration := domain.Integration{
		ID:          "i1",
		Name:        req.Name,
		Type:        req.Type,
		LLMProvider: req.LLMProvider,
	}
	require.NoError(t, integration.Validate("passphrase"))
	require.NotEmpty(t, integration.LLMProvider.DeepSeek.EncryptedAPIKey,
		"validation must not drop what it already encrypted")
}
