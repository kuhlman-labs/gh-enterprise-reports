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

// getRepoCollaborators retrieves all collaborators for the specified repository.
func getRepoCollaborators(ctx context.Context, restClient *github.Client, repo *github.Repository) ([]*github.User, error) {
	log.Info().Str("Repository", repo.GetFullName()).Msg("Fetching repository collaborators.")

	opts := &github.ListCollaboratorsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	var allCollaborators []*github.User
	for {
		collaborators, resp, err := restClient.Repositories.ListCollaborators(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
		if err != nil {
			log.Error().Err(err).Str("Repository", repo.GetFullName()).Msg("Failed to fetch collaborators.")
			return nil, fmt.Errorf("failed to fetch collaborators for repository %s: %w", repo.GetFullName(), err)
		}
		allCollaborators = append(allCollaborators, collaborators...)

		// Check REST API rate limit.
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	log.Info().Int("TotalCollaborators", len(allCollaborators)).Str("Repository", repo.GetFullName()).Msg("Successfully fetched repository collaborators.")
	return allCollaborators, nil
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

// runCollaboratorsReport generates a CSV report of repository collaborators for the specified enterprise.
// The report includes repository full name and collaborator details (login, ID, and highest permission level).
func runCollaboratorsReport(ctx context.Context, restClient *github.Client, graphClient *githubv4.Client, enterpriseSlug string, config *Config, fileName string) error {
	log.Info().Str("Enterprise", enterpriseSlug).Msg("Starting collaborators report generation.")

	// Create CSV file for report.
	file, err := os.Create(fileName)
	if err != nil {
		log.Error().Err(err).Str("Filename", fileName).Msg("Failed to create CSV file.")
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header row.
	header := []string{"Repository", "Collaborators"}
	if err := writer.Write(header); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header.")
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Fetch all enterprise organizations.
	orgs, err := getEnterpriseOrgs(ctx, graphClient, config)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch enterprise organizations.")
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
				collaborators, err := getRepoCollaborators(ctx, restClient, repo)
				if err != nil {
					log.Warn().Err(err).Str("Repository", repo.GetFullName()).Msg("Skipping repository due to collaborator fetch error.")
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
					log.Error().Err(err).Str("Repository", repo.GetFullName()).Msg("Failed to write collaborators to CSV.")
				} else {
					log.Info().Str("Repository", repo.GetFullName()).Msg("Collaborators written to CSV.")
				}
				mu.Unlock()
			}
		}()
	}

	// Enqueue repositories into the worker pool.
	for _, org := range orgs {
		// Fetch repositories for the organization.
		repos, err := getOrganizationRepositories(ctx, restClient, org.Login)
		if err != nil {
			log.Warn().Err(err).Str("Organization", org.Login).Msg("Skipping organization due to repository fetch error.")
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
		log.Error().Err(err).Msg("Error occurred during CSV writing.")
		return fmt.Errorf("error writing to CSV file: %w", err)
	}

	log.Info().Str("Filename", fileName).Msg("Collaborators report generated successfully.")
	return nil
}
