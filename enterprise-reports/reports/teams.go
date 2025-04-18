package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

// TeamsReport generates a CSV report of teams for the specified Enterprise.
// It includes columns for Team ID, Organization, Team Name, Team Slug, External Group, and Members.
func TeamsReport(ctx context.Context, restClient *github.Client, graphqlClient *githubv4.Client, enterpriseSlug, fileName string) error {
	slog.Info("Generating teams report", "enterprise", enterpriseSlug)

	// Create a CSV file to write the report
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fileName, err)
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
		return fmt.Errorf("failed to write header to file %s: %w", fileName, err)
	}

	// Get all organizations in the enterprise
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphqlClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to get organizations for enterprise %s: %w", enterpriseSlug, err)
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
			teams, err := api.FetchTeamsForOrganizations(ctx, restClient, orgLogin)
			if err != nil {
				slog.Warn("Skipping organization due to error fetching teams", "error", err, "organization", orgLogin)
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

				members, err := api.FetchTeamMembers(ctx, restClient, team, orgLogin)
				if err != nil {
					slog.Warn("Skipping team due to error fetching members", "error", err, "team", team.GetSlug())
					members = nil
				}

				externalGroups, err := api.FetchExternalGroups(ctx, restClient, orgLogin, team.GetSlug())
				if err != nil {
					slog.Warn("Skipping external groups due to error", "error", err, "team", team.GetSlug())
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
			slog.Warn("Failed to write team to file", "error", err, "team", result.team.GetSlug())
		}
	}

	return nil
}
