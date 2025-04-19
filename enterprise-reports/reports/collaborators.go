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

// runCollaboratorsReport generates a CSV report of repository collaborators for the specified enterprise.
// The report includes repository full name and collaborator details (login, ID, and highest permission level).
func CollaboratorsReport(ctx context.Context, restClient *github.Client, graphClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting collaborators report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename))

	// Create CSV file to write the report
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Repository", "Collaborators"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to file: %w", err)
	}

	// Fetch all enterprise organizations.
	orgs, err := api.FetchEnterpriseOrgs(ctx, graphClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch enterprise organizations: %w", err)
	}

	// Create a worker pool.
	const maxWorkers = 50 // Limit to 50 concurrent workers to avoid hitting secondary rate limits.
	repoChan := make(chan *github.Repository, maxWorkers)
	resultsChan := make(chan []string, maxWorkers)

	var wg sync.WaitGroup
	var collaboratorCount int64

	// Start worker pool.
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case repo, ok := <-repoChan:
					if !ok {
						return
					}
					// Fetch collaborators for repository.
					collaborators, err := api.FetchRepoCollaborators(ctx, restClient, repo)
					if err != nil {
						slog.Warn("skipping repository due to collaborator fetch error",
							slog.String("repository", repo.GetFullName()),
							slog.Any("err", err),
						)
						continue
					}

					// build JSON-encoded collaborator list
					var infos []struct {
						Login      string `json:"login"`
						ID         int64  `json:"id"`
						Permission string `json:"permission"`
					}
					for _, c := range collaborators {
						infos = append(infos, struct {
							Login      string `json:"login"`
							ID         int64  `json:"id"`
							Permission string `json:"permission"`
						}{
							Login:      c.GetLogin(),
							ID:         c.GetID(),
							Permission: getHighestPermission(c.GetPermissions()),
						})
					}
					data, err := json.Marshal(infos)
					if err != nil {
						slog.Warn("failed to marshal collaborators to JSON",
							slog.String("repository", repo.GetFullName()),
							slog.Any("err", err),
						)
						data = []byte("[]")
					}
					record := []string{repo.GetFullName(), string(data)}

					atomic.AddInt64(&collaboratorCount, int64(len(infos)))
					slog.Info("processing collaborators for repository", "repository", repo.GetFullName(), "collaborators", len(infos))
					resultsChan <- record
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Enqueue repositories into the worker pool.
	for _, org := range orgs {
		slog.Debug("processing organization", slog.String("organization", org.GetLogin()))
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
			slog.Debug("processing repository", slog.String("repository", repo.GetFullName()))
			select {
			case repoChan <- repo:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	close(repoChan)
	wg.Wait()
	slog.Info("processing collaborators complete", slog.Int64("total", atomic.LoadInt64(&collaboratorCount)))
	close(resultsChan)

	// Single goroutine to write CSV rows.
ResultsLoop:
	for {
		select {
		case record, ok := <-resultsChan:
			if !ok {
				slog.Debug("all records processed, closing CSV writer")
				break ResultsLoop
			}
			if err := writer.Write(record); err != nil {
				slog.Error("failed to write collaborators to csv",
					slog.String("repository", record[0]),
					slog.Any("err", err),
				)
			} else {
				slog.Debug("collaborators written to csv", slog.String("repository", record[0]))
			}
		case <-ctx.Done():
			slog.Warn("context cancelled, stopping results processing", slog.Any("err", ctx.Err()))
			return fmt.Errorf("context cancelled while writing csv: %w", ctx.Err())
		}
	}

	slog.Info("collaborators report complete", slog.String("filename", filename))
	return nil
}
