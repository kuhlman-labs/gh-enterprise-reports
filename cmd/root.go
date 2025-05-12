// Package cmd implements the command line interface for the GitHub Enterprise Reports application.
package cmd

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/config"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/report"
	"github.com/spf13/cobra"
)

var (
	configProvider *config.ManagerProvider
	logFile        *os.File
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-enterprise-reports",
	Short: "A CLI extension to generate GitHub Enterprise reports",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return configProvider.LoadConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Set log level from configuration.
		var level slog.Level
		if err := level.UnmarshalText([]byte(configProvider.GetLogLevel())); err != nil {
			slog.Warn("invalid log level specified, defaulting to info", "error", err)
			level = slog.LevelInfo
		}
		// reconfigure slog at chosen level
		setLogLevel(level)

		// Create REST and GraphQL clients with retry mechanism using the new interface
		restClient, err := configProvider.CreateRESTClient()
		if err != nil {
			slog.Error("creating rest client", "error", err)
			os.Exit(1)
		}
		// Create retryable REST client
		retryableREST := api.NewRetryableRESTClient(restClient, 3, 500*time.Millisecond)
		slog.Debug("created retryable REST client", "maxRetries", retryableREST.MaxRetries)

		graphQLClient, err := configProvider.CreateGraphQLClient()
		if err != nil {
			slog.Error("creating graphql client", "error", err)
			os.Exit(1)
		}
		// Create retryable GraphQL client
		retryableGraphQL := api.NewRetryableGraphQLClient(graphQLClient, 3, 500*time.Millisecond)
		slog.Debug("created retryable GraphQL client", "maxRetries", retryableGraphQL.MaxRetries)

		// Log the configuration details with a standout banner.
		slog.Info("==================================================")
		slog.Info("configuration values:",
			"auth_method", configProvider.GetAuthMethod(),
			"base_url", configProvider.GetBaseURL(),
			"enterprise", configProvider.GetEnterpriseSlug(),
			"output_format", configProvider.GetOutputFormat(),
			"output_dir", configProvider.GetOutputDir(),
			"profile", configProvider.GetProfile(),
		)
		slog.Info("==================================================")

		ctx := cmd.Context()

		// Ensure rate limits are sufficient before proceeding.
		api.EnsureRateLimits(ctx, restClient)

		// Start monitoring rate limits every 30 seconds asynchronously.
		go api.MonitorRateLimits(ctx, restClient.RateLimit, graphQLClient, 30*time.Second)

		// Create a new report executor using our configuration provider
		reportExecutor := report.NewReportExecutor(configProvider)

		// Execute the selected reports using the new interface-based approach
		reportExecutor.Execute(ctx, restClient, graphQLClient)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// Init initializes the command structure and configuration.
// It creates the configuration provider and adds all subcommands to the root command.
func Init() {
	// Create a new configuration manager
	configProvider = config.NewManagerProvider()

	// Initialize CLI flags using the config provider
	configProvider.InitializeFlags(rootCmd)

	// Add subcommands
	rootCmd.AddCommand(initCmd)
}
