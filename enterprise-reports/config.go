package enterprisereports

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"log/slog"

	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/reports"
)

// Config encapsulates all configuration from CLI flags and Viper.
type Config struct {
	Organizations           bool   `mapstructure:"organizations"`
	Repositories            bool   `mapstructure:"repositories"`
	Teams                   bool   `mapstructure:"teams"`
	Collaborators           bool   `mapstructure:"collaborators"`
	Users                   bool   `mapstructure:"users"`
	Workers                 int    `mapstructure:"workers"` // Workers for all reports
	AuthMethod              string `mapstructure:"auth-method"`
	Token                   string `mapstructure:"token"`
	GithubAppID             int64  `mapstructure:"app-id"`
	GithubAppPrivateKey     string `mapstructure:"app-private-key-file"`
	GithubAppInstallationID int64  `mapstructure:"app-installation-id"`
	EnterpriseSlug          string `mapstructure:"enterprise"`
	LogLevel                string `mapstructure:"log-level"`
	BaseURL                 string `mapstructure:"base-url"`
}

// Validate checks for required flags based on the chosen authentication method.
func (c *Config) Validate() error {
	var errs []error

	// default empty log‐level to "info"
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	c.LogLevel = strings.ToLower(c.LogLevel)

	// enterprise slug
	if c.EnterpriseSlug == "" {
		errs = append(errs, errors.New("enterprise flag is required"))
	}

	// normalize & validate auth method
	c.AuthMethod = strings.ToLower(c.AuthMethod)
	switch c.AuthMethod {
	case "token":
		if c.Token == "" {
			errs = append(errs, errors.New("token is required when using token authentication"))
		}
	case "app":
		// GitHub App authentication requires app-id, app-private-key, and app-installation-id
		if c.GithubAppID == 0 && c.GithubAppPrivateKey == "" && c.GithubAppInstallationID == 0 {
			errs = append(errs, errors.New("app-id, app-private-key, and app-installation-id are required when using GitHub App authentication"))
		} else {
			if c.GithubAppID == 0 {
				errs = append(errs, errors.New("app-id is required when using GitHub App authentication"))
			}
			if c.GithubAppPrivateKey == "" {
				errs = append(errs, errors.New("app-private-key-file is required when using GitHub App authentication"))
			} else if _, err := os.Stat(c.GithubAppPrivateKey); err != nil {
				errs = append(errs, fmt.Errorf("app-private-key-file %q does not exist", c.GithubAppPrivateKey))
			}
			if c.GithubAppInstallationID == 0 {
				errs = append(errs, errors.New("app-installation-id is required when using GitHub App authentication"))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("unknown auth-method %q: please use 'token' or 'app'", c.AuthMethod))
	}

	// at least one report
	if !c.Organizations && !c.Repositories && !c.Teams && !c.Collaborators && !c.Users {
		errs = append(errs, errors.New("no report selected: please specify at least one of: organizations, repositories, teams, collaborators, users"))
	}

	// BaseURL validation & trimming
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("base-url %q is not a valid URL", c.BaseURL))
		} else {
			c.BaseURL = strings.TrimSuffix(c.BaseURL, "/")
		}
	}

	// log-level validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "fatal": true, "panic": true}
	if !validLevels[c.LogLevel] {
		errs = append(errs, fmt.Errorf("log-level %q is not one of: debug, info, warn, error, fatal, panic", c.LogLevel))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// InitializeFlags configures the CLI flags and binds them to Viper.
func InitializeFlags(rootCmd *cobra.Command, config *Config) {
	// Set defaults in Viper
	viper.SetDefault("organizations", false)
	viper.SetDefault("repositories", false)
	viper.SetDefault("teams", false)
	viper.SetDefault("collaborators", false)
	viper.SetDefault("users", false)
	// Worker count default
	viper.SetDefault("workers", 5)

	viper.SetDefault("log-level", "info")
	viper.SetDefault("base-url", "https://api.github.com")
	viper.SetDefault("auth-method", "token")
	viper.SetDefault("enterprise", "")
	viper.SetDefault("token", "")
	viper.SetDefault("app-id", 0)
	viper.SetDefault("app-private-key-file", "")
	viper.SetDefault("app-installation-id", 0)

	// Report flags.
	rootCmd.Flags().BoolVar(&config.Organizations, "organizations", false, "Run Organizations report")
	rootCmd.Flags().BoolVar(&config.Repositories, "repositories", false, "Run Repositories report")
	rootCmd.Flags().BoolVar(&config.Teams, "teams", false, "Run Teams report")
	rootCmd.Flags().BoolVar(&config.Collaborators, "collaborators", false, "Run Collaborators report")
	rootCmd.Flags().BoolVar(&config.Users, "users", false, "Run Users report")
	// Worker count flag
	rootCmd.Flags().IntVar(&config.Workers, "workers", config.Workers, "Number of concurrent workers for fetching data (default 5)")

	//Log-level flag.
	rootCmd.Flags().StringVar(&config.LogLevel, "log-level", "info", "Set log level (debug, info, warn, error, fatal, panic)")

	// Base URL flag.
	rootCmd.Flags().StringVar(&config.BaseURL, "base-url", "https://api.github.com", "Base URL for GitHub API (optional)")

	// Authentication flags.
	rootCmd.Flags().StringVar(&config.AuthMethod, "auth-method", "token", "Authentication method: token or app")
	rootCmd.Flags().StringVar(&config.EnterpriseSlug, "enterprise", "", "Enterprise slug (required)")
	rootCmd.Flags().StringVar(&config.Token, "token", "", "Authentication token (required if auth-method is token)")
	rootCmd.Flags().Int64Var(&config.GithubAppID, "app-id", 0, "GitHub App ID (required if auth-method is app)")
	rootCmd.Flags().StringVar(&config.GithubAppPrivateKey, "app-private-key-file", "", "GitHub App private key file path (required if auth-method is app)")
	rootCmd.Flags().Int64Var(&config.GithubAppInstallationID, "app-installation-id", 0, "GitHub App installation ID (required if auth-method is app)")

	// Bind flags to Viper
	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		panic(fmt.Errorf("failed to bind flags: %w", err))
	}

	// Custom handler for unknown flags: list all valid flags
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		var flags []string
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			flags = append(flags, "--"+f.Name)
		})
		return fmt.Errorf("%s; valid flags are: %s", err.Error(), strings.Join(flags, ", "))
	})

	// Optionally read in config file and environment variables:
	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath(".")      // look for config in the working directory
	viper.AutomaticEnv()          // read in environment variables that match

	// Read & unmarshal config file
	if err := viper.ReadInConfig(); err != nil {
		slog.Warn("failed to read config file", slog.Any("err", err))
	} else {
		slog.Info("using config file", slog.String("configFile", viper.ConfigFileUsed()))
		if err := viper.UnmarshalExact(config); err != nil {
			slog.Error("failed to unmarshal config file", slog.Any("err", err))
		}
		// validate immediately
		if err := config.Validate(); err != nil {
			slog.Error("configuration validation failed", slog.Any("err", err))
			os.Exit(1)
		}
	}
}

