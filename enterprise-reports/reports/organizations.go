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

type Members struct {
	Login string
	ID    string
	Name  string
	Type  string
}

// writeCSVHeader writes the header row to the CSV file.
func writeCSVHeader(writer *csv.Writer, headers []string) error {
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}
	return nil
}

// runOrganizationsReport generates a CSV report for all enterprise organizations, including organization details and memberships.
func OrganizationsReport(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, enterpriseSlug, filename string) error {

	slog.Info("Starting organizations report generation", "filename", filename)

	// Create and open the CSV file
	file, err := os.Create(filename)
	if err != nil {
		slog.Error("Failed to create report file", "filename", filename, "err", err)
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("Failed to close report file", "filename", filename, "err", err)
		}
	}()

	writer := csv.NewWriter(file)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			slog.Warn("Failed to flush CSV writer", "err", err)
		}
	}()

	// Write CSV header
	if err := writeCSVHeader(writer, []string{
		"Organization",
		"Organization ID",
		"Organization Default Repository Permission",
		"Members",
		"Total Members",
	}); err != nil {
		slog.Error("Failed to write CSV header", "err", err)
		return err
	}

	// Fetch organizations
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}

	type orgResult struct {
		organization *github.Organization
		members      []*github.User
		err          error
	}

	// Concurrency setup
	orgChan := make(chan *github.Organization, len(orgs))
	resultChan := make(chan orgResult, len(orgs))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit the number of concurrent workers

	// Start worker function with context cancellation checks
	worker := func() {
		defer wg.Done()
		for org := range orgChan {
			// Check for context cancellation before processing an organization.
			if ctx.Err() != nil {
				slog.Warn("Context canceled, stopping worker", "organization", *org.Login)
				return
			}
			semaphore <- struct{}{} // Acquire semaphore token
			organization, err := api.FetchOrganization(ctx, restClient, *org.Login)
			<-semaphore // Release token
			if err != nil {
				slog.Warn("Failed to fetch organization details; marking as unavailable", "organization", *org.Login, "err", err)
				resultChan <- orgResult{organization: nil, members: nil, err: fmt.Errorf("failed to fetch organization details for %s: %w", *org.Login, err)}
				continue
			}
			// Check cancellation after fetching organization details.
			if ctx.Err() != nil {
				slog.Warn("Context canceled after fetching details, stopping worker", "organization", *org.Login)
				return
			}

			users, err := api.FetchOrganizationMemberships(ctx, restClient, organization.GetLogin())
			if err != nil {
				slog.Warn("Failed to fetch memberships; marking as unavailable", "organization", *org.Login, "err", err)
				resultChan <- orgResult{organization: organization, members: nil, err: fmt.Errorf("failed to fetch memberships for %s: %w", *org.Login, err)}
				continue
			}

			resultChan <- orgResult{organization: organization, members: users}
		}
	}

	// Start workers
	numWorkers := 5
	workerIDs := make([]int, numWorkers)
	for range workerIDs {
		wg.Add(1)
		go worker()
	}

	// Send organizations to workers
	go func() {
		for _, org := range orgs {
			orgChan <- org
		}
		close(orgChan)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

OuterLoop:
	for result := range resultChan {
		select {
		case <-ctx.Done():
			slog.Warn("Context canceled, stopping result processing")
			break OuterLoop
		default:
		}

		var orgLogin, orgID, defaultRepoPermission, membersString, totalMembers string

		if result.err != nil {
			slog.Warn("Error processing organization. Writing placeholder information", "err", result.err)
			orgLogin = result.err.Error()
			orgID = "N/A"
			defaultRepoPermission = "N/A"
			membersString = "N/A"
			totalMembers = "N/A"
		} else {
			orgLogin = result.organization.GetLogin()
			orgID = fmt.Sprintf("%d", result.organization.GetID())
			defaultRepoPermission = result.organization.GetDefaultRepoPermission()

			var memberDetails []string
			for _, user := range result.members {
				memberDetails = append(memberDetails, fmt.Sprintf("{%s,%d,%s,%s}",
					user.GetLogin(),
					user.GetID(),
					user.GetName(),
					user.GetRoleName(),
				))
			}
			membersString = strings.Join(memberDetails, ",")
			totalMembers = fmt.Sprintf("%d", len(result.members))
		}

		if err := writer.Write([]string{
			orgLogin,
			orgID,
			defaultRepoPermission,
			membersString,
			totalMembers,
		}); err != nil {
			slog.Warn("Failed to write organization to report; skipping", "organization", orgLogin, "err", err)
			continue
		}
		slog.Debug("Successfully processed organization", "organization", orgLogin)
	}

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		slog.Error("Context deadline exceeded while generating organizations report", "err", ctx.Err())
		return ctx.Err()
	}

	slog.Info("Organizations report generated successfully", "filename", filename)
	return nil
}
