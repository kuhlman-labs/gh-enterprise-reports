package enterprisereports

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigManager(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir := t.TempDir()

	// Create a test config file with profiles
	configContent := `
enterprise: "test-enterprise"
token: "test-token"
auth-method: "token"
log-level: "info"
workers: 5
output-format: "csv"
output-dir: "./reports"

# Profile configurations
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
`
	configPath := filepath.Join(tempDir, "config.yml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Run("LoadDefaultProfile", func(t *testing.T) {
		// Create a config manager and point it to our test file
		cm := NewConfigManager()

		// Create a mock command
		cmd := createMockCommand()
		cm.InitializeFlags(cmd)

		// Set the config file path
		err := os.Setenv("GH_REPORT_CONFIG_FILE", configPath)
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_CONFIG_FILE")

		// Load the configuration
		err = cm.LoadConfig()
		require.NoError(t, err)

		// Check the default profile values
		assert.Equal(t, "test-enterprise", cm.Config.EnterpriseSlug)
		assert.Equal(t, "test-token", cm.Config.Token)
		assert.Equal(t, 5, cm.Config.Workers)
		assert.Equal(t, "csv", cm.Config.OutputFormat)
		assert.True(t, cm.Config.Organizations)
		assert.True(t, cm.Config.Repositories)
		assert.True(t, cm.Config.Teams)
	})

	t.Run("LoadMinimalProfile", func(t *testing.T) {
		// Create a config manager and point it to our test file
		cm := NewConfigManager()

		// Create a mock command
		cmd := createMockCommand()
		cm.InitializeFlags(cmd)

		// Set the config file path and profile
		err := os.Setenv("GH_REPORT_CONFIG_FILE", configPath)
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_CONFIG_FILE")

		err = os.Setenv("GH_REPORT_PROFILE", "minimal")
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_PROFILE")

		// Load the configuration
		err = cm.LoadConfig()
		require.NoError(t, err)

		// Check the minimal profile values
		assert.Equal(t, "test-enterprise", cm.Config.EnterpriseSlug)
		assert.Equal(t, 2, cm.Config.Workers)
		assert.True(t, cm.Config.Organizations)
		assert.False(t, cm.Config.Repositories)
	})

	t.Run("LoadCustomProfile", func(t *testing.T) {
		// Create a config manager and point it to our test file
		cm := NewConfigManager()

		// Create a mock command
		cmd := createMockCommand()
		cm.InitializeFlags(cmd)

		// Set the config file path and profile
		err := os.Setenv("GH_REPORT_CONFIG_FILE", configPath)
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_CONFIG_FILE")

		err = os.Setenv("GH_REPORT_PROFILE", "custom")
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_PROFILE")

		// Load the configuration
		err = cm.LoadConfig()
		require.NoError(t, err)

		// Check the custom profile values
		assert.Equal(t, "json", cm.Config.OutputFormat)
		assert.False(t, cm.Config.Organizations)
		assert.True(t, cm.Config.Repositories)
		assert.False(t, cm.Config.Teams)
	})

	t.Run("EnvironmentVariableOverride", func(t *testing.T) {
		// Create a config manager and point it to our test file
		cm := NewConfigManager()

		// Create a mock command
		cmd := createMockCommand()
		cm.InitializeFlags(cmd)

		// Set the config file path
		err := os.Setenv("GH_REPORT_CONFIG_FILE", configPath)
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_CONFIG_FILE")

		// Override some values with environment variables
		err = os.Setenv("GH_REPORT_WORKERS", "10")
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_WORKERS")

		err = os.Setenv("GH_REPORT_OUTPUT_FORMAT", "xlsx")
		require.NoError(t, err)
		defer os.Unsetenv("GH_REPORT_OUTPUT_FORMAT")

		// Load the configuration
		err = cm.LoadConfig()
		require.NoError(t, err)

		// Check that environment variables override config file values
		assert.Equal(t, 10, cm.Config.Workers)
		assert.Equal(t, "xlsx", cm.Config.OutputFormat)
	})

	t.Run("CreateOutputFileName", func(t *testing.T) {
		// Create a config manager
		cm := NewConfigManager()
		cm.Config = &Config{
			EnterpriseSlug: "test-enterprise",
			OutputFormat:   "json",
			OutputDir:      tempDir,
		}

		// Test file name creation
		filename := cm.CreateOutputFileName("organizations")
		assert.Contains(t, filename, "test-enterprise_organizations_")
		assert.Contains(t, filename, ".json")
		assert.Contains(t, filename, tempDir)
	})
}

// Helper function to create a mock cobra.Command for testing
func createMockCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
}
