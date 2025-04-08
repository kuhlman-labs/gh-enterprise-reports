package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	enterprisereports "github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	// Open log file in append mode.
	logFile, err := os.OpenFile("gh-enterprise-reports.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open log file")
	}
	defer logFile.Close()

	// Create a ConsoleWriter with colored output for the terminal.
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	}

	// Setup logger with both file and console outputs.
	configuredLogger := zerolog.New(zerolog.MultiLevelWriter(logFile, consoleWriter)).With().Timestamp().Logger()
	log.Logger = configuredLogger

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
		log.Info().Msg("Received shutdown signal, canceling context...")
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
			// Set global log level for console output based on configuration.
			level, err := zerolog.ParseLevel(config.LogLevel)
			if err != nil {
				log.Warn().Err(err).Msg("Invalid log level specified, defaulting to info.")
				level = zerolog.InfoLevel
			}
			zerolog.SetGlobalLevel(level)

			// Create REST and GraphQL clients.
			restClient, err := enterprisereports.NewRESTClient(ctx, config)
			if err != nil {
				log.Fatal().Err(err).Msg("Error creating REST client")
			}
			graphQLClient, err := enterprisereports.NewGraphQLClient(ctx, config)
			if err != nil {
				log.Fatal().Err(err).Msg("Error creating GraphQL client")
			}

			// Log the configuration details.
			log.Info().Msg("========================================")
			log.Info().Str("Auth Method", config.AuthMethod).Msg("Configuration")
			log.Info().Str("Base URL", "https://api.github.com/").Msg("Configuration")
			log.Info().Str("Enterprise", config.EnterpriseSlug).Msg("Configuration")
			log.Info().Msg("========================================")

			// Ensure rate limits are sufficient before proceeding.
			enterprisereports.EnsureRateLimits(ctx, restClient)

			// Start monitoring rate limits every 15 seconds asynchronously and log the results.
			go enterprisereports.MonitorRateLimits(ctx, restClient, graphQLClient, 30*time.Second)

			// Run the reports.
			enterprisereports.RunReports(ctx, config, restClient, graphQLClient)
		},
	}

	// Initialize CLI flags and bind them with Viper inside the enterprise report package.
	enterprisereports.InitializeFlags(rootCmd, config)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Error executing command")
	}
}
