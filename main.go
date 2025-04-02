package main

import (
	"context"
	"os"
	"time"

	enterprisereports "github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	// Initialize zerolog with console writer for colored output.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

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
			enterprisereports.EnsureRateLimits(ctx, restClient, graphQLClient)

			// Start monitoring rate limits every minute asynchronously and log the results.
			go enterprisereports.MonitorRateLimits(ctx, restClient, graphQLClient, 1*time.Minute)

			// Measure the time taken to run reports.
			startTime := time.Now()
			enterprisereports.RunReports(ctx, config, restClient, graphQLClient)
			duration := time.Since(startTime)
			duration = duration.Round(time.Second)
			log.Info().Msg("========================================")
			log.Info().Dur("Duration", duration).Msg("Report completed")
			log.Info().Msg("========================================")
		},
	}

	// Initialize CLI flags and bind them with Viper inside the enterprise report package.
	enterprisereports.InitializeFlags(rootCmd, config)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Error executing command")
	}
}
