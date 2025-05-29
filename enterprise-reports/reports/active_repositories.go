// Package reports implements various report generation functionalities for GitHub Enterprise.
package reports

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// ActiveRepoReport contains repository information with recent commit activity
// and the list of contributors who have committed within the specified time range.
type ActiveRepoReport struct {
	*github.Repository
	RecentContributors []string // List of unique contributor names who committed in the last 90 days
}

// ActiveRepositoriesReport generates a CSV report for repositories with recent commit activity.
// It identifies repositories that have been committed to within the last 90 days and lists
// all contributors who have made commits during that period.
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
// The report includes repository owner, name, last pushed date, and a list of
// recent contributors who committed within the last 90 days.
func ActiveRepositoriesReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string, workerCount int, cache *utils.SharedCache) error {
	slog.Info("starting active repositories report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename), slog.Int("workers", workerCount))

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
		"Pushed_At",
		"Recent_Contributors",
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

	// Filter repositories that have been pushed to in the last 90 days
	cutoffDate := time.Now().AddDate(0, 0, -90)
	var activeRepos []*github.Repository

	for _, repo := range reposList {
		if repo.GetPushedAt().After(cutoffDate) {
			activeRepos = append(activeRepos, repo)
		}
	}

	slog.Info("filtered active repositories", "total_repos", len(reposList), "active_repos", len(activeRepos))

	// Processor: fetch recent commits and extract contributors
	processor := func(ctx context.Context, repo *github.Repository) (*ActiveRepoReport, error) {
		slog.Info("processing active repository", "repo", repo.GetFullName())

		// Fetch commits from the last 90 days
		commits, err := api.FetchRepositoryCommits(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName(), cutoffDate)
		if err != nil {
			slog.Warn("failed to fetch commits", "repo", repo.GetFullName(), "err", err)
			// Return repo with empty contributors list rather than failing
			return &ActiveRepoReport{Repository: repo, RecentContributors: []string{}}, nil
		}

		// Extract unique contributors from commits
		contributorMap := make(map[string]bool)
		for _, commit := range commits {
			if commit.GetCommit() != nil && commit.GetCommit().GetCommitter() != nil {
				committerName := commit.GetCommit().GetCommitter().GetName()
				if committerName != "" && committerName != "GitHub" && committerName != "github-merge-queue" {
					contributorMap[committerName] = true
				}
			}
			// Also check commit author in case it's different from committer
			if commit.GetCommit() != nil && commit.GetCommit().GetAuthor() != nil {
				authorName := commit.GetCommit().GetAuthor().GetName()
				if authorName != "" && authorName != "GitHub" && authorName != "github-merge-queue" {
					contributorMap[authorName] = true
				}
			}
		}

		// Convert map to sorted slice
		contributors := make([]string, 0, len(contributorMap))
		for name := range contributorMap {
			contributors = append(contributors, name)
		}
		sort.Strings(contributors)

		return &ActiveRepoReport{Repository: repo, RecentContributors: contributors}, nil
	}

	// Formatter: format ActiveRepoReport into CSV row
	formatter := func(r *ActiveRepoReport) []string {
		contributorsStr := strings.Join(r.RecentContributors, "; ")
		if contributorsStr == "" {
			contributorsStr = "N/A"
		}

		return []string{
			r.GetOwner().GetLogin(),
			r.GetName(),
			r.GetPushedAt().Format(time.RFC3339),
			contributorsStr,
		}
	}

	// Create a limiter for rate limiting - more conservative due to commit fetching
	// Commit fetching can be expensive, so we limit to 1 request per second
	limiter := rate.NewLimiter(rate.Limit(1), workerCount)

	// Run the report using the new report writer interface
	return RunReportWithWriter(ctx, activeRepos, processor, formatter, limiter, workerCount, reportWriter)
}
