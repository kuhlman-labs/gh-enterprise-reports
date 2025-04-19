package reports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

// runOrganizationsReport generates a CSV report for all enterprise organizations, including organization details and memberships.
func OrganizationsReport(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, enterpriseSlug, filename string) error {
	slog.Info("starting organizations report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename))

	// Create CSV file to write the report
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"Organization",
		"Organization ID",
		"Organization Default Repository Permission",
		"Members",
		"Total Members",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to file: %w", err)
	}

	// Fetch all enterprise organizations
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}

	type orgResult struct {
		organization *github.Organization
		members      []*github.User
		err          error
	}

	// Channels for organization processing
	orgChan := make(chan *github.Organization, len(orgs))
	resultChan := make(chan orgResult, len(orgs))

	// Use a WaitGroup to wait for all workers to finish
	// and a semaphore to limit the number of concurrent workers
	var wg sync.WaitGroup
	var orgCount int64
	semaphore := make(chan struct{}, 10) // Limit the number of concurrent workers

	// Worker function to process organizations
	worker := func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				slog.Warn("Context canceled, stopping worker")
				return
			case org, ok := <-orgChan:
				if !ok {
					// all work dispatched
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
				// Check cancellation after fetching details.
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

				row := orgResult{
					organization: organization,
					members:      users,
				}

				atomic.AddInt64(&orgCount, 1)
				slog.Info("processing organization", "organization", *org.Login)

				resultChan <- row

			}
		}
	}

	// Start workers
	numWorkers := 5
	for range numWorkers {
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
		slog.Info("processing organizations complete", "total", atomic.LoadInt64(&orgCount))
		close(resultChan)
	}()

ResultsLoop:
	for {
		select {
		case <-ctx.Done():
			slog.Warn("context canceled, stopping results processing")
			return fmt.Errorf("context canceled while writing csv: %w", ctx.Err())
		case result, ok := <-resultChan:
			if !ok {
				slog.Debug("all records processed, closing CSV writer")
				break ResultsLoop
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

				// build JSON-encoded member list
				var members []struct {
					Login    string `json:"login"`
					ID       int64  `json:"id"`
					Name     string `json:"name"`
					RoleName string `json:"roleName"`
				}
				for _, user := range result.members {
					members = append(members, struct {
						Login    string `json:"login"`
						ID       int64  `json:"id"`
						Name     string `json:"name"`
						RoleName string `json:"roleName"`
					}{
						Login:    user.GetLogin(),
						ID:       user.GetID(),
						Name:     user.GetName(),
						RoleName: user.GetRoleName(),
					})
				}
				data, err := json.Marshal(members)
				if err != nil {
					slog.Warn("Failed to marshal members to JSON", "err", err)
					membersString = "[]"
				} else {
					membersString = string(data)
				}
				totalMembers = fmt.Sprintf("%d", len(result.members))
			}

			if err := writer.Write([]string{
				orgLogin,
				orgID,
				defaultRepoPermission,
				membersString,
				totalMembers,
			}); err != nil {
				slog.Warn("failed to write organization to report; skipping", "organization", orgLogin, "err", err)
				continue
			}
			slog.Debug("processed organization", "organization", orgLogin)
		}
	}

	slog.Info("organizations report complete", slog.String("filename", filename))
	return nil
}
