package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResendBroadcastsSettingsValidateAndEncrypt(t *testing.T) {
	settings := &ResendBroadcastsSettings{}
	require.Error(t, settings.Validate("secret"))

	settings.APIKey = "re_test"
	require.NoError(t, settings.Validate("secret"))
	require.NoError(t, settings.EncryptAPIKey("secret"))
	require.NotEmpty(t, settings.EncryptedAPIKey)

	settings.APIKey = ""
	require.NoError(t, settings.DecryptAPIKey("secret"))
	assert.Equal(t, "re_test", settings.APIKey)
}

func TestResendBroadcastsProviderIsMarketingOnly(t *testing.T) {
	assert.True(t, EmailProviderKindResendBroadcasts.IsMarketingOnly())
	assert.False(t, EmailProviderKindResend.IsMarketingOnly())
	assert.False(t, EmailProviderKindResendBroadcasts.IsTransactionalOnly())
}
