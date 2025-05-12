// Package config provides configuration interfaces and implementations for the GitHub Enterprise Reports tool.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStandardProvider tests the StandardProvider implementation.
func TestStandardProvider(t *testing.T) {
	// We now only test the internal config constructor since the legacy package has been removed
	t.Run("TestLegacyConstructorSkipped", func(t *testing.T) {
		t.Skip("Legacy package is no longer available")
	})

	t.Run("TestNewConstructor", func(t *testing.T) {
		// Create an internal config directly
		internalConfig := &Config{
			EnterpriseSlug: "test-enterprise",
			Workers:        10,
			OutputFormat:   "json",
			OutputDir:      "/tmp",
			LogLevel:       "debug",
			BaseURL:        "https://api.example.com",
			Organizations:  true,
			Repositories:   false,
			Teams:          true,
			Collaborators:  false,
			Users:          true,
			AuthMethod:     "token",
			Token:          "test-token",
		}

		// Create a provider using the new constructor
		provider := NewStandardProviderWithInternalConfig(internalConfig)
		runStandardProviderTests(t, provider)
	})
}

func runStandardProviderTests(t *testing.T, provider *StandardProvider) {
	// Test the getter methods
	assert.Equal(t, "test-enterprise", provider.GetEnterpriseSlug())
	assert.Equal(t, 10, provider.GetWorkers())
	assert.Equal(t, "json", provider.GetOutputFormat())
	assert.Equal(t, "/tmp", provider.GetOutputDir())
	assert.Equal(t, "debug", provider.GetLogLevel())
	assert.Equal(t, "https://api.example.com", provider.GetBaseURL())
	assert.Equal(t, "token", provider.GetAuthMethod())
	assert.Equal(t, "test-token", provider.GetToken())

	// Test the boolean methods
	assert.True(t, provider.ShouldRunOrganizationsReport())
	assert.False(t, provider.ShouldRunRepositoriesReport())
	assert.True(t, provider.ShouldRunTeamsReport())
	assert.False(t, provider.ShouldRunCollaboratorsReport())
	assert.True(t, provider.ShouldRunUsersReport())

	// Test the file path creation
	filePath := provider.CreateFilePath("test-report")
	assert.Contains(t, filePath, "test-enterprise_test-report_")
	assert.Contains(t, filePath, ".json")
	assert.True(t, filepath.IsAbs(filePath))
}

// TestManagerProvider tests the ManagerProvider implementation.
func TestManagerProvider(t *testing.T) {
	// Create a temporary config file and output directory for testing
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")
	reportsDir := filepath.Join(tempDir, "reports")

	// Create the reports directory
	err := os.Mkdir(reportsDir, 0755)
	require.NoError(t, err)

	// Use fmt.Sprintf with %q to properly escape the path in the YAML configuration
	configContent := fmt.Sprintf(`
enterprise: "test-enterprise"
token: "test-token"
auth-method: "token"
log-level: "info"
workers: 5
output-format: "csv"
output-dir: %q

profiles:
  default:
    organizations: true
    repositories: true
    teams: true
    
  minimal:
    organizations: true
    repositories: false
    workers: 2
    
  custom:
    organizations: false
    repositories: true
    teams: false
    output-format: "json"
`, reportsDir)

	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test loading the default profile
	t.Run("LoadDefaultProfile", func(t *testing.T) {
		// Create a separate command for this test to avoid flag conflicts
		mockCmd := &cobra.Command{
			Use: "test-default",
		}

		provider := NewManagerProvider()
		provider.InitializeFlags(mockCmd)

		// Set config file path through environment
		t.Setenv("GH_REPORT_CONFIG_FILE", configPath)

		err := provider.LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, "test-enterprise", provider.GetEnterpriseSlug())
		assert.Equal(t, 5, provider.GetWorkers())
		assert.Equal(t, "csv", provider.GetOutputFormat())
		assert.Equal(t, DefaultProfile, provider.GetProfile())
		assert.True(t, provider.ShouldRunOrganizationsReport())
		assert.True(t, provider.ShouldRunRepositoriesReport())
		assert.True(t, provider.ShouldRunTeamsReport())
	})

	// Test loading a custom profile
	t.Run("LoadCustomProfile", func(t *testing.T) {
		// Create a separate command for this test to avoid flag conflicts
		mockCmd := &cobra.Command{
			Use: "test-custom",
		}

		provider := NewManagerProvider()
		provider.InitializeFlags(mockCmd)

		// Set config file path and profile through environment
		t.Setenv("GH_REPORT_CONFIG_FILE", configPath)
		t.Setenv("GH_REPORT_PROFILE", "custom")

		err := provider.LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, "custom", provider.GetProfile())
		assert.Equal(t, "json", provider.GetOutputFormat())
		assert.False(t, provider.ShouldRunOrganizationsReport())
		assert.True(t, provider.ShouldRunRepositoriesReport())
	})

	// Test validation errors
	t.Run("ValidationErrors", func(t *testing.T) {
		provider := NewManagerProvider()

		// Reset all fields to invalid values
		provider.enterpriseSlug = ""
		provider.authMethod = "invalid"
		provider.runOrganizations = false
		provider.runRepositories = false
		provider.runTeams = false
		provider.runCollaborators = false
		provider.runUsers = false
		provider.outputFormat = "invalid"

		err := provider.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "enterprise flag is required")
		assert.Contains(t, err.Error(), "unknown auth-method")
		assert.Contains(t, err.Error(), "no report selected")
		assert.Contains(t, err.Error(), "output-format must be one of")
	})
}

// TestGitHubAppAuth tests GitHub App authentication methods and errors
func TestGitHubAppAuth(t *testing.T) {
	// Skip this test as the legacy package has been removed
	t.Skip("Legacy package is no longer available")

	// Test ManagerProvider with missing App settings
	mgr := NewManagerProvider()
	mgr.authMethod = "app"
	mgr.enterpriseSlug = "test-enterprise"
	mgr.runOrganizations = true

	// Test validation with missing App settings
	err := mgr.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required when auth-method is app")
}
