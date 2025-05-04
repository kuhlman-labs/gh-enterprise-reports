// Package reports implements various report generation functionalities for GitHub Enterprise.
// It provides utilities and specific report types for organizations, repositories, teams,
// collaborators, and user data, with results exported as CSV files.
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// OrgReport represents an organization with its members list,
// used to generate organization reports with membership details.
type OrgReport struct {
	Organization *github.Organization // Organization details
	Members      []*github.User       // List of organization members
}

// OrgMemberInfo represents a simplified organization member for CSV output.
// Contains only the essential member information needed for reporting.
type OrgMemberInfo struct {
	Login    string `json:"login"`    // User's GitHub login name
	ID       int64  `json:"id"`       // User's numeric ID
	Name     string `json:"name"`     // User's display name
	RoleName string `json:"roleName"` // User's role in the organization (admin, member, etc.)
}

// OrganizationsReport generates a CSV report for all enterprise organizations.
// It fetches all organizations within the enterprise, along with their members and settings,
// and outputs the data to a CSV file.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - graphQLClient: GitHub GraphQL API client
//   - restClient: GitHub REST API client
//   - enterpriseSlug: Enterprise identifier
//   - filename: Output CSV file path
//   - workerCount: Number of concurrent workers for processing organizations
//
// The report includes organization name, ID, default repository permission settings,
// a JSON-encoded list of members with their details, and the total member count.
func OrganizationsReport(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, enterpriseSlug, filename string, workerCount int) error {
	slog.Info("starting organizations report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename), slog.Int("workers", workerCount))
	// Validate output path early to catch file creation errors before API calls
	if err := validateFilePath(filename); err != nil {
		return err
	}
	header := []string{
		"Organization",
		"Organization ID",
		"Organization Default Repository Permission",
		"Members",
		"Total Members",
	}

	// Fetch initial list of orgs
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}

	// Processor: enrich organization with details and members
	processor := func(ctx context.Context, org *github.Organization) (*OrgReport, error) {
		slog.Info("processing organization", "org", org.GetLogin())
		info, err := api.FetchOrganization(ctx, restClient, org.GetLogin())
		if err != nil {
			// Log the error but return a report with basic info and empty members.
			// The original 'org' from FetchEnterpriseOrgs lacks DefaultRepoPermission.
			slog.Warn("failed to fetch organization details, reporting basic info", "org", org.GetLogin(), "err", err)
			// Use the input 'org' which has at least Login and ID. Mark members as empty.
			// The formatter will handle the missing DefaultRepoPermission.
			return &OrgReport{Organization: org, Members: []*github.User{}}, nil // Return non-nil report, nil error
		}

		members, err := api.FetchOrganizationMemberships(ctx, restClient, org.GetLogin())
		if err != nil {
			// Log the error but return the fetched org details with empty members.
			slog.Warn("failed to fetch memberships, reporting org details with empty members", "org", org.GetLogin(), "err", err)
			return &OrgReport{Organization: info, Members: []*github.User{}}, nil // Return non-nil report, nil error
		}
		return &OrgReport{Organization: info, Members: members}, nil
	}

	// Formatter: build CSV row from OrgReport
	formatter := func(r *OrgReport) []string {
		var membersStr string
		var defaultPermStr string

		// Check if Members slice is nil or empty
		if len(r.Members) == 0 {
			membersStr = "N/A" // Set to "N/A" if no members
		} else {
			// Only marshal if there are members
			var membersList []OrgMemberInfo
			for _, m := range r.Members {
				// Add a nil check for individual members just in case
				if m == nil {
					continue
				}
				membersList = append(membersList, OrgMemberInfo{m.GetLogin(), m.GetID(), m.GetName(), m.GetRoleName()})
			}
			data, err := json.Marshal(membersList)
			if err != nil {
				slog.Error("failed to marshal members list", "org", r.Organization.GetLogin(), "err", err)
				membersStr = "ERROR_MARSHAL" // Indicate an error during marshaling
			} else {
				membersStr = string(data) // Use JSON string if marshaling succeeded
			}
		}

		// Handle potentially missing DefaultRepoPermission
		// GetDefaultRepoPermission returns "" if the field is nil.
		defaultPerm := r.Organization.GetDefaultRepoPermission()
		if defaultPerm == "" {
			defaultPermStr = "N/A" // Use "N/A" if permission wasn't fetched or is empty
		} else {
			defaultPermStr = defaultPerm
		}

		return []string{
			r.Organization.GetLogin(),
			fmt.Sprintf("%d", r.Organization.GetID()),
			defaultPermStr,                    // Use the determined defaultPermStr
			membersStr,                        // Use the determined membersStr
			fmt.Sprintf("%d", len(r.Members)), // Total members
		}
	}

	// Create a limiter for rate limiting - aiming for ~5 orgs/sec
	// (Consumes 10 REST points/sec, below the 15 points/sec limit)
	// Burst matches worker count for responsiveness.
	limiter := rate.NewLimiter(rate.Limit(5), workerCount) // e.g., 5 requests/sec, burst of workerCount

	// Run the report
	return RunReport(ctx, orgs, processor, formatter, limiter, workerCount, filename, header)
}
