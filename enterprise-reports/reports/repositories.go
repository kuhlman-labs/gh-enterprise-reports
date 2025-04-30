package reports

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

type RepoReport struct {
	*github.Repository
	Teams            []*repoTeam
	CustomProperties []*github.CustomPropertyValue
}

type repoTeam struct {
	*github.Team
	ExternalGroups *github.ExternalGroupList
}

// runRepositoryReport generates a CSV report for repositories, including repository details, teams, and custom properties.
func RepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string, workerCount int) error {
	slog.Info("starting repository report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename), slog.Int("workers", workerCount))
	// Validate output path early to catch file creation errors before API calls
	if err := validateFilePath(filename); err != nil {
		return err
	}
	header := []string{
		"Owner",
		"Repository",
		"Archived",
		"Visibility",
		"Pushed_At",
		"Created_At",
		"Topics",
		"Custom_Properties",
		"Teams",
	}
	// Fetch all organizations
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}
	// Collect all repositories
	var reposList []*github.Repository
	for _, org := range orgs {
		repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
		if err != nil {
			slog.Warn("failed to fetch repositories for org", "org", org.GetLogin(), "err", err)
			continue
		}
		reposList = append(reposList, repos...)
	}
	// Processor: enrich repository with teams and custom properties
	processor := func(ctx context.Context, repo *github.Repository) (*RepoReport, error) {
		slog.Info("processing repository", "repo", repo.GetFullName())
		// Fetch teams and external groups
		teams, err := api.FetchTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
		if err != nil {
			slog.Debug("failed to fetch teams", "repo", repo.GetFullName(), "err", err)
		}
		var repoTeams []*repoTeam
		for _, t := range teams {
			eg, err := api.FetchExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), t.GetSlug())
			if err != nil {
				slog.Debug("failed to fetch external groups", "team", t.GetSlug(), "err", err)
			}
			if eg == nil {
				eg = &github.ExternalGroupList{}
			}
			repoTeams = append(repoTeams, &repoTeam{Team: t, ExternalGroups: eg})
		}
		// Fetch custom properties
		props, err := api.FetchCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
		if err != nil {
			slog.Debug("failed to fetch custom properties", "repo", repo.GetFullName(), "err", err)
		}
		if props == nil {
			props = []*github.CustomPropertyValue{}
		}
		return &RepoReport{Repository: repo, Teams: repoTeams, CustomProperties: props}, nil
	}
	// Formatter: format RepoReport into CSV row
	formatter := func(r *RepoReport) []string {
		propStrs := make([]string, len(r.CustomProperties))
		for i, cp := range r.CustomProperties {
			propStrs[i] = fmt.Sprintf("%s=%v", cp.PropertyName, cp.Value)
		}
		var teams []string
		for _, t := range r.Teams {
			name := t.GetSlug()
			if t.ExternalGroups != nil {
				for _, g := range t.ExternalGroups.Groups {
					name += fmt.Sprintf(" (%s)", g.GetGroupName())
				}
			}
			teams = append(teams, name)
		}
		return []string{
			r.GetOwner().GetLogin(),
			r.GetName(),
			fmt.Sprintf("%t", r.GetArchived()),
			r.GetVisibility(),
			r.GetPushedAt().String(),
			r.GetCreatedAt().String(),
			fmt.Sprintf("%v", r.Topics),
			strings.Join(propStrs, ","),
			strings.Join(teams, ","),
		}
	}

	// Create a limiter for rate limiting - aiming for ~2-3 repos/sec due to variable cost per repo
	// (Fetching external groups for each team adds points).
	// Cost = (2 + N_teams) REST points/repo. 5 repos/sec could exceed 15 points/sec limit if N_teams > 1.
	// Burst matches worker count for responsiveness.
	limiter := rate.NewLimiter(rate.Limit(2), workerCount) // e.g., 2 requests/sec, burst of workerCount

	// Run the report
	return RunReport(ctx, reposList, processor, formatter, limiter, workerCount, filename, header)
}
