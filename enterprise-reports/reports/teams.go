package reports

import (
	"context"
	"fmt"
	"strings"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

type TeamReport struct {
	*github.Team
	*github.Organization
	ExternalGroups *github.ExternalGroupList
	Members        []*github.User
}

// TeamsReport generates a CSV report of teams for the specified Enterprise.
func TeamsReport(ctx context.Context, restClient *github.Client, graphqlClient *githubv4.Client, enterpriseSlug, filename string, workerCount int) error {
	slog.Info("starting teams report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename), slog.Int("workers", workerCount))
	// Validate output path early to catch file creation errors before API calls
	if err := validateFilePath(filename); err != nil {
		return err
	}
	header := []string{
		"Team ID",
		"Owner",
		"Team Name",
		"Team Slug",
		"External Group",
		"Members",
	}
	// Fetch organizations
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphqlClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}
	// Prepare initial items: organization teams
	var items []*TeamReport
	for _, org := range orgs {
		slog.Info("fetching teams for org", "org", org.GetLogin())
		teams, err := api.FetchTeamsForOrganizations(ctx, restClient, org.GetLogin())
		if err != nil {
			slog.Debug("failed to fetch teams for org", "org", org.GetLogin(), "err", err)
			continue
		}
		for _, t := range teams {
			items = append(items, &TeamReport{Team: t, Organization: org})
		}
	}
	// Processor: fetch members and external groups
	processor := func(ctx context.Context, tr *TeamReport) (*TeamReport, error) {
		slog.Info("processing team", "team", tr.GetSlug())
		members, err := api.FetchTeamMembers(ctx, restClient, tr.Team, tr.GetLogin())
		if err != nil {
			slog.Debug("skipping membership fetch", "team", tr.GetSlug(), "err", err)
			tr.Members = []*github.User{} // Initialize to empty slice on error
		} else {
			tr.Members = members // Assign fetched members if successful
		}

		ext, err := api.FetchExternalGroups(ctx, restClient, tr.GetLogin(), tr.GetSlug())
		if err != nil {
			slog.Debug("skipping external groups fetch", "team", tr.GetSlug(), "err", err)
			tr.ExternalGroups = &github.ExternalGroupList{} // Initialize to empty struct on error
		} else {
			tr.ExternalGroups = ext // Assign fetched external groups if successful
		}

		return tr, nil
	}
	// Formatter: build CSV row
	formatter := func(tr *TeamReport) []string {
		// members
		mem := "N/A"
		if len(tr.Members) > 0 {
			var logins []string
			for _, m := range tr.Members {
				logins = append(logins, m.GetLogin())
			}
			mem = strings.Join(logins, ", ")
		}
		// external groups
		eg := "N/A"
		if tr.ExternalGroups != nil && len(tr.ExternalGroups.Groups) > 0 {
			var names []string
			for _, g := range tr.ExternalGroups.Groups {
				names = append(names, g.GetGroupName())
			}
			eg = strings.Join(names, ", ")
		}
		return []string{
			fmt.Sprintf("%d", tr.Team.GetID()),
			tr.GetLogin(),
			tr.Team.GetName(),
			tr.GetSlug(),
			eg,
			mem,
		}
	}

	// Create a limiter for rate limiting - aiming for ~5 teams/sec
	// (Consumes 10 REST points/sec, below the 15 points/sec limit)
	// Burst matches worker count for responsiveness.
	limiter := rate.NewLimiter(rate.Limit(5), workerCount) // e.g., 5 requests/sec, burst of workerCount

	// Run the report
	return RunReport(ctx, items, processor, formatter, limiter, workerCount, filename, header)
}
