package reports

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

type TeamReport struct {
	*github.Team
	*github.Organization
	ExternalGroups *github.ExternalGroupList
	Members        []*github.User
}

// setMembers sets the members of the team.
func (t *TeamReport) setMembers(members []*github.User) {
	// Check if members are nil
	if members == nil {
		members = []*github.User{}
	}
	t.Members = members
}

// setExternalGroups sets the external groups of the team.
func (t *TeamReport) setExternalGroups(externalGroups *github.ExternalGroupList) {
	// Check if external groups are nil
	if externalGroups == nil {
		externalGroups = &github.ExternalGroupList{}
	}
	t.ExternalGroups = externalGroups
}

// TeamsReport generates a CSV report of teams for the specified Enterprise.
// It includes columns for Team ID, Organization, Team Name, Team Slug, External Group, and Members.
func TeamsReport(ctx context.Context, restClient *github.Client, graphqlClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting teams report", slog.String("enterprise", enterpriseSlug), slog.String("file", filename))

	// Create CSV file to write the report
	header := []string{
		"Team ID",
		"Owner",
		"Team Name",
		"Team Slug",
		"External Group",
		"Members",
	}

	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	defer file.Close()

	// Get all organizations in the enterprise
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphqlClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to get organizations for enterprise %s: %w", enterpriseSlug, err)
	}

	// Channels for team processing
	teamChan := make(chan *TeamReport, 1000)    // buffer size to avoid blocking
	resultsChan := make(chan *TeamReport, 1000) // buffer size to avoid blocking

	var teamWg sync.WaitGroup
	var teamCount int64

	for i := 0; i < 20; i++ {
		teamWg.Add(1)
		go processTeams(ctx, &teamCount, &teamWg, teamChan, resultsChan, restClient)
	}

	// Close resultsChan when all teams are processed
	go func() {
		teamWg.Wait()
		slog.Info("processing teams complete", slog.Int64("total", teamCount))
		close(resultsChan)
	}()

	// Enqueue teams for processing
	go func() {
		defer close(teamChan)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for _, org := range orgs {
			select {
			case <-ctx.Done():
				slog.Warn("context cancelled, stopping team processing")
				return
			case <-ticker.C:
				teams, err := api.FetchTeamsForOrganizations(ctx, restClient, org.GetLogin())
				if err != nil {
					slog.Warn("failed to fetch teams for org", "org", org.GetLogin(), "error", err)
					continue
				}
				if len(teams) == 0 {
					slog.Debug("no teams found for org", "org", org.GetLogin())
					continue
				}
				for _, team := range teams {
					teamChan <- &TeamReport{
						Team:         team,
						Organization: org,
					}
				}
			}
		}
	}()

	for team := range resultsChan {

		// Build members string or "N/A"
		var membersStr string
		if len(team.Members) == 0 {
			membersStr = "N/A"
		} else {
			mLogins := make([]string, len(team.Members))
			for i, member := range team.Members {
				mLogins[i] = member.GetLogin()
			}
			membersStr = strings.Join(mLogins, ", ")
		}

		// Build external‐groups string or "N/A"
		var externalGroupsStr string
		if team.ExternalGroups == nil || len(team.ExternalGroups.Groups) == 0 {
			externalGroupsStr = "N/A"
		} else {
			gNames := make([]string, len(team.ExternalGroups.Groups))
			for i, group := range team.ExternalGroups.Groups {
				gNames[i] = group.GetGroupName()
			}
			externalGroupsStr = strings.Join(gNames, ", ")
		}

		// Prepare CSV row
		rowData := []string{
			fmt.Sprintf("%d", team.Team.GetID()),
			team.Organization.GetLogin(),
			team.Team.GetName(),
			team.GetSlug(),
			externalGroupsStr,
			membersStr,
		}
		// Write the row to the CSV file
		if err := writer.Write(rowData); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	// Ensure all CSV data is written out
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	slog.Info("completed teams report", slog.String("file", filename))
	return nil
}

// processTeams processes the teams for a given organization and sends to the team channel.
func processTeams(ctx context.Context, count *int64, wg *sync.WaitGroup, in <-chan *TeamReport, out chan<- *TeamReport, restClient *github.Client) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			slog.Warn("context cancelled, stopping processing teams")
			return
		case team, ok := <-in:
			if !ok {
				slog.Debug("no more teams to process")
				return
			}

			// Fetch members and external groups for each team
			members, err := api.FetchTeamMembers(ctx, restClient, team.Team, team.Organization.GetLogin())
			if err != nil {
				slog.Debug("Skipping team due to error fetching members", "error", err, "team", team.GetSlug())
				members = nil
			}
			externalGroups, err := api.FetchExternalGroups(ctx, restClient, team.Organization.GetLogin(), team.GetSlug())
			if err != nil {
				slog.Debug("Skipping external groups due to error", "error", err, "team", team.GetSlug())
				externalGroups = nil
			}

			// Set members and external groups for the team
			team.setMembers(members)
			team.setExternalGroups(externalGroups)
			// Send the team report to the output channel
			out <- team
			// Increment the team count
			atomic.AddInt64(count, 1)
			slog.Info("processing team", slog.String("team", team.GetSlug()))

		}
	}
}
