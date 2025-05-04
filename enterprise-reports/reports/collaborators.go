// Package reports implements various report generation functionalities for GitHub Enterprise.
// It provides utilities and specific report types for organizations, repositories, teams,
// collaborators, and user data, with results exported as CSV files.
package reports

import (
	"context"
	"encoding/json"
	"fmt"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// CollaboratorReport represents a repository with its associated collaborators
// for generating reports about repository access across an enterprise.
type CollaboratorReport struct {
	Repository    *github.Repository // The repository being analyzed
	Collaborators []CollaboratorInfo // List of collaborators with their permissions
}

// CollaboratorInfo contains simplified collaborator information for CSV output.
// Includes the essential user identification and permission level.
type CollaboratorInfo struct {
	Login      string `json:"login"`      // User's GitHub login name
	ID         int64  `json:"id"`         // User's numeric ID
	Permission string `json:"permission"` // User's highest permission level on the repository
}

// CollaboratorsReport generates a CSV report of repository collaborators across an enterprise.
// It fetches all repositories in all organizations within the enterprise, then collects
// direct collaborator information for each repository.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - restClient: GitHub REST API client
//   - graphClient: GitHub GraphQL API client
//   - enterpriseSlug: Enterprise identifier
//   - filename: Output CSV file path
//   - workerCount: Number of concurrent workers for processing repositories
//
// The report includes repository full name and JSON-encoded collaborator details
// with login, ID, and permission level for each collaborator.
func CollaboratorsReport(ctx context.Context, restClient *github.Client, graphClient *githubv4.Client, enterpriseSlug, filename string, workerCount int) error {
	slog.Info("starting collaborators report", "enterprise", enterpriseSlug, "filename", filename, "workers", workerCount)
	// Validate output path early to catch file creation errors before API calls
	if err := validateFilePath(filename); err != nil {
		return err
	}
	header := []string{"Repository", "Collaborators"}

	// Fetch all enterprise orgs
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch enterprise orgs: %w", err)
	}

	// Collect all repositories across orgs
	var repos []*github.Repository
	for _, org := range orgs {
		slog.Info("fetching repositories for org", "org", org.GetLogin())
		rs, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
		if err != nil {
			slog.Error("failed to fetch repositories for org", "org", org.GetLogin(), "error", err)
			continue
		}
		repos = append(repos, rs...)
	}

	// Processor: fetch collaborators for a repository
	processor := func(ctx context.Context, repo *github.Repository) (*CollaboratorReport, error) {
		cols, err := api.FetchRepoCollaborators(ctx, restClient, repo)
		if err != nil {
			// Log the error but return a report with empty collaborators instead of skipping.
			slog.Warn("failed to fetch collaborators, reporting repo with empty collaborators", slog.String("repo", repo.GetFullName()), "error", err)
			return &CollaboratorReport{Repository: repo, Collaborators: []CollaboratorInfo{}}, nil // Return non-nil report, nil error
		}
		var infos []CollaboratorInfo
		for _, c := range cols {
			infos = append(infos, CollaboratorInfo{c.GetLogin(), c.GetID(), getHighestPermission(c.GetPermissions())})
		}
		return &CollaboratorReport{Repository: repo, Collaborators: infos}, nil
	}

	// Formatter: format collaborators into CSV row
	formatter := func(r *CollaboratorReport) []string {
		row := []string{r.Repository.GetFullName()}
		if len(r.Collaborators) == 0 {
			row = append(row, "N/A") // Add "N/A" if no collaborators
		} else {
			for _, ci := range r.Collaborators {
				data, err := json.Marshal(ci)
				if err != nil {
					slog.Error("failed to marshal collaborator info", slog.String("repo", r.Repository.GetFullName()), slog.Any("ci", ci), "error", err)
					continue // Skip this collaborator on error
				}
				row = append(row, string(data))
			}
		}
		return row
	}

	// Create a limiter for rate limiting - aiming for ~10 requests/sec (below 15/sec limit)
	// with a burst matching the number of workers.
	limiter := rate.NewLimiter(rate.Limit(10), workerCount) // e.g., 10 requests/sec, burst of workerCount

	return RunReport(ctx, repos, processor, formatter, limiter, workerCount, filename, header)
}
