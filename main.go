package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	enterprisereports "github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	// Open log file in append mode.
	file, err := os.OpenFile("gh-enterprise-reports.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open log file")
	}
	// Create a multiwriter to log to both terminal and file.
	writer := io.MultiWriter(os.Stderr, file)
	// Initialize zerolog with console writer using the multiwriter.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: writer, TimeFormat: time.UTC.String()})

	config := &enterprisereports.Config{}
	ctx := context.Background()

	// Define our root command with configuration validation.
	rootCmd := &cobra.Command{
		Use:   "gh-enterprise-reports",
		Short: "A CLI extension to generate GitHub Enterprise reports",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return config.Validate()
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Set global log level based on configuration.
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
			go enterprisereports.MonitorRateLimits(ctx, restClient, graphQLClient, 15*time.Second)

			// Measure the time taken to run reports.
			startTime := time.Now().UTC() // Ensure UTC
			enterprisereports.RunReports(ctx, config, restClient, graphQLClient)
			duration := time.Since(startTime).Round(time.Second)
			minutes := int(duration.Minutes())
			seconds := int(duration.Seconds()) % 60
			log.Info().Msg("========================================")
			log.Info().Str("Duration", fmt.Sprintf("%dm %ds", minutes, seconds)).Msg("Report completed")
			log.Info().Msg("========================================")
		},
	}

	// Initialize CLI flags and bind them with Viper inside the enterprise report package.
	enterprisereports.InitializeFlags(rootCmd, config)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Error executing command")
	}
}
