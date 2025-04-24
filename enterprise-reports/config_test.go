package enterprisereports

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "token",
		Token:          "test-token",
		Organizations:  true, // at least one report flag
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

// Ensure Validate() errors when no report flag is set.
func TestValidateConfigNoReportsSelected(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		AuthMethod:     "token",
		Token:          "test-token",
		// no report flags true
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no report selected")
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

func TestUnknownFlagReturnsList(t *testing.T) {
	root := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {}, // no-op
	}
	config := &Config{}
	InitializeFlags(root, config)

	// misspell "organizations"
	root.SetArgs([]string{"--organizatins"})
	err := root.Execute()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "valid flags")
	assert.Contains(t, err.Error(), "--organizations")
	assert.Contains(t, err.Error(), "--repositories")
}

// Auth-method should be case-insensitive.
func TestValidateConfigAuthMethodCaseInsensitive(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug: "e",
		AuthMethod:     "ToKen",
		Token:          "t",
		Organizations:  true,
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Reject invalid BaseURL and trim trailing slash.
func TestValidateConfigInvalidBaseURL(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug: "e",
		AuthMethod:     "token",
		Token:          "t",
		Organizations:  true,
		BaseURL:        "://bad",
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base-url")
}

func TestValidateConfigTrimBaseURL(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug: "e",
		AuthMethod:     "token",
		Token:          "t",
		Organizations:  true,
		BaseURL:        "https://example.com/",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", cfg.BaseURL)
}

// Reject invalid log levels and accept case-insensitive.
func TestValidateConfigInvalidLogLevel(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug: "e",
		AuthMethod:     "token",
		Token:          "t",
		Organizations:  true,
		LogLevel:       "verbose",
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log-level")
}

func TestValidateConfigLogLevelCaseInsensitive(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug: "e",
		AuthMethod:     "token",
		Token:          "t",
		Organizations:  true,
		LogLevel:       "INFO",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Check missing private-key file triggers error.
func TestValidateConfigAppKeyFileNotExists(t *testing.T) {
	cfg := &Config{
		EnterpriseSlug:          "e",
		AuthMethod:              "app",
		GithubAppID:             1,
		GithubAppPrivateKey:     "nonexistent.pem",
		GithubAppInstallationID: 2,
		Organizations:           true,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// Accumulate multiple validation errors.
func TestValidateConfigAccumulatesErrors(t *testing.T) {
	cfg := &Config{
		// missing enterprise, token, reports
		AuthMethod: "token",
	}
	err := cfg.Validate()
	assert.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "enterprise flag is required")
	assert.Contains(t, msg, "token is required")
	assert.Contains(t, msg, "no report selected")
}
