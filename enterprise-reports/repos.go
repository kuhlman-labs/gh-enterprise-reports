package enterprisereports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
)

type RepoTeam struct {
	TeamID        int64
	TeamName      string
	TeamSlug      string
	ExternalGroup []string
	Permission    string
}

// getOrganizationRepositories retrieves all repositories for the specified organization.
func getOrganizationRepositories(ctx context.Context, restClient *github.Client, org string) ([]*github.Repository, error) {
	log.Info().Str("organization", org).Msg("Getting repositories")
	log.Debug().Str("organization", org).Msg("Starting repositories retrieval")

	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}
	allRepos := []*github.Repository{}

	for {
		repos, resp, err := restClient.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			log.Error().Err(err).Str("organization", org).Msg("Failed to get repositories")
			return nil, fmt.Errorf("failed to get repositories for organization: %w", err)
		}
		log.Debug().Int("repos_in_page", len(repos)).Msg("Fetched a page of repositories")
		allRepos = append(allRepos, repos...)

		// Check rate limit
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	log.Info().Int("count", len(allRepos)).Str("organization", org).Msg("Found repositories")
	return allRepos, nil
}

// getTeams retrieves all teams for the specified repository.
func getTeams(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.Team, error) {
	log.Info().Str("repository", repo).Msg("Getting teams")

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    1,
	}
	allTeams := []*github.Team{}

	for {
		teams, resp, err := restClient.Repositories.ListTeams(ctx, owner, repo, opts)
		if err != nil {
			log.Error().Err(err).Str("repository", repo).Msg("Failed to get teams")
			return nil, fmt.Errorf("failed to get teams for repository: %w", err)
		}
		log.Debug().Int("teams_in_page", len(teams)).Msg("Fetched a page of teams")
		allTeams = append(allTeams, teams...)

		// Check rate limit
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allTeams, nil
}

// getExternalGroups retrieves external groups for the specified team.
func getExternalGroups(ctx context.Context, restClient *github.Client, owner, teamSlug string) (*github.ExternalGroupList, error) {
	log.Info().Str("teamSlug", teamSlug).Msg("Getting external groups")

	externalGroups, resp, err := restClient.Teams.ListExternalGroupsForTeamBySlug(ctx, owner, teamSlug)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get external groups")
		return nil, fmt.Errorf("failed to get external groups for repository: %w", err)
	}
	log.Debug().Int("external_groups_count", len(externalGroups.Groups)).Msg("Fetched external groups")

	// Check rate limit
	handleRESTRateLimit(ctx, resp.Rate)

	return externalGroups, nil
}

// getCustomProperties retrieves all custom properties for the specified repository.
func getCustomProperties(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.CustomPropertyValue, error) {
	log.Info().Str("repository", repo).Msg("Getting custom properties")

	customProperties, resp, err := restClient.Repositories.GetAllCustomPropertyValues(ctx, owner, repo)
	if err != nil {
		log.Error().Err(err).Str("repository", repo).Msg("Failed to get custom properties")
		return nil, fmt.Errorf("failed to get custom properties for repository: %w", err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, resp.Rate)

	return customProperties, nil
}

// runRepositoryReport generates a CSV report for repositories, including repository details, teams, and custom properties.
func runRepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, config *Config, filename string) error {
	log.Info().Str("filename", filename).Msg("Starting repository report")

	// Create and open the CSV file
	f, err := os.Create(filename)
	if err != nil {
		log.Error().Err(err).Str("filename", filename).Msg("Failed to create report file")
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
		log.Error().Err(err).Msg("Failed to write CSV header")
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Create a channel for CSV rows
	rowChan := make(chan []string)
	// Create a concurrency limiter for up to 5 concurrent requests
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	// Get Enterprise Organizations
	organizations, err := getEnterpriseOrgs(ctx, graphQLClient, config)
	if err != nil {
		return err
	}

	// Process each organization
	for _, org := range organizations {
		orgCopy := org // capture local copy
		wg.Add(1)      // add wait group for each organization
		go func(org Organization) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore
			log.Debug().Str("organization", org.Login).Msg("Processing organization")
			repos, err := getOrganizationRepositories(ctx, restClient, org.Login)
			if err != nil {
				log.Error().Err(err).Str("organization", org.Login).Msg("Failed to get repositories")
				return
			}
			for _, repo := range repos {
				wg.Add(1) // add wait group for each repository
				go func(repo *github.Repository, orgLogin string) {
					defer wg.Done()
					sem <- struct{}{}        // acquire semaphore
					defer func() { <-sem }() // release semaphore

					log.Debug().Str("repository", repo.GetFullName()).Msg("Processing repository")
					// Process teams for the repository
					repoTeams := []RepoTeam{}
					var teamsMu sync.Mutex
					var teamWg sync.WaitGroup // local wait group for teams

					teams, err := getTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						log.Error().Err(err).Str("repository", repo.GetFullName()).Msg("Failed to get teams")
						return
					}
					for _, team := range teams {
						teamWg.Add(1)
						go func(t *github.Team) {
							defer teamWg.Done()
							externalGroups, err := getExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), t.GetSlug())
							if err != nil {
								log.Error().Err(err).Msgf("Failed to get external groups for repository %s", repo.GetFullName())
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

					customProperties, err := getCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						log.Error().Err(err).Str("repository", repo.GetFullName()).Msg("Failed to get custom properties")
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
					log.Debug().Str("repository", repo.GetFullName()).Msg("Finished processing repository")
				}(repo, org.Login)
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
			log.Error().Err(err).Msg("Failed to write repository report to CSV")
			return fmt.Errorf("failed to write repository report to CSV: %w", err)
		}
	}

	log.Debug().Msg("Completed running repository report")
	return nil
}
