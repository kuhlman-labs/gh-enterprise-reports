package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	enterprisereports "github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/spf13/cobra"
)

func main() {
	// Open log file in append mode.
	logFile, err := os.OpenFile("gh-enterprise-reports.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		slog.Error("failed to open log file", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			slog.Error("failed to close log file", "error", err)
		}
	}()

	// initialize slog to file+terminal at Info level
	enterprisereports.SetupLogging(logFile, slog.LevelInfo)

	// Create a new configuration object.
	config := &enterprisereports.Config{}

	// Add context cancellation for long-running operations.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown.
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		slog.Info("received shutdown signal, canceling context")
		cancel()
	}()

	// Define our root command with configuration validation.
	rootCmd := &cobra.Command{
		Use:   "gh-enterprise-reports",
		Short: "A CLI extension to generate GitHub Enterprise reports",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return config.Validate()
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Set log level from configuration.
			var level slog.Level
			if err := level.UnmarshalText([]byte(config.LogLevel)); err != nil {
				slog.Warn("invalid log level specified, defaulting to info", "error", err)
				level = slog.LevelInfo
			}
			// reconfigure slog to both outputs at chosen level
			enterprisereports.SetupLogging(logFile, level)

			// Create REST and GraphQL clients.
			restClient, err := enterprisereports.NewRESTClient(ctx, config)
			if err != nil {
				slog.Error("creating rest client", "error", err)
				os.Exit(1)
			}
			graphQLClient, err := enterprisereports.NewGraphQLClient(ctx, config)
			if err != nil {
				slog.Error("creating graphql client", "error", err)
				os.Exit(1)
			}

			// Log the configuration details with a standout banner.
			slog.Info("==================================================")
			slog.Info("configuration values:",
				"auth_method", config.AuthMethod,
				"base_url", config.BaseURL,
				"enterprise", config.EnterpriseSlug,
			)
			slog.Info("==================================================")

			// Ensure rate limits are sufficient before proceeding.
			api.EnsureRateLimits(ctx, restClient)

			// Start monitoring rate limits every 30 seconds asynchronously.
			go api.MonitorRateLimits(ctx, restClient.RateLimit, graphQLClient, 30*time.Second)

			// Run the reports.
			enterprisereports.RunReports(ctx, config, restClient, graphQLClient)
		},
	}

	// Initialize CLI flags and bind them with Viper inside the enterprise report package.
	enterprisereports.InitializeFlags(rootCmd, config)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("executing command", "error", err)
		os.Exit(1)
	}
}
