// Package config provides configuration interfaces and implementations for the GitHub Enterprise Reports tool.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
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

// ManagerProvider implements the Provider interface using Viper for flexible configuration management.
// This provides support for profiles, environment variables, and config files.
type ManagerProvider struct {
	// viper instance used for configuration management
	v *viper.Viper

	// profile is the selected configuration profile
	profile string

	// configPaths are the paths where config files are searched
	configPaths []string

	// The underlying configuration values
	enterpriseSlug string
	workers        int
	outputFormat   string
	outputDir      string
	logLevel       string
	baseURL        string

	// Report selection flags
	runOrganizations bool
	runRepositories  bool
	runTeams         bool
	runCollaborators bool
	runUsers         bool

	// Auth settings
	authMethod      string
	token           string
	appID           int64
	appKeyFile      string
	appInstallation int64
}

// NewManagerProvider creates a new ManagerProvider with default settings.
func NewManagerProvider() *ManagerProvider {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Set default values
	v.SetDefault("workers", 5)
	v.SetDefault("auth-method", "token")
	v.SetDefault("log-level", "info")
	v.SetDefault("output-format", "csv")
	v.SetDefault("output-dir", ".")

	return &ManagerProvider{
		v:            v,
		profile:      DefaultProfile,
		configPaths:  []string{".", "$HOME/.gh-enterprise-reports"},
		workers:      5,
		logLevel:     "info",
		outputFormat: "csv",
		outputDir:    ".",
		authMethod:   "token",
	}
}

// InitializeFlags sets up the CLI flags and binds them to Viper.
func (m *ManagerProvider) InitializeFlags(rootCmd *cobra.Command) {
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
	if err := m.v.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		slog.Error("Failed to bind flags to viper", "error", err)
	}
}

// LoadConfig loads the configuration from command line flags, environment variables, and config file.
func (m *ManagerProvider) LoadConfig() error {
	// First, get the profile and config file from flags/env vars
	m.profile = m.v.GetString("profile")
	configFile := m.v.GetString("config-file")

	// If a specific config file is provided, use it
	if configFile != "" {
		m.v.SetConfigFile(configFile)
		if err := m.v.ReadInConfig(); err != nil {
			return utils.NewAppError(utils.ErrorTypeConfig, fmt.Sprintf("Error reading config file %s", configFile), err)
		}
	} else {
		// Otherwise, search for config files in standard locations
		m.v.SetConfigName(DefaultConfigFilename)
		m.v.SetConfigType(DefaultConfigType)

		for _, path := range m.configPaths {
			expandedPath := os.ExpandEnv(path)
			m.v.AddConfigPath(expandedPath)
		}

		// Try to read the config file but don't error if not found
		if err := m.v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				// Only return error if it's not a "config file not found" error
				return utils.NewAppError(utils.ErrorTypeConfig, "Error reading config file", err)
			}
			// Log that no config file was found but continue
			slog.Info("No config file found, using only environment variables and flags")
		}
	}

	// If using a non-default profile, check if it exists in the config
	if m.profile != DefaultProfile {
		profileSection := m.v.GetStringMap("profiles." + m.profile)
		if len(profileSection) == 0 {
			return utils.NewAppError(utils.ErrorTypeConfig,
				fmt.Sprintf("Profile '%s' not found in configuration", m.profile), nil)
		}

		// Override with profile-specific values
		for k, v := range profileSection {
			m.v.Set(k, v)
		}
	} else {
		// For default profile, check if profiles.default exists and apply those settings
		defaultProfileSection := m.v.GetStringMap("profiles.default")
		if len(defaultProfileSection) > 0 {
			for k, v := range defaultProfileSection {
				m.v.Set(k, v)
			}
		}
	}

	// Load all configuration values
	m.enterpriseSlug = m.v.GetString("enterprise")
	m.workers = m.v.GetInt("workers")
	m.outputFormat = m.v.GetString("output-format")
	m.outputDir = m.v.GetString("output-dir")
	m.logLevel = m.v.GetString("log-level")
	m.baseURL = m.v.GetString("base-url")

	m.runOrganizations = m.v.GetBool("organizations")
	m.runRepositories = m.v.GetBool("repositories")
	m.runTeams = m.v.GetBool("teams")
	m.runCollaborators = m.v.GetBool("collaborators")
	m.runUsers = m.v.GetBool("users")

	m.authMethod = m.v.GetString("auth-method")
	m.token = m.v.GetString("token")
	m.appID = m.v.GetInt64("app-id")
	m.appKeyFile = m.v.GetString("app-private-key-file")
	m.appInstallation = m.v.GetInt64("app-installation-id")

	return m.Validate()
}

