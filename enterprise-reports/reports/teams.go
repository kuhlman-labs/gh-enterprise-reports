package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

// TeamsReport generates a CSV report of teams for the specified Enterprise.
// It includes columns for Team ID, Organization, Team Name, Team Slug, External Group, and Members.
func TeamsReport(ctx context.Context, restClient *github.Client, graphqlClient *githubv4.Client, enterpriseSlug, fileName string) error {
	slog.Info("starting teams report", slog.String("enterprise", enterpriseSlug), slog.String("file", fileName))

	// Create CSV file to write the report
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fileName, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write the CSV header
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

	teamChan := make(chan []*github.Team, len(orgs))
	var teamWg sync.WaitGroup
	var teamsCount int64
	concurrencyLimit := 10 // limit to 10 calls
	semaphore := make(chan struct{}, concurrencyLimit)

	// Enqueue team fetching for each organization
	for _, org := range orgs {
		teamWg.Add(1)
		semaphore <- struct{}{}
		go func(orgLogin string) {
			defer teamWg.Done()
			defer func() { <-semaphore }()
			teams, err := api.FetchTeamsForOrganizations(ctx, restClient, orgLogin)
			if err != nil {
				slog.Warn("Skipping organization due to error fetching teams", "error", err, "organization", orgLogin)
				teams = nil
			}
			teamChan <- teams
		}(org.GetLogin())
	}

	teamWg.Wait()
	close(teamChan)

	type memberGroupResult struct {
		team           *github.Team
		org            string
		members        []*github.User
		externalGroups *github.ExternalGroupList
	}

	var resultWg sync.WaitGroup
	resultsChan := make(chan memberGroupResult, 1000) // buffer size to avoid blocking

	// Fetch members and external groups concurrently for each team
MembersLoop:
	for {
		select {
		case <-ctx.Done():
			slog.Warn("Context cancelled fetching teams")
			return ctx.Err()
		case teams, ok := <-teamChan:
			if !ok {
				slog.Debug("Finished fetching teams")
				break MembersLoop
			}
			for _, team := range teams {
				resultWg.Add(1)
				semaphore <- struct{}{}
				go func(team *github.Team) {
					defer resultWg.Done()
					defer func() { <-semaphore }()

					members, err := api.FetchTeamMembers(ctx, restClient, team, team.GetOrganization().GetLogin())
					if err != nil {
						slog.Warn("Skipping team due to error fetching members", "error", err, "team", team.GetSlug())
						members = nil
					}

					externalGroups, err := api.FetchExternalGroups(ctx, restClient, team.GetOrganization().GetLogin(), team.GetSlug())
					if err != nil {
						slog.Warn("Skipping external groups due to error", "error", err, "team", team.GetSlug())
						externalGroups = nil
					}

					row := memberGroupResult{
						team:           team,
						org:            team.GetOrganization().GetLogin(),
						members:        members,
						externalGroups: externalGroups,
					}

					atomic.AddInt64(&teamsCount, 1)
					slog.Info("processing team", slog.String("team", team.GetSlug()))

					// Send the result to the results channel
					resultsChan <- row
				}(team)
			}
		}
	}

	// collect all results and then close resultsChan
	go func() {
		resultWg.Wait()
		slog.Info("processing teams complete", slog.Int64("total", teamsCount))
		close(resultsChan)
	}()

	// Write results to CSV
ResultsLoop:
	for {
		select {
		case <-ctx.Done():
			slog.Warn("Context cancelled writing report")
			return ctx.Err()
		case result, ok := <-resultsChan:
			if !ok {
				slog.Debug("Finished writing report")
				break ResultsLoop
			}
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
				slog.Warn("failed to write team to file", "error", err, "team", result.team.GetSlug())
			}
		}
	}

	slog.Info("teams report complete", slog.String("file", fileName))
	return nil
}
