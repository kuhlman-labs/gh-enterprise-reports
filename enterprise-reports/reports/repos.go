package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

type RepoTeam struct {
	TeamID        int64
	TeamName      string
	TeamSlug      string
	ExternalGroup []string
	Permission    string
}

// runRepositoryReport generates a CSV report for repositories, including repository details, teams, and custom properties.
func RepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting repository report", "filename", filename)

	// Create and open the CSV file
	f, err := os.Create(filename)
	if err != nil {
		slog.Error("failed to create report file", "filename", filename, "error", err)
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer f.Close()

	// Create a new CSV writer
	w := csv.NewWriter(f)
	defer w.Flush()

	// Write the CSV header
	if err := w.Write([]string{
		"owner",
		"repository",
		"archived",
		"visibility",
		"pushed_at",
		"created_at",
		"topics",
		"custom_properties",
		"teams",
	}); err != nil {
		slog.Error("failed to write CSV header", "error", err)
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Create a channel for CSV rows
	rowChan := make(chan []string)
	// Create a concurrency limiter for up to 5 concurrent requests
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	// Get Enterprise Organizations
	organizations, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return err
	}

	// Process each organization
	for _, org := range organizations {
		orgCopy := org // capture local copy
		wg.Add(1)      // add wait group for each organization
		go func(org *github.Organization) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore
			slog.Debug("processing organization", "organization", org.GetLogin())
			repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
			if err != nil {
				slog.Warn("failed to get repositories", "organization", org.GetLogin(), "error", err)
				return
			}
			for _, repo := range repos {
				wg.Add(1) // add wait group for each repository
				go func(repo *github.Repository, orgLogin string) {
					defer wg.Done()
					sem <- struct{}{}        // acquire semaphore
					defer func() { <-sem }() // release semaphore

					slog.Debug("processing repository", "repository", repo.GetFullName())
					// Process teams for the repository
					repoTeams := []RepoTeam{}
					var teamsMu sync.Mutex
					var teamWg sync.WaitGroup // local wait group for teams

					teams, err := api.FetchTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						slog.Warn("failed to get teams", "repository", repo.GetFullName(), "error", err)
						return
					}
					for _, team := range teams {
						teamWg.Add(1)
						go func(t *github.Team) {
							defer teamWg.Done()
							externalGroups, err := api.FetchExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), t.GetSlug())
							if err != nil {
								slog.Warn("failed to get external groups", "repository", repo.GetFullName(), "error", err)
								return
							}
							groupNames := []string{}
							for _, eg := range externalGroups.Groups {
								groupNames = append(groupNames, *eg.GroupName)
							}
							repoTeam := RepoTeam{
								TeamID:        t.GetID(),
								TeamName:      t.GetName(),
								TeamSlug:      t.GetSlug(),
								ExternalGroup: groupNames,
								Permission:    t.GetPermission(),
							}
							teamsMu.Lock()
							repoTeams = append(repoTeams, repoTeam)
							teamsMu.Unlock()
						}(team)
					}
					teamWg.Wait() // wait for all team goroutines to complete

					var teamsFormatted []string
					for _, t := range repoTeams {
						externalGroupsStr := ""
						if len(t.ExternalGroup) > 0 {
							externalGroupsStr = fmt.Sprintf("%s", t.ExternalGroup)
						} else {
							externalGroupsStr = "N/A"
						}
						teamsFormatted = append(teamsFormatted, fmt.Sprintf("(Team Name: %s, TeamID: %d, Team Slug: %s, External Group: %s, Permission: %s)",
							t.TeamName, t.TeamID, t.TeamSlug, externalGroupsStr, t.Permission))
					}
					teamsStr := "N/A"
					if len(teamsFormatted) > 0 {
						teamsStr = fmt.Sprintf("%s", teamsFormatted)
					}

					customProperties, err := api.FetchCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						slog.Warn("failed to get custom properties", "repository", repo.GetFullName(), "error", err)
						return
					}
					var propsFormatted []string
					for _, property := range customProperties {
						value := ""
						if property.Value != nil {
							value = fmt.Sprintf("%v", property.Value)
						}
						propName := ""
						if property.PropertyName != "" {
							propName = property.PropertyName
						}
						propsFormatted = append(propsFormatted, fmt.Sprintf("{%s: %s}", propName, value))
					}
					propsStr := "(" + strings.Join(propsFormatted, ",") + ")"
					rowChan <- []string{
						repo.GetOwner().GetLogin(),
						repo.GetName(),
						fmt.Sprintf("%t", repo.GetArchived()),
						repo.GetVisibility(),
						repo.GetPushedAt().String(),
						repo.GetCreatedAt().String(),
						fmt.Sprintf("%v", repo.Topics),
						propsStr,
						teamsStr,
					}
					slog.Debug("finished processing repository", "repository", repo.GetFullName())
				}(repo, org.GetLogin())
			}
		}(orgCopy) // end of organization goroutine
	}

	// Close rowChan after all goroutines complete
	go func() {
		wg.Wait()
		close(rowChan)
	}()

	// Write rows from rowChan to CSV
	for row := range rowChan {
		if err := w.Write(row); err != nil {
			slog.Error("failed to write repository report to CSV", "error", err)
			return fmt.Errorf("failed to write repository report to CSV: %w", err)
		}
	}

	slog.Debug("completed running repository report")
	return nil
}
