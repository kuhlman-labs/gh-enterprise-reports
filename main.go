// Package main is the entry point for the GitHub Enterprise Reports application.
// It initializes logging, sets up configuration, handles signals, and executes reports
// based on the provided command-line flags.
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

	// Create a new configuration manager.
	configManager := enterprisereports.NewConfigManager()

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

	// Root command for running reports
	rootCmd := &cobra.Command{
		Use:   "gh-enterprise-reports",
		Short: "A CLI extension to generate GitHub Enterprise reports",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return configManager.LoadConfig()
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Get the config from the manager
			config := configManager.Config

			// Set log level from configuration.
			var level slog.Level
			if err := level.UnmarshalText([]byte(config.LogLevel)); err != nil {
				slog.Warn("invalid log level specified, defaulting to info", "error", err)
				level = slog.LevelInfo
			}
			// reconfigure slog to both outputs at chosen level
			enterprisereports.SetupLogging(logFile, level)

			// Create REST and GraphQL clients with retry mechanism
			restClient, err := enterprisereports.NewRESTClient(ctx, config)
			if err != nil {
				slog.Error("creating rest client", "error", err)
				os.Exit(1)
			}
			// Create retryable REST client
			retryableREST := api.NewRetryableRESTClient(restClient, 3, 500*time.Millisecond)
			slog.Debug("created retryable REST client", "maxRetries", retryableREST.MaxRetries)

			graphQLClient, err := enterprisereports.NewGraphQLClient(ctx, config)
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
				"auth_method", config.AuthMethod,
				"base_url", config.BaseURL,
				"enterprise", config.EnterpriseSlug,
				"output_format", config.OutputFormat,
				"output_dir", config.OutputDir,
				"profile", configManager.GetProfile(),
			)
			slog.Info("==================================================")

			// Ensure rate limits are sufficient before proceeding.
			api.EnsureRateLimits(ctx, restClient)

			// Start monitoring rate limits every 30 seconds asynchronously.
			go api.MonitorRateLimits(ctx, restClient.RateLimit, graphQLClient, 30*time.Second)

			// Run the reports with the new configManager
			enterprisereports.RunReportsWithConfig(ctx, configManager, restClient, graphQLClient)
		},
	}

	// Initialize config command
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new configuration file",
		Run: func(cmd *cobra.Command, args []string) {
			outputPath, _ := cmd.Flags().GetString("output")
			configFile := outputPath
			if configFile == "" {
				configFile = "config.yml"
			}

			if _, err := os.Stat(configFile); err == nil {
				slog.Error("configuration file already exists", "file", configFile)
				os.Exit(1)
			}

			templateData, err := os.ReadFile("config-template.yml")
			if err != nil {
				slog.Error("failed to read template file", "error", err)
				os.Exit(1)
			}

			if err := os.WriteFile(configFile, templateData, 0600); err != nil {
				slog.Error("failed to write configuration file", "error", err)
				os.Exit(1)
			}

			slog.Info("created new configuration file", "file", configFile)
			slog.Info("please edit the file to add your GitHub Enterprise details")
		},
	}

	// Add output flag to init command
	initCmd.Flags().StringP("output", "o", "", "Output path for the generated configuration file")

	rootCmd.AddCommand(initCmd)

	// Initialize CLI flags using the config manager
	configManager.InitializeFlags(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("executing command", "error", err)
		os.Exit(1)
	}
}
