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

type CollaboratorReport struct {
	Repository    *github.Repository
	Collaborators []CollaboratorInfo
}

type CollaboratorInfo struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	Permission string `json:"permission"`
}

// CollaboratorsReport generates a CSV report of repository collaborators for the enterprise.
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