// generateReportFilename centralizes timestamp+slug → filename logic.
func generateReportFilename(enterprise, reportName string) string {
	timestamp := time.Now().Format("20060102150405")
	return fmt.Sprintf("%s_%s_report_%s.csv", enterprise, reportName, timestamp)
}

// RunReports executes the selected report logic.
func RunReports(ctx context.Context, conf *Config, restClient *github.Client, graphQLClient *githubv4.Client) {
	runReport := func(reportName string, reportFunc func()) {
		start := time.Now()
		slog.Info("running report", slog.String("report", reportName))

		reportFunc()

		duration := time.Since(start).Round(time.Second)
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60
		slog.Info("========================================")
		slog.Info("report completed",
			slog.String("report", reportName),
			slog.String("duration", fmt.Sprintf("%d minutes %d seconds", minutes, seconds)),
		)
		slog.Info("========================================")
	}

	if conf.Organizations {
		runReport("organizations", func() {
			fileName := generateReportFilename(conf.EnterpriseSlug, "organizations")
			if err := reports.OrganizationsReport(ctx, graphQLClient, restClient, conf.EnterpriseSlug, fileName, conf.Workers); err != nil {
				slog.Error("failed to run organizations report", slog.Any("err", err))
			}
		})
	}
	if conf.Repositories {
		runReport("repositories", func() {
			fileName := generateReportFilename(conf.EnterpriseSlug, "repositories")
			if err := reports.RepositoryReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName, conf.Workers); err != nil {
				slog.Error("failed to run repositories report", slog.Any("err", err))
			}
		})
	}
	if conf.Teams {
		runReport("teams", func() {
			fileName := generateReportFilename(conf.EnterpriseSlug, "teams")
			if err := reports.TeamsReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName, conf.Workers); err != nil {
				slog.Error("failed to run teams report", slog.Any("err", err))
			}
		})
	}
	if conf.Collaborators {
		runReport("collaborators", func() {
			fileName := generateReportFilename(conf.EnterpriseSlug, "collaborators")
			if err := reports.CollaboratorsReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName, conf.Workers); err != nil {
				slog.Error("failed to run collaborators report", slog.Any("err", err))
			}
		})
	}
	if conf.Users {
		runReport("users", func() {
			fileName := generateReportFilename(conf.EnterpriseSlug, "users")
			if err := reports.UsersReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName, conf.Workers); err != nil {
				slog.Error("failed to run users report", slog.Any("err", err))
			}
		})
	}
}
