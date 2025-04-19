package reports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

type RepoTeam struct {
	TeamID        int64
	TeamName      string
	TeamSlug      string
	ExternalGroup []string
	Permission    string
}

// runRepositoryReport generates a CSV report for repositories, including repository details, teams, and custom properties.
func RepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting repository report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename))

	// Create CSV file to write the report
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	header := []string{
		"Owner",
		"Repository",
		"Archived",
		"Visibility",
		"Pushed_At",
		"Created_At",
		"Topics",
		"Custom_Properties",
		"Teams",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to file: %w", err)
	}

	// Set up concurrency limits
	maxWorkers := 5
	resultBufferSize := 100

	// Create a channel for CSV rows
	resultsChan := make(chan []string, resultBufferSize)
	repoChan := make(chan *github.Repository)

	// Semaphore channels for limiting concurrency
	semOrg := make(chan struct{}, maxWorkers)  // for org‐fetch
	semRepo := make(chan struct{}, maxWorkers) // for repo‐processing
	// WaitGroup for goroutines
	var wg sync.WaitGroup
	var wgWorkers sync.WaitGroup

	var repoCount int64

	// Start repo workers
	for i := 0; i < maxWorkers; i++ {
		wgWorkers.Add(1)
		go func() {
			defer wgWorkers.Done()
			for repo := range repoChan {
				select {
				case <-ctx.Done():
					slog.Warn("context cancelled, stopping repository processing")
					return
				case semRepo <- struct{}{}: // acquire repo token
				}
				func(repo *github.Repository) {
					defer func() { <-semRepo }() // release repo token
					slog.Debug("processing repository", "repository", repo.GetFullName())
					// Fetch teams sequentially (no unbounded goroutines)
					teams, err := api.FetchTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						slog.Debug("failed to get teams", "repository", repo.GetFullName(), "error", err)
						return
					}
					var collected []RepoTeam
					for _, t := range teams {
						externalGroups, err := api.FetchExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), t.GetSlug())
						if err != nil {
							slog.Debug("failed to get external groups", "repository", repo.GetFullName(), "error", err)
							continue
						}
						groupNames := make([]string, 0, len(externalGroups.Groups))
						for _, eg := range externalGroups.Groups {
							groupNames = append(groupNames, *eg.GroupName)
						}
						collected = append(collected, RepoTeam{
							TeamID:        t.GetID(),
							TeamName:      t.GetName(),
							TeamSlug:      t.GetSlug(),
							ExternalGroup: groupNames,
							Permission:    t.GetPermission(),
						})
					}
					// Sort for deterministic JSON order
					sort.Slice(collected, func(i, j int) bool {
						return collected[i].TeamSlug < collected[j].TeamSlug
					})
					teamsJSON, err := json.Marshal(collected)
					teamsStr := "[]"
					if err == nil {
						teamsStr = string(teamsJSON)
					} else {
						slog.Warn("failed to marshal teams to JSON", "error", err)
					}

					customProperties, err := api.FetchCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
					if err != nil {
						slog.Debug("failed to get custom properties", "repository", repo.GetFullName(), "error", err)
						return
					}

					propsStr := ""

					// JSON-encode custom properties
					propsJSON, err := json.Marshal(customProperties)
					if err != nil {
						slog.Warn("failed to marshal custom properties to JSON", "error", err)
						propsStr = "[]"
					} else {
						propsStr = string(propsJSON)
					}

					row := []string{
						repo.GetOwner().GetLogin(),
						repo.GetName(),
						fmt.Sprintf("%t", repo.GetArchived()),
						repo.GetVisibility(),
						repo.GetPushedAt().Format(time.RFC3339),
						repo.GetCreatedAt().Format(time.RFC3339),
						fmt.Sprintf("%v", repo.Topics),
						propsStr,
						teamsStr,
					}

					// Send the row (abort on cancellation)
					select {
					case resultsChan <- row:
					case <-ctx.Done():
						return
					}

					slog.Debug("finished processing repository", "repository", repo.GetFullName())
				}(repo)
			}
		}()
	}

	// Get Enterprise Organizations
	organizations, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return err
	}

	// Enqueue repositories
	for _, org := range organizations {
		wg.Add(1)
		go func(org *github.Organization) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case semOrg <- struct{}{}: // acquire org token
			}
			defer func() { <-semOrg }() // release org token
			slog.Debug("processing organization", "organization", org.GetLogin())
			repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
			if err != nil {
				slog.Debug("failed to get repositories", "organization", org.GetLogin(), "error", err)
				return
			}
			for _, repo := range repos {
				repoChan <- repo
				atomic.AddInt64(&repoCount, 1)
				slog.Info("processing repository", "repository", repo.GetFullName())
			}
		}(org)
	}

	go func() {
		wg.Wait()
		slog.Info("processing repositories complete", slog.Int64("total", atomic.LoadInt64(&repoCount)))
		close(repoChan)
		wgWorkers.Wait()
		close(resultsChan)
	}()

	// Write rows from resultsChan to CSV
ResultsLoop:
	for {
		select {
		case <-ctx.Done():
			slog.Warn("context cancelled, stopping results processing")
			return fmt.Errorf("context cancelled while writing csv: %w", ctx.Err())
		case row, ok := <-resultsChan:
			if !ok {
				slog.Debug("all records processed, closing CSV writer")
				break ResultsLoop
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write repository report to CSV: %w", err)
			}
		}
	}

	slog.Info("repository report complete", slog.String("filename", filename))
	return nil
}
