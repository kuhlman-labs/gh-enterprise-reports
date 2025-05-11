// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
package enterprisereports

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/reports"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
)

// RunReportsWithConfig executes reports based on the configuration manager settings.
// This is an enhanced version that supports multiple output formats and profiles.
func RunReportsWithConfig(ctx context.Context, configManager *ConfigManager, restClient *github.Client, graphQLClient *githubv4.Client) {
	config := configManager.Config
	startTime := time.Now()

	// Create a shared cache for improved performance
	cache := utils.NewSharedCache()

	// Number of concurrent workers for each report
	workers := config.Workers
	if workers < 1 {
		workers = 5 // Default to 5 workers
	}

	// Log the start of report generation
	slog.Info("starting report generation",
		"workers", workers,
		"outputFormat", config.OutputFormat,
		"outputDir", config.OutputDir)

	// Generate organizations report if requested
	if config.Organizations {
		filename := configManager.CreateOutputFileName("organizations")
		slog.Info("generating organizations report", "filename", filename)
		if err := reports.OrganizationsReport(ctx, graphQLClient, restClient, config.EnterpriseSlug, filename, workers, cache); err != nil {
			slog.Error("organizations report failed", "error", err)
		}
	}

	// Generate repositories report if requested
	if config.Repositories {
		filename := configManager.CreateOutputFileName("repositories")
		slog.Info("generating repositories report", "filename", filename)
		if err := reports.RepositoryReport(ctx, restClient, graphQLClient, config.EnterpriseSlug, filename, workers, cache); err != nil {
			slog.Error("repositories report failed", "error", err)
		}
	}

	// Generate teams report if requested
	if config.Teams {
		filename := configManager.CreateOutputFileName("teams")
		slog.Info("generating teams report", "filename", filename)
		if err := reports.TeamsReport(ctx, restClient, graphQLClient, config.EnterpriseSlug, filename, workers, cache); err != nil {
			slog.Error("teams report failed", "error", err)
		}
	}

	// Generate collaborators report if requested
	if config.Collaborators {
		filename := configManager.CreateOutputFileName("collaborators")
		slog.Info("generating collaborators report", "filename", filename)
		if err := reports.CollaboratorsReport(ctx, restClient, graphQLClient, config.EnterpriseSlug, filename, workers, cache); err != nil {
			slog.Error("collaborators report failed", "error", err)
		}
	}

	// Generate users report if requested
	if config.Users {
		filename := configManager.CreateOutputFileName("users")
		slog.Info("generating users report", "filename", filename)
		if err := reports.UsersReport(ctx, restClient, graphQLClient, config.EnterpriseSlug, filename, workers, cache); err != nil {
			slog.Error("users report failed", "error", err)
		}
	}

	// Report completion
	duration := time.Since(startTime).Round(time.Second)
	slog.Info("reports completed", "duration", duration)
}
