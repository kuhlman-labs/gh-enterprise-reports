package enterprisereports

import (
	"context"
	"fmt"
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

	if c.EnterpriseSlug == "" {
		return fmt.Errorf("enterprise flag is required")
	}

	switch c.AuthMethod {
	case "token":
		if c.Token == "" {
			return fmt.Errorf("token is required when using token authentication")
		}
	case "app":
		if c.GithubAppID == 0 || c.GithubAppPrivateKey == "" || c.GithubAppInstallationID == 0 {
			return fmt.Errorf("app-id, app-private-key, and app-installation-id are required when using GitHub App authentication")
		}
	default:
		return fmt.Errorf("unknown auth-method %q: please use 'token' or 'app'", c.AuthMethod)
	}

	// Ensure at least one report flag is provided.
	if !c.Organizations && !c.Repositories && !c.Teams && !c.Collaborators && !c.Users {
		valid := []string{"organizations", "repositories", "teams", "collaborators", "users"}
		return fmt.Errorf("no report selected: please specify at least one of: %s", strings.Join(valid, ", "))
	}

	return nil
}

// InitializeFlags configures the CLI flags and binds them to Viper.
func InitializeFlags(rootCmd *cobra.Command, config *Config) {
	// Report flags.
	rootCmd.Flags().BoolVar(&config.Organizations, "organizations", false, "Run Organizations report")
	rootCmd.Flags().BoolVar(&config.Repositories, "repositories", false, "Run Repositories report")
	rootCmd.Flags().BoolVar(&config.Teams, "teams", false, "Run Teams report")
	rootCmd.Flags().BoolVar(&config.Collaborators, "collaborators", false, "Run Collaborators report")
	rootCmd.Flags().BoolVar(&config.Users, "users", false, "Run Users report")

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
	viper.BindPFlags(rootCmd.Flags())

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

	// Validate the configuration file.
	if err := viper.ReadInConfig(); err != nil {
		slog.Warn("failed to read config file", slog.Any("err", err))
	} else {
		slog.Info("using config file", slog.String("configFile", viper.ConfigFileUsed()))
		// Read the config file and bind it to the config struct.
		if err := viper.UnmarshalExact(config); err != nil {
			slog.Error("failed to unmarshal config file", slog.Any("err", err))
		}
	}

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
			currentTime := time.Now()
			formattedTime := currentTime.Format("20060102150405")
			fileName := fmt.Sprintf("%s_organizations_report_%s.csv", conf.EnterpriseSlug, formattedTime)
			if err := reports.OrganizationsReport(ctx, graphQLClient, restClient, conf.EnterpriseSlug, fileName); err != nil {
				slog.Error("failed to run organizations report", slog.Any("err", err))
			}
		})
	}
	if conf.Repositories {
		runReport("repositories", func() {
			currentTime := time.Now()
			formattedTime := currentTime.Format("20060102150405")
			fileName := fmt.Sprintf("%s_repositories_report_%s.csv", conf.EnterpriseSlug, formattedTime)
			if err := reports.RepositoryReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName); err != nil {
				slog.Error("failed to run repositories report", slog.Any("err", err))
			}
		})
	}
	if conf.Teams {
		runReport("teams", func() {
			currentTime := time.Now()
			formattedTime := currentTime.Format("20060102150405")
			fileName := fmt.Sprintf("%s_teams_report_%s.csv", conf.EnterpriseSlug, formattedTime)
			if err := reports.TeamsReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName); err != nil {
				slog.Error("failed to run teams report", slog.Any("err", err))
			}
		})
	}
	if conf.Collaborators {
		runReport("collaborators", func() {
			currentTime := time.Now()
			formattedTime := currentTime.Format("20060102150405")
			fileName := fmt.Sprintf("%s_collaborators_report_%s.csv", conf.EnterpriseSlug, formattedTime)
			if err := reports.CollaboratorsReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName); err != nil {
				slog.Error("failed to run collaborators report", slog.Any("err", err))
			}
		})
	}
	if conf.Users {
		runReport("users", func() {
			currentTime := time.Now()
			formattedTime := currentTime.Format("20060102150405")
			fileName := fmt.Sprintf("%s_users_report_%s.csv", conf.EnterpriseSlug, formattedTime)
			if err := reports.UsersReport(ctx, restClient, graphQLClient, conf.EnterpriseSlug, fileName); err != nil {
				slog.Error("failed to run users report", slog.Any("err", err))
			}
		})
	}
}
