package enterprisereports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

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

// getOrganization Repositories
func getOrganizationRepositories(ctx context.Context, restClient *github.Client, org string) ([]*github.Repository, error) {
	log.Info().Str("organization", org).Msg("Getting repositories")

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

		allRepos = append(allRepos, repos...)

		// Check rate limit
		if resp.Rate.Remaining < RESTRateLimitThreshold {
			log.Warn().Int("remaining", resp.Rate.Remaining).
				Int("limit", resp.Rate.Limit).
				Msg("Rate limit low, waiting until reset")
			waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
	}

	log.Info().Int("count", len(allRepos)).
		Str("organization", org).
		Msg("Found repositories")

	return allRepos, nil
}

// getTeams returns a list of teams for the given repository.
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

		allTeams = append(allTeams, teams...)

		// Check rate limit
		if resp.Rate.Remaining < RESTRateLimitThreshold {
			log.Warn().Int("remaining", resp.Rate.Remaining).
				Int("limit", resp.Rate.Limit).
				Msg("Rate limit low, waiting until reset")
			waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
	}
	return allTeams, nil
}

// getExternalGroups returns a list of external groups connected to the given team.
func getExternalGroups(ctx context.Context, restClient *github.Client, owner, teamSlug string) (*github.ExternalGroupList, error) {
	log.Info().Str("teamSlug", teamSlug).Msg("Getting external groups")

	externalGroups, resp, err := restClient.Teams.ListExternalGroupsForTeamBySlug(ctx, owner, teamSlug)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get external groups")
		return nil, fmt.Errorf("failed to get external groups for repository: %w", err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("remaining", resp.Rate.Remaining).
			Int("limit", resp.Rate.Limit).
			Msg("Rate limit low, waiting until reset")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	return externalGroups, nil
}

// getCustomProperties returns a list of custom properties for the given repository.
func getCustomProperties(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.CustomPropertyValue, error) {
	log.Info().Str("repository", repo).Msg("Getting custom properties")

	customProperties, resp, err := restClient.Repositories.GetAllCustomPropertyValues(ctx, owner, repo)
	if err != nil {
		log.Error().Err(err).Str("repository", repo).Msg("Failed to get custom properties")
		return nil, fmt.Errorf("failed to get custom properties for repository: %w", err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("remaining", resp.Rate.Remaining).
			Int("limit", resp.Rate.Limit).
			Msg("Rate limit low, waiting until reset")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	return customProperties, nil
}

// runRepositoryReport generates a report for the given repository.
// It has columns for the repository ID, owner, name, visibility, whether it is archived, pushed at, created at, topics, custom properties.
// It also has a column for the teams that have access to the repository.
// The teams is listed as a group of information including the team name, id, slug, external group, and permission level on the repo.
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
		"repository",
		"owner",
		"archived",
		"visibility",
		"pushed_at",
		"created_at",
		"topics",
		"custom_properties",
		"teams",
		"external_groups",
	}); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header")
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Get Enterprise Organizations
	organizations, err := getEnterpriseOrgs(ctx, graphQLClient, config)
	if err != nil {
		return err
	}

	for _, org := range organizations {
		// Get the organization's repositories.
		repos, err := getOrganizationRepositories(ctx, restClient, org.Login)
		if err != nil {
			log.Error().Err(err).Str("organization", org.Login).Msg("Failed to get repositories")
			continue
		}
		for _, repo := range repos {

			repoTeams := []RepoTeam{}

			// Get the repository's teams.
			teams, err := getTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
			if err != nil {
				log.Error().Err(err).
					Str("repository", repo.GetFullName()).
					Msg("Failed to get teams")
				continue
			}
			for _, team := range teams {
				// Get the Teams's external groups.
				externalGroups, err := getExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), team.GetSlug())
				if err != nil {
					log.Error().Err(err).Msgf("Failed to get external groups for repository %s", repo.GetFullName())
					continue
				}

				groupNames := []string{}

				for _, externalGroup := range externalGroups.Groups {
					groupNames = append(groupNames, *externalGroup.GroupName)
				}

				repoTeam := RepoTeam{
					TeamID:        team.GetID(),
					TeamName:      team.GetName(),
					TeamSlug:      team.GetSlug(),
					ExternalGroup: groupNames,
					Permission:    team.GetPermission(),
				}
				repoTeams = append(repoTeams, repoTeam)
			}

			var teamsFormatted []string
			for _, t := range repoTeams {
				externalGroups := ""
				if len(t.ExternalGroup) > 0 {
					externalGroups = fmt.Sprintf("%s", t.ExternalGroup)
				}
				teamsFormatted = append(teamsFormatted, fmt.Sprintf("(Team Name: %s, TeamID: %d, Team Slug: %s, External Group: %s, Permission: %s)",
					t.TeamName, t.TeamID, t.TeamSlug, externalGroups, t.Permission))
			}
			teamsStr := ""
			if len(teamsFormatted) > 0 {
				teamsStr = fmt.Sprintf("%s", teamsFormatted)
			}

			// Get the repository's custom properties.
			customProperties, err := getCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
			if err != nil {
				log.Error().Err(err).Str("repository", repo.GetFullName()).
					Msg("Failed to get custom properties")
				continue
			}

			var propsFormatted []string
			for _, property := range customProperties {
				value := ""
				if property.Value != nil {
					value = fmt.Sprintf("%v", property.Value)
				}
				// Assumes property.PropertyName is a string pointer.
				propName := ""
				if property.PropertyName != "" {
					propName = property.PropertyName
				}
				propsFormatted = append(propsFormatted, fmt.Sprintf("{%s: %s}", propName, value))
			}
			propsStr := "(" + strings.Join(propsFormatted, ",") + ")"

			// Write the repository report to the CSV file.
			if err := w.Write([]string{
				repo.GetFullName(),
				repo.GetOwner().GetLogin(),
				fmt.Sprintf("%t", repo.GetArchived()),
				repo.GetVisibility(),
				repo.GetPushedAt().String(),
				repo.GetCreatedAt().String(),
				fmt.Sprintf("%v", repo.Topics),
				propsStr,
				teamsStr,
			}); err != nil {
				log.Error().Err(err).Str("repository", repo.GetFullName()).
					Msg("Failed to write repository report to CSV")
				return fmt.Errorf("failed to write repository report to CSV: %w", err)
			}
		}
	}

	return nil
}
