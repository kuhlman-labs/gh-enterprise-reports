// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
// It handles configuration parsing, client initialization, and report generation for organizations,
// repositories, teams, collaborators, and users across a GitHub Enterprise instance.
package enterprisereports

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// DefaultConfigFilename is the default name for the config file (without extension).
	DefaultConfigFilename = "config"

	// DefaultConfigType is the default type of the config file.
	DefaultConfigType = "yml"

	// DefaultProfile is the default profile name.
	DefaultProfile = "default"

	// EnvPrefix is the prefix for environment variables.
	EnvPrefix = "GH_REPORT"
)

// ConfigManager manages configuration loading from different sources.
type ConfigManager struct {
	// viper instance used for configuration management
	v *viper.Viper

	// profile is the selected configuration profile
	profile string

	// configPaths are the paths where config files are searched
	configPaths []string

	// Config is the loaded configuration
	Config *Config
}

// NewConfigManager creates a new configuration manager.
func NewConfigManager() *ConfigManager {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Set default values
	v.SetDefault("workers", 5)
	v.SetDefault("auth-method", "token")
	v.SetDefault("log-level", "info")
	v.SetDefault("output-format", "csv")

	return &ConfigManager{
		v:           v,
		profile:     DefaultProfile,
		configPaths: []string{".", "$HOME/.gh-enterprise-reports"},
		Config:      &Config{},
	}
}

// InitializeFlags sets up the CLI flags and binds them to Viper.
func (cm *ConfigManager) InitializeFlags(rootCmd *cobra.Command) {
	// Add profile flag first, as it affects how the config is loaded
	rootCmd.PersistentFlags().StringP("profile", "p", DefaultProfile, "Configuration profile to use")
	rootCmd.PersistentFlags().StringP("config-file", "c", "", "Path to config file (default is ./config.yml, ~/.gh-enterprise-reports/config.yml)")

	// Report selection flags
	rootCmd.PersistentFlags().Bool("organizations", false, "Generate the organizations report")
	rootCmd.PersistentFlags().Bool("repositories", false, "Generate the repositories report")
	rootCmd.PersistentFlags().Bool("teams", false, "Generate the teams report")
	rootCmd.PersistentFlags().Bool("collaborators", false, "Generate the collaborators report")
	rootCmd.PersistentFlags().Bool("users", false, "Generate the users report")

	// Authentication flags
	rootCmd.PersistentFlags().String("auth-method", "token", "Authentication method (token or app)")
	rootCmd.PersistentFlags().String("token", "", "GitHub personal access token (required if auth-method is token)")
	rootCmd.PersistentFlags().Int64("app-id", 0, "GitHub App ID (required if auth-method is app)")
	rootCmd.PersistentFlags().String("app-private-key-file", "", "GitHub App private key file path (required if auth-method is app)")
	rootCmd.PersistentFlags().Int64("app-installation-id", 0, "GitHub App installation ID (required if auth-method is app)")

	// Enterprise and API settings
	rootCmd.PersistentFlags().String("enterprise", "", "Enterprise slug (required)")
	rootCmd.PersistentFlags().String("base-url", "", "Base URL for GitHub API (defaults to https://api.github.com)")

	// Format and output options
	rootCmd.PersistentFlags().String("output-format", "csv", "Output format for reports (csv, json, or xlsx)")
	rootCmd.PersistentFlags().String("output-dir", ".", "Directory where report files will be saved")

	// Other settings
	rootCmd.PersistentFlags().Int("workers", 5, "Number of concurrent workers for fetching data")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Bind flags to Viper
	cm.v.BindPFlags(rootCmd.PersistentFlags())
}

// LoadConfig loads the configuration from command line flags, environment variables, and config file.
func (cm *ConfigManager) LoadConfig() error {
	// First, get the profile and config file from flags/env vars
	cm.profile = cm.v.GetString("profile")
	configFile := cm.v.GetString("config-file")

	// If a specific config file is provided, use it
	if configFile != "" {
		cm.v.SetConfigFile(configFile)
		if err := cm.v.ReadInConfig(); err != nil {
			return utils.NewAppError(utils.ErrorTypeConfig, fmt.Sprintf("Error reading config file %s", configFile), err)
		}
	} else {
		// Otherwise, search for config files in standard locations
		cm.v.SetConfigName(DefaultConfigFilename)
		cm.v.SetConfigType(DefaultConfigType)

		for _, path := range cm.configPaths {
			expandedPath := os.ExpandEnv(path)
			cm.v.AddConfigPath(expandedPath)
		}

		// Try to read the config file but don't error if not found
		if err := cm.v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				// Only return error if it's not a "config file not found" error
				return utils.NewAppError(utils.ErrorTypeConfig, "Error reading config file", err)
			}
			// Log that no config file was found but continue
			slog.Info("No config file found, using only environment variables and flags")
		}
	}

	// If using a non-default profile, check if it exists in the config
	if cm.profile != DefaultProfile {
		profileSection := cm.v.GetStringMap("profiles." + cm.profile)
		if len(profileSection) == 0 {
			return utils.NewAppError(utils.ErrorTypeConfig,
				fmt.Sprintf("Profile '%s' not found in configuration", cm.profile), nil)
		}

		// Override with profile-specific values
		for k, v := range profileSection {
			cm.v.Set(k, v)
		}
	} else {
		// For default profile, check if profiles.default exists and apply those settings
		defaultProfileSection := cm.v.GetStringMap("profiles.default")
		if len(defaultProfileSection) > 0 {
			for k, v := range defaultProfileSection {
				cm.v.Set(k, v)
			}
		}
	}

	// Unmarshal the configuration into the Config struct
	if err := cm.v.Unmarshal(cm.Config); err != nil {
		return utils.NewAppError(utils.ErrorTypeConfig, "Error unmarshaling configuration", err)
	}

	// Output format - add to Config struct
	cm.Config.OutputFormat = cm.v.GetString("output-format")
	cm.Config.OutputDir = cm.v.GetString("output-dir")

	return cm.Config.Validate()
}

// CreateOutputFileName creates a filename for a report based on the report type and configuration.
func (cm *ConfigManager) CreateOutputFileName(reportType string) string {
	// Create a timestamp for unique filenames
	timestamp := time.Now().Format("2006-01-02_150405")

	// Construct the base filename
	baseFilename := fmt.Sprintf("%s_%s_%s", cm.Config.EnterpriseSlug, reportType, timestamp)

	// Add appropriate extension based on output format
	var extension string
	switch strings.ToLower(cm.Config.OutputFormat) {
	case "csv":
		extension = ".csv"
	case "json":
		extension = ".json"
	case "xlsx":
		extension = ".xlsx"
	default:
		extension = ".csv" // Default to CSV
	}

	// Join the output directory and filename
	return filepath.Join(cm.Config.OutputDir, baseFilename+extension)
}

// GetProfile returns the current active profile.
func (cm *ConfigManager) GetProfile() string {
	return cm.profile
}
