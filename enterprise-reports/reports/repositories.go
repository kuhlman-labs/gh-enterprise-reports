// Package reports implements various report generation functionalities for GitHub Enterprise.
// It provides utilities and specific report types for organizations, repositories, teams,
// collaborators, and user data, with results exported as CSV files.
package reports

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// RepoReport contains repository information along with additional metadata
// about teams and custom properties associated with the repository.
type RepoReport struct {
	*github.Repository
	Teams            []*repoTeam                   // Teams with access to the repository
	CustomProperties []*github.CustomPropertyValue // Custom properties set on the repository
}

// repoTeam represents a team with access to a repository,
// including any external identity provider groups associated with the team.
type repoTeam struct {
	*github.Team
	ExternalGroups *github.ExternalGroupList // SAML/SCIM groups linked to this team
}

// RepositoryReport generates a CSV report for all repositories across all organizations in an enterprise.
// It fetches repository details, teams with access, external groups, and custom properties.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - restClient: GitHub REST API client
//   - graphQLClient: GitHub GraphQL API client
//   - enterpriseSlug: Enterprise identifier
//   - filename: Output CSV file path
//   - workerCount: Number of concurrent workers for processing repositories
//   - cache: Shared cache for storing and retrieving GitHub data
//
// The report includes repository owner organization, name, archive status, visibility,
// timestamps, topics, custom properties, and associated teams with their external groups.
func RepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string, workerCount int, cache *utils.SharedCache) error {
	slog.Info("starting repository report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename), slog.Int("workers", workerCount))

	// Create appropriate report writer based on file extension
	reportWriter, reportErr := NewReportWriter(filename)
	if reportErr != nil {
		return reportErr
	}
	defer func() {
		if err := reportWriter.Close(); err != nil {
			slog.Error("Failed to close report writer", "error", err)
		}
	}()

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

	// Write header to report
	if headerErr := reportWriter.WriteHeader(header); headerErr != nil {
		return fmt.Errorf("failed to write header: %w", headerErr)
	}
	// Check cache for organizations or fetch from API
	var orgs []*github.Organization
	var err error

	if cachedOrgs, found := cache.GetEnterpriseOrgs(); found {
		slog.Info("using cached enterprise organizations")
		orgs = cachedOrgs
	} else {
		// Fetch all organizations
		slog.Info("fetching enterprise organizations", slog.String("enterprise", enterpriseSlug))
		orgs, err = api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
		if err != nil {
			return fmt.Errorf("failed to fetch organizations: %w", err)
		}
		// Store in cache
		cache.SetEnterpriseOrgs(orgs)
	}

	// Collect all repositories
	var reposList []*github.Repository
	for _, org := range orgs {
		// Check cache for organization repositories
		var repos []*github.Repository
		if cachedRepos, found := cache.GetOrgRepositories(org.GetLogin()); found {
			slog.Info("using cached repositories for org", "org", org.GetLogin())
			repos = cachedRepos
		} else {
			slog.Info("fetching repositories for org", "org", org.GetLogin())
			repos, err = api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
			if err != nil {
				slog.Warn("failed to fetch repositories for org", "org", org.GetLogin(), "err", err)
				continue
			}
			// Store in cache
			cache.SetOrgRepositories(org.GetLogin(), repos)
		}
		reposList = append(reposList, repos...)
	}
	// Processor: enrich repository with teams and custom properties
	processor := func(ctx context.Context, repo *github.Repository) (*RepoReport, error) {
		slog.Info("processing repository", "repo", repo.GetFullName())

		// Check cache for repository teams
		var teams []*github.Team
		if cachedTeams, found := cache.GetRepoTeams(repo.GetFullName()); found {
			slog.Info("using cached teams for repo", "repo", repo.GetFullName())
			teams = cachedTeams
		} else {
			// Fetch teams and external groups
			teams, err = api.FetchTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
			if err != nil {
				slog.Debug("failed to fetch teams", "repo", repo.GetFullName(), "err", err)
				teams = []*github.Team{} // Initialize to empty slice on error
			}
			// Store in cache
			cache.SetRepoTeams(repo.GetFullName(), teams)
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

	// Run the report using the new report writer interface
	return RunReportWithWriter(ctx, reposList, processor, formatter, limiter, workerCount, reportWriter)
}
