// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
// This file contains tests for the configuration handling functionality.
package enterprisereports

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestValidateConfig tests that a properly configured Config object
// passes validation with no errors.
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

// TestValidateConfigMissingEnterprise tests that validation fails when
// the required enterprise slug is missing.
func TestValidateConfigMissingEnterprise(t *testing.T) {
	config := &Config{
		AuthMethod: "token",
		Token:      "test-token",
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "enterprise flag is required")
}

// TestValidateConfigMissingToken tests that validation fails when
// token authentication is selected but no token is provided.
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

// TestValidateConfigMissingAppFields tests that validation fails when
// GitHub App authentication is selected but required app fields are missing.
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

// TestValidateConfigUnknownAuthMethod tests that validation fails when
// an unrecognized authentication method is specified.
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

// TestValidateConfigNoReportsSelected tests that validation fails when
// no report type flags are set to true.
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

// TestNewRESTClient tests that a REST client can be successfully created
// with token authentication.
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

// TestNewRESTClientUnknownAuth tests that creating a REST client fails
// when an unrecognized authentication method is specified.
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

// TestUnknownFlagReturnsList tests that the custom flag error handler
// returns a list of valid flags when an unknown flag is provided.
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

// TestValidateConfigAuthMethodCaseInsensitive tests that auth method validation
// is case-insensitive (e.g., "ToKen" is accepted as "token").
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

// TestValidateConfigInvalidBaseURL tests that validation fails when
// an invalid base URL format is provided.
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

// TestValidateConfigTrimBaseURL tests that trailing slashes are removed
// from the base URL during validation.
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

// TestValidateConfigInvalidLogLevel tests that validation fails when
// an invalid log level is specified.
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

// TestValidateConfigLogLevelCaseInsensitive tests that log level validation
// is case-insensitive (e.g., "INFO" is accepted as "info").
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

// TestValidateConfigAppKeyFileNotExists tests that validation fails when
// GitHub App authentication is selected and the specified private key file does not exist.
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

// TestValidateConfigAccumulatesErrors tests that validation collects and
// reports multiple errors rather than stopping at the first error found.
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
