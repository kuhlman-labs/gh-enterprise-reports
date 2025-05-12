// Package report provides functionality for generating GitHub Enterprise reports.
package report

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/config"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/reports"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
)

// ReportRunner represents a report generation operation
type ReportRunner interface {
	// Run executes the report using the provided clients and configuration
	Run(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client,
		outputFilename string, workers int, cache *utils.SharedCache) error

	// Name returns the report's name for logging and identification
	Name() string
}

// OrganizationsReportRunner implements the ReportRunner interface for organizations report
type OrganizationsReportRunner struct {
	enterpriseSlug string
}

// NewOrganizationsReportRunner is a constructor function for creating organizations report runners
var NewOrganizationsReportRunner = func(enterpriseSlug string) ReportRunner {
	return &OrganizationsReportRunner{
		enterpriseSlug: enterpriseSlug,
	}
}

// Run executes the organizations report
func (r *OrganizationsReportRunner) Run(ctx context.Context, restClient *github.Client,
	graphQLClient *githubv4.Client, outputFilename string, workers int, cache *utils.SharedCache) error {

	return reports.OrganizationsReport(ctx, graphQLClient, restClient, r.enterpriseSlug, outputFilename, workers, cache)
}

// Name returns the report name
func (r *OrganizationsReportRunner) Name() string {
	return "organizations"
}

// RepositoriesReportRunner implements the ReportRunner interface for repositories report
type RepositoriesReportRunner struct {
	enterpriseSlug string
}

// NewRepositoriesReportRunner is a constructor function for creating repositories report runners
var NewRepositoriesReportRunner = func(enterpriseSlug string) ReportRunner {
	return &RepositoriesReportRunner{
		enterpriseSlug: enterpriseSlug,
	}
}

// Run executes the repositories report
func (r *RepositoriesReportRunner) Run(ctx context.Context, restClient *github.Client,
	graphQLClient *githubv4.Client, outputFilename string, workers int, cache *utils.SharedCache) error {

	return reports.RepositoryReport(ctx, restClient, graphQLClient, r.enterpriseSlug, outputFilename, workers, cache)
}

// Name returns the report name
func (r *RepositoriesReportRunner) Name() string {
	return "repositories"
}

// TeamsReportRunner implements the ReportRunner interface for teams report
type TeamsReportRunner struct {
	enterpriseSlug string
}

// NewTeamsReportRunner is a constructor function for creating teams report runners
var NewTeamsReportRunner = func(enterpriseSlug string) ReportRunner {
	return &TeamsReportRunner{
		enterpriseSlug: enterpriseSlug,
	}
}

// Run executes the teams report
func (r *TeamsReportRunner) Run(ctx context.Context, restClient *github.Client,
	graphQLClient *githubv4.Client, outputFilename string, workers int, cache *utils.SharedCache) error {

	return reports.TeamsReport(ctx, restClient, graphQLClient, r.enterpriseSlug, outputFilename, workers, cache)
}

// Name returns the report name
func (r *TeamsReportRunner) Name() string {
	return "teams"
}

// CollaboratorsReportRunner implements the ReportRunner interface for collaborators report
type CollaboratorsReportRunner struct {
	enterpriseSlug string
}

// NewCollaboratorsReportRunner is a constructor function for creating collaborators report runners
var NewCollaboratorsReportRunner = func(enterpriseSlug string) ReportRunner {
	return &CollaboratorsReportRunner{
		enterpriseSlug: enterpriseSlug,
	}
}

// Run executes the collaborators report
func (r *CollaboratorsReportRunner) Run(ctx context.Context, restClient *github.Client,
	graphQLClient *githubv4.Client, outputFilename string, workers int, cache *utils.SharedCache) error {

	return reports.CollaboratorsReport(ctx, restClient, graphQLClient, r.enterpriseSlug, outputFilename, workers, cache)
}

// Name returns the report name
func (r *CollaboratorsReportRunner) Name() string {
	return "collaborators"
}

// UsersReportRunner implements the ReportRunner interface for users report
type UsersReportRunner struct {
	enterpriseSlug string
}

// NewUsersReportRunner is a constructor function for creating users report runners
var NewUsersReportRunner = func(enterpriseSlug string) ReportRunner {
	return &UsersReportRunner{
		enterpriseSlug: enterpriseSlug,
	}
}

// Run executes the users report
func (r *UsersReportRunner) Run(ctx context.Context, restClient *github.Client,
	graphQLClient *githubv4.Client, outputFilename string, workers int, cache *utils.SharedCache) error {

	return reports.UsersReport(ctx, restClient, graphQLClient, r.enterpriseSlug, outputFilename, workers, cache)
}

// Name returns the report name
func (r *UsersReportRunner) Name() string {
	return "users"
}

// ReportExecutor coordinates the execution of multiple reports
type ReportExecutor struct {
	config config.Provider
	cache  *utils.SharedCache
}

// NewReportExecutor creates a new report executor
func NewReportExecutor(config config.Provider) *ReportExecutor {
	return &ReportExecutor{
		config: config,
		cache:  utils.NewSharedCache(),
	}
}

// Execute runs all the selected reports based on configuration
func (re *ReportExecutor) Execute(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client) {
	startTime := time.Now()
	workers := re.config.GetWorkers()
	if workers < 1 {
		workers = 5 // Default to 5 workers if not specified
	}

	// Log the start of report generation
	slog.Info("starting report generation",
		"workers", workers,
		"outputFormat", re.config.GetOutputFormat(),
		"outputDir", re.config.GetOutputDir())

	// Create report runners if they're enabled in config
	var runners []ReportRunner

	if re.config.ShouldRunOrganizationsReport() {
		runners = append(runners, NewOrganizationsReportRunner(re.config.GetEnterpriseSlug()))
	}

	if re.config.ShouldRunRepositoriesReport() {
		runners = append(runners, NewRepositoriesReportRunner(re.config.GetEnterpriseSlug()))
	}

	if re.config.ShouldRunTeamsReport() {
		runners = append(runners, NewTeamsReportRunner(re.config.GetEnterpriseSlug()))
	}

	if re.config.ShouldRunCollaboratorsReport() {
		runners = append(runners, NewCollaboratorsReportRunner(re.config.GetEnterpriseSlug()))
	}

	if re.config.ShouldRunUsersReport() {
		runners = append(runners, NewUsersReportRunner(re.config.GetEnterpriseSlug()))
	}

	// Execute each selected report
	for _, runner := range runners {
		re.executeReport(ctx, runner, restClient, graphQLClient, workers)
	}

	// Report completion
	duration := time.Since(startTime).Round(time.Second)
	slog.Info("reports completed", "duration", duration)
}

// executeReport runs a single report and logs its execution
func (re *ReportExecutor) executeReport(ctx context.Context, runner ReportRunner,
	restClient *github.Client, graphQLClient *githubv4.Client, workers int) {

	reportName := runner.Name()
	startTime := time.Now()

	slog.Info("generating report", "report", reportName)

	filename := re.config.CreateFilePath(reportName)
	err := runner.Run(ctx, restClient, graphQLClient, filename, workers, re.cache)

	if err != nil {
		slog.Error("report failed", "report", reportName, "error", err)
	} else {
		duration := time.Since(startTime).Round(time.Second)
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60

		slog.Info("========================================")
		slog.Info("report completed",
			"report", reportName,
			"duration", fmt.Sprintf("%d minutes %d seconds", minutes, seconds),
		)
		slog.Info("========================================")
	}
}
