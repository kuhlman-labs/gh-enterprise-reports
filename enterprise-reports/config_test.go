package enterprisereports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "token",
		Token:          "test-token",
	}

	err := config.Validate()

	assert.NoError(t, err)
}

func TestValidateConfigMissingEnterprise(t *testing.T) {
	config := &Config{
		AuthMethod: "token",
		Token:      "test-token",
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "enterprise flag is required")
}

func TestValidateConfigMissingToken(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "token",
		// Token missing
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}

func TestValidateConfigMissingAppFields(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "app",
		// Missing GithubAppID, GithubAppPrivateKey, and GithubAppInstallationID
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "app-id, app-private-key, and app-installation-id are required")
}

func TestValidateConfigUnknownAuthMethod(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "unknown",
		Token:          "test-token",
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown auth-method")
}

func TestNewRESTClient(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		AuthMethod: "token",
		Token:      "test-token",
	}

	restClient, err := NewRESTClient(ctx, config)

	assert.NoError(t, err)
	assert.NotNil(t, restClient)
}

func TestNewRESTClientUnknownAuth(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "unknown",
		Token:          "test-token",
	}
	client, err := NewRESTClient(ctx, config)
	assert.Error(t, err)
	assert.Nil(t, client)
}
