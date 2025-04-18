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

// runCollaboratorsReport generates a CSV report of repository collaborators for the specified enterprise.
// The report includes repository full name and collaborator details (login, ID, and highest permission level).
func CollaboratorsReport(ctx context.Context, restClient *github.Client, graphClient *githubv4.Client, enterpriseSlug, fileName string) error {
	slog.Info("starting collaborators report generation", slog.String("enterprise", enterpriseSlug))

	// Create CSV file for report.
	file, err := os.Create(fileName)
	if err != nil {
		slog.Error("failed to create csv file", slog.String("filename", fileName), slog.Any("err", err))
		return fmt.Errorf("failed to create csv file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header row.
	header := []string{"Repository", "Collaborators"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	// Fetch all enterprise organizations.
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch enterprise organizations: %w", err)
	}

	// Create a worker pool.
	const maxWorkers = 50 // Limit to 50 concurrent workers to avoid hitting secondary rate limits.
	repoChan := make(chan *github.Repository, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects CSV writer.

	// Start worker pool.
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range repoChan {
				// Fetch collaborators for repository.
				collaborators, err := api.FetchRepoCollaborators(ctx, restClient, repo)
				if err != nil {
					slog.Warn("skipping repository due to collaborator fetch error",
						slog.String("repository", repo.GetFullName()),
						slog.Any("err", err),
					)
					continue
				}

				// Format collaborators into a string.
				var collaboratorStrings []string
				for _, collaborator := range collaborators {
					highestPermission := getHighestPermission(collaborator.GetPermissions())
					collaboratorStrings = append(collaboratorStrings, fmt.Sprintf("{Login: %s, ID: %d, Permission: %s}", collaborator.GetLogin(), collaborator.GetID(), highestPermission))
				}
				collaboratorString := strings.Join(collaboratorStrings, ", ")

				// Write repository and collaborators to CSV.
				mu.Lock()
				if err := writer.Write([]string{repo.GetFullName(), collaboratorString}); err != nil {
					slog.Error("failed to write collaborators to csv",
						slog.String("repository", repo.GetFullName()),
						slog.Any("err", err),
					)
				} else {
					slog.Info("collaborators written to csv", slog.String("repository", repo.GetFullName()))
				}
				mu.Unlock()
			}
		}()
	}

	// Enqueue repositories into the worker pool.
	for _, org := range orgs {
		// Fetch repositories for the organization.
		repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
		if err != nil {
			slog.Warn("skipping organization due to repository fetch error",
				slog.String("organization", org.GetLogin()),
				slog.Any("err", err),
			)
			continue
		}

		for _, repo := range repos {
			repoChan <- repo
		}
	}

	// Close the channel and wait for workers to finish.
	close(repoChan)
	wg.Wait()

	// Check for CSV writer errors.
	if err := writer.Error(); err != nil {
		slog.Error("error writing to csv file", slog.Any("err", err))
		return fmt.Errorf("error writing to csv file: %w", err)
	}

	slog.Info("collaborators report generated successfully", slog.String("filename", fileName))
	return nil
}

// getHighestPermission returns the highest permission level from the provided permissions map.
func getHighestPermission(permissions map[string]bool) string {
	switch {
	case permissions["admin"]:
		return "admin"
	case permissions["maintain"]:
		return "maintain"
	case permissions["push"]:
		return "push"
	case permissions["triage"]:
		return "triage"
	case permissions["pull"]:
		return "pull"
	default:
		return "none"
	}
}