// GetProfile returns the current active profile.
func (m *ManagerProvider) GetProfile() string {
	return m.profile
}

// GetEnterpriseSlug returns the enterprise slug.
func (m *ManagerProvider) GetEnterpriseSlug() string {
	return m.enterpriseSlug
}

// GetWorkers returns the number of workers.
func (m *ManagerProvider) GetWorkers() int {
	return m.workers
}

// GetOutputFormat returns the output format.
func (m *ManagerProvider) GetOutputFormat() string {
	return m.outputFormat
}

// GetOutputDir returns the output directory.
func (m *ManagerProvider) GetOutputDir() string {
	return m.outputDir
}

// GetLogLevel returns the log level.
func (m *ManagerProvider) GetLogLevel() string {
	return m.logLevel
}

// GetBaseURL returns the base URL.
func (m *ManagerProvider) GetBaseURL() string {
	return m.baseURL
}

// ShouldRunOrganizationsReport returns whether to run the organizations report.
func (m *ManagerProvider) ShouldRunOrganizationsReport() bool {
	return m.runOrganizations
}

// ShouldRunRepositoriesReport returns whether to run the repositories report.
func (m *ManagerProvider) ShouldRunRepositoriesReport() bool {
	return m.runRepositories
}

// ShouldRunTeamsReport returns whether to run the teams report.
func (m *ManagerProvider) ShouldRunTeamsReport() bool {
	return m.runTeams
}

// ShouldRunCollaboratorsReport returns whether to run the collaborators report.
func (m *ManagerProvider) ShouldRunCollaboratorsReport() bool {
	return m.runCollaborators
}

// ShouldRunUsersReport returns whether to run the users report.
func (m *ManagerProvider) ShouldRunUsersReport() bool {
	return m.runUsers
}

// GetAuthMethod returns the authentication method.
func (m *ManagerProvider) GetAuthMethod() string {
	return m.authMethod
}

// GetToken returns the token.
func (m *ManagerProvider) GetToken() string {
	return m.token
}

// GetAppID returns the GitHub App ID.
func (m *ManagerProvider) GetAppID() int64 {
	return m.appID
}

// GetAppPrivateKeyFile returns the GitHub App private key file.
func (m *ManagerProvider) GetAppPrivateKeyFile() string {
	return m.appKeyFile
}

// GetAppInstallationID returns the GitHub App installation ID.
func (m *ManagerProvider) GetAppInstallationID() int64 {
	return m.appInstallation
}

// CreateFilePath creates a file path for a report.
func (m *ManagerProvider) CreateFilePath(reportType string) string {
	timestamp := time.Now().Format("2006-01-02_15-04")
	filename := fmt.Sprintf("%s_%s_%s.%s", m.GetEnterpriseSlug(), reportType, timestamp, m.GetOutputFormat())
	return filepath.Join(m.GetOutputDir(), filename)
}

// Validate validates the configuration.
func (m *ManagerProvider) Validate() error {
	var errs []error

	// Enterprise slug is required
	if m.enterpriseSlug == "" {
		errs = append(errs, fmt.Errorf("enterprise flag is required"))
	}

	// Authentication validation
	switch m.authMethod {
	case "token":
		if m.token == "" {
			errs = append(errs, fmt.Errorf("token is required when auth-method is token"))
		}
	case "app":
		if m.appID == 0 {
			errs = append(errs, fmt.Errorf("app-id is required when auth-method is app"))
		}
		if m.appKeyFile == "" {
			errs = append(errs, fmt.Errorf("app-private-key-file is required when auth-method is app"))
		} else if _, err := os.Stat(m.appKeyFile); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("app-private-key-file %q does not exist", m.appKeyFile))
		}
		if m.appInstallation == 0 {
			errs = append(errs, fmt.Errorf("app-installation-id is required when auth-method is app"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown auth-method %q: please use 'token' or 'app'", m.authMethod))
	}

	// at least one report
	if !m.runOrganizations && !m.runRepositories && !m.runTeams &&
		!m.runCollaborators && !m.runUsers {
		errs = append(errs, fmt.Errorf("no report selected: please specify at least one of: organizations, repositories, teams, collaborators, users"))
	}

	// Output format validation
	validFormats := map[string]bool{"csv": true, "json": true, "xlsx": true}
	if !validFormats[strings.ToLower(m.outputFormat)] {
		errs = append(errs, fmt.Errorf("output-format must be one of: csv, json, xlsx; got %q", m.outputFormat))
	}

	// Output directory validation
	if m.outputDir != "" {
		if _, err := os.Stat(m.outputDir); err != nil {
			if os.IsNotExist(err) {
				// Try to create the directory
				if err := os.MkdirAll(m.outputDir, 0755); err != nil {
					errs = append(errs, fmt.Errorf("output-dir %q does not exist and could not be created: %v", m.outputDir, err))
				}
			} else {
				errs = append(errs, fmt.Errorf("error accessing output-dir %q: %v", m.outputDir, err))
			}
		}
	}

	// Log level validation
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true,
		"error": true, "fatal": true, "panic": true,
	}
	if !validLevels[strings.ToLower(m.logLevel)] {
		errs = append(errs, fmt.Errorf("log-level %q is not one of: debug, info, warn, error, fatal, panic", m.logLevel))
	}

	if len(errs) > 0 {
		combined := ""
		for i, err := range errs {
			if i > 0 {
				combined += "; "
			}
			combined += err.Error()
		}
		return fmt.Errorf("%s", combined) // Using a constant format string
	}
	return nil
}

