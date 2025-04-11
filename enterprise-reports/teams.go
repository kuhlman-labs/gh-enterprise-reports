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

// getTeamsForOrganizations retrieves all teams for the specified organizations.
func getTeamsForOrganizations(ctx context.Context, restClient *github.Client, org string) ([]*github.Team, error) {
	log.Info().Msg("Getting teams")

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    1,
	}
	allTeams := []*github.Team{}

	for {
		teams, resp, err := restClient.Teams.ListTeams(ctx, org, opts)
		if err != nil {
			log.Error().Err(err).Str("organization", org).Msg("Failed to get teams")
			return nil, fmt.Errorf("failed to get teams for organization: %w", err)
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

	log.Info().Int("count", len(allTeams)).Msg("Found teams")
	return allTeams, nil
}

// getTeamMembers retrieves all members for the specified team and organization.
func getTeamMembers(ctx context.Context, restClient *github.Client, team *github.Team, org string) ([]*github.User, error) {
	log.Info().Str("team", team.GetSlug()).Msg("Getting members")

	opts := &github.TeamListTeamMembersOptions{
		Role: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	allMembers := []*github.User{}

	for {
		members, resp, err := restClient.Teams.ListTeamMembersBySlug(ctx, org, team.GetSlug(), opts)
		if err != nil {
			log.Error().Err(err).Str("team", team.GetSlug()).Msg("Failed to get members")
			return nil, fmt.Errorf("failed to get members for team: %w", err)
		}
		log.Debug().Int("members_in_page", len(members)).Msg("Fetched a page of members")
		allMembers = append(allMembers, members...)

		// Check rate limit
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	log.Info().Int("count", len(allMembers)).Str("team", team.GetSlug()).Msg("Found members")
	return allMembers, nil
}

// runTeamsReport generates a CSV report of teams for the specified Enterprise.
// It includes columns for Team ID, Organization, Team Name, Team Slug, External Group, and Members.
func runTeamsReport(ctx context.Context, restClient *github.Client, graphqlClient *githubv4.Client, config *Config, fileName string) error {
	log.Info().Str("enterprise", config.EnterpriseSlug).Msg("Generating teams report")

	// Create a CSV file to write the report
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write the header row to the CSV file
	header := []string{
		"Team ID",
		"Organization",
		"Team Name",
		"Team Slug",
		"External Group",
		"Members",
	}
	err = writer.Write(header)
	if err != nil {
		return fmt.Errorf("failed to write header to file: %w", err)
	}

	// Get all organizations in the enterprise
	orgs, err := getEnterpriseOrgs(ctx, graphqlClient, config)
	if err != nil {
		return fmt.Errorf("failed to get organizations for enterprise: %w", err)
	}

	type teamResult struct {
		org   string
		teams []*github.Team
	}

	orgChan := make(chan teamResult, len(orgs))
	var wg sync.WaitGroup
	concurrencyLimit := 10 // limit to 10 calls
	semaphore := make(chan struct{}, concurrencyLimit)

	// Fetch teams concurrently for each organization
	for _, org := range orgs {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(orgLogin string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			teams, err := getTeamsForOrganizations(ctx, restClient, orgLogin)
			if err != nil {
				log.Warn().Err(err).Str("organization", orgLogin).Msg("Skipping organization due to error fetching teams")
				teams = nil
			}
			orgChan <- teamResult{org: orgLogin, teams: teams}
		}(org.GetLogin())
	}

	wg.Wait()
	close(orgChan)

	type memberGroupResult struct {
		team           *github.Team
		org            string
		members        []*github.User
		externalGroups *github.ExternalGroupList
	}

	var teamWg sync.WaitGroup
	teamChan := make(chan memberGroupResult, 1000) // buffer size to avoid blocking

	// Fetch members and external groups concurrently for each team
	for orgResult := range orgChan {
		for _, team := range orgResult.teams {
			teamWg.Add(1)
			semaphore <- struct{}{}
			go func(team *github.Team, orgLogin string) {
				defer teamWg.Done()
				defer func() { <-semaphore }()

				members, err := getTeamMembers(ctx, restClient, team, orgLogin)
				if err != nil {
					log.Warn().Err(err).Str("team", team.GetSlug()).Msg("Skipping team due to error fetching members")
					members = nil
				}

				externalGroups, err := getExternalGroups(ctx, restClient, orgLogin, team.GetSlug())
				if err != nil {
					log.Warn().Err(err).Str("team", team.GetSlug()).Msg("Skipping external groups due to error")
					externalGroups = nil
				}

				teamChan <- memberGroupResult{
					team:           team,
					org:            orgLogin,
					members:        members,
					externalGroups: externalGroups,
				}
			}(team, orgResult.org)
		}
	}

	go func() {
		teamWg.Wait()
		close(teamChan)
	}()

	// Write results to CSV
	for result := range teamChan {
		var membersStr []string
		if len(result.members) > 0 {
			for _, member := range result.members {
				membersStr = append(membersStr, member.GetLogin())
			}
		} else {
			membersStr = append(membersStr, "N/A")
		}

		var externalGroupsStr []string
		if result.externalGroups != nil && len(result.externalGroups.Groups) > 0 {
			for _, externalGroup := range result.externalGroups.Groups {
				externalGroupsStr = append(externalGroupsStr, externalGroup.GetGroupName())
			}
		} else {
			externalGroupsStr = append(externalGroupsStr, "N/A")
		}

		record := []string{
			fmt.Sprintf("%d", result.team.GetID()),
			result.org,
			result.team.GetName(),
			result.team.GetSlug(),
			strings.Join(externalGroupsStr, ","),
			strings.Join(membersStr, ","),
		}
		err = writer.Write(record)
		if err != nil {
			log.Error().Err(err).Str("team", result.team.GetSlug()).Msg("Failed to write team to file")
		}
	}

	return nil
}
