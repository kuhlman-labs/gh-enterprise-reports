package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

type OrgReport struct {
	Organization *github.Organization
	Members      []*github.User
}

// OrganizationsReport generates a CSV report for all enterprise organizations, including organization details and memberships.
func OrganizationsReport(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, enterpriseSlug, filename string) error {
	slog.Info("starting organizations report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename))

	// Create CSV file to write the report
	header := []string{
		"Organization",
		"Organization ID",
		"Organization Default Repository Permission",
		"Members",
		"Total Members",
	}

	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	defer file.Close()

	// Fetch all enterprise organizations
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}

	// Channels for organization processing
	orgChan := make(chan *OrgReport, len(orgs))
	resultChan := make(chan *OrgReport, len(orgs))

	// Use a WaitGroup to wait for all workers to finish
	var orgWg sync.WaitGroup
	var orgCount int64

	// Start workers
	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		orgWg.Add(1)
		go processOrganization(ctx, &orgCount, &orgWg, orgChan, resultChan, restClient)
	}

	// Enqueue organizations for processing
	go func() {
		for _, org := range orgs {
			orgChan <- &OrgReport{
				Organization: org}
		}
		close(orgChan)
	}()

	// Collect results
	go func() {
		orgWg.Wait()
		slog.Info("processing organizations complete", "total", atomic.LoadInt64(&orgCount))
		close(resultChan)
	}()

	// Process results
	for result := range resultChan {

		rowData := []string{
			result.Organization.GetLogin(),
			fmt.Sprintf("%d", result.Organization.GetID()),
			result.Organization.GetDefaultRepoPermission(),
		}

		var membersString, totalMembers string

		// build JSON-encoded member list
		var members []struct {
			Login    string `json:"login"`
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			RoleName string `json:"roleName"`
		}
		for _, member := range result.Members {
			members = append(members, struct {
				Login    string `json:"login"`
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				RoleName string `json:"roleName"`
			}{
				Login:    member.GetLogin(),
				ID:       member.GetID(),
				Name:     member.GetName(),
				RoleName: member.GetRoleName(),
			})
		}
		data, err := json.Marshal(members)
		if err != nil {
			slog.Warn("Failed to marshal members to JSON", "err", err)
			membersString = "[]"
		} else {
			membersString = string(data)
		}
		totalMembers = fmt.Sprintf("%d", len(result.Members))

		rowData = append(rowData, membersString, totalMembers)

		if err := writer.Write(rowData); err != nil {
			return fmt.Errorf("failed to write row to CSV: %w", err)
		}

		slog.Debug("processed organization", "organization", result.Organization.GetLogin(), "members", len(result.Members))
	}

	// Ensure all CSV data is written out
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	slog.Info("organizations report complete", slog.String("filename", filename))
	return nil
}

// processOrganization processes the orgChannel and fetches organization details and memberships.
func processOrganization(ctx context.Context, count *int64, wg *sync.WaitGroup, in <-chan *OrgReport, out chan<- *OrgReport, restClient *github.Client) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			slog.Warn("context canceled, stopping organization processing")
			return
		case orgReport, ok := <-in:
			if !ok {
				slog.Debug("no more organizations to process")
				return
			}

			orgInfo, err := api.FetchOrganization(ctx, restClient, orgReport.Organization.GetLogin())
			if err != nil {
				slog.Warn("failed to fetch organization details; marking as unavailable", "organization", orgReport.Organization.GetLogin(), "err", err)
				continue
			}

			members, err := api.FetchOrganizationMemberships(ctx, restClient, orgReport.Organization.GetLogin())
			if err != nil {
				slog.Warn("failed to fetch memberships; marking as unavailable", "organization", orgReport.Organization.GetLogin(), "err", err)
				continue
			}
			// Update the orgReport with fetched data
			orgReport.Organization = orgInfo
			orgReport.Members = members

			atomic.AddInt64(count, 1)
			slog.Info("processing organization", "organization", orgReport.Organization.GetLogin())

			out <- orgReport
		}
	}

}