// CreateRESTClient creates a GitHub REST client.
func (m *ManagerProvider) CreateRESTClient() (*github.Client, error) {
	ctx := context.Background()
	var client *github.Client

	switch m.GetAuthMethod() {
	case "token":
		if m.GetToken() == "" {
			return nil, fmt.Errorf("token required for token authentication")
		}
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: m.GetToken()},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
	case "app":
		// GitHub App authentication
		if m.GetAppID() == 0 {
			return nil, fmt.Errorf("app-id is required for GitHub App authentication")
		}
		if m.GetAppPrivateKeyFile() == "" {
			return nil, fmt.Errorf("app-private-key-file is required for GitHub App authentication")
		}
		if m.GetAppInstallationID() == 0 {
			return nil, fmt.Errorf("app-installation-id is required for GitHub App authentication")
		}

		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			m.GetAppID(),
			m.GetAppInstallationID(),
			m.GetAppPrivateKeyFile(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub App installation transport: %w", err)
		}
		client = github.NewClient(&http.Client{Transport: itr})
	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", m.GetAuthMethod())
	}

	// Set custom base URL if provided
	if m.GetBaseURL() != "" {
		baseURL, err := url.Parse(m.GetBaseURL())
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		client.BaseURL = baseURL
	}

	return client, nil
}

// CreateGraphQLClient creates a GitHub GraphQL client.
func (m *ManagerProvider) CreateGraphQLClient() (*githubv4.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var client *githubv4.Client

	switch m.GetAuthMethod() {
	case "token":
		if m.GetToken() == "" {
			return nil, fmt.Errorf("token required for token authentication")
		}
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: m.GetToken()})
		httpClient := oauth2.NewClient(ctx, src)

		// If a custom base URL is specified, use it with the GraphQL client
		if m.GetBaseURL() != "" {
			// Extract the hostname for GraphQL endpoint
			baseURL, err := url.Parse(m.GetBaseURL())
			if err != nil {
				return nil, fmt.Errorf("invalid base URL: %w", err)
			}

			// Construct GraphQL API endpoint (usually the REST API base URL with /graphql appended)
			graphqlURL := fmt.Sprintf("%s://%s/graphql", baseURL.Scheme, baseURL.Host)
			client = githubv4.NewEnterpriseClient(graphqlURL, httpClient)
		} else {
			// Use the standard GitHub GraphQL endpoint
			client = githubv4.NewClient(httpClient)
		}

	case "app":
		// GitHub App authentication
		if m.GetAppID() == 0 {
			return nil, fmt.Errorf("app-id is required for GitHub App authentication")
		}
		if m.GetAppPrivateKeyFile() == "" {
			return nil, fmt.Errorf("app-private-key-file is required for GitHub App authentication")
		}
		if m.GetAppInstallationID() == 0 {
			return nil, fmt.Errorf("app-installation-id is required for GitHub App authentication")
		}

		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			m.GetAppID(),
			m.GetAppInstallationID(),
			m.GetAppPrivateKeyFile(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub App installation transport: %w", err)
		}
		httpClient := &http.Client{Transport: itr}

		// Handle custom base URL
		if m.GetBaseURL() != "" {
			baseURL, err := url.Parse(m.GetBaseURL())
			if err != nil {
				return nil, fmt.Errorf("invalid base URL: %w", err)
			}
			graphqlURL := fmt.Sprintf("%s://%s/graphql", baseURL.Scheme, baseURL.Host)
			client = githubv4.NewEnterpriseClient(graphqlURL, httpClient)
		} else {
			client = githubv4.NewClient(httpClient)
		}

	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", m.GetAuthMethod())
	}

	return client, nil
}
