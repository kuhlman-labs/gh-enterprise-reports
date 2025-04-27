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

type CollaboratorReport struct {
	Repository    *github.Repository
	Collaborators []CollaboratorInfo
}

type CollaboratorInfo struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	Permission string `json:"permission"`
}

// CollaboratorsReport generates a CSV report of repository collaborators for the enterprise.
func CollaboratorsReport(ctx context.Context, restClient *github.Client, graphClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting collaborators report",
		slog.String("enterprise", enterpriseSlug),
		slog.String("filename", filename),
	)

	header := []string{"Repository", "Collaborators"}
	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close CSV file", slog.Any("err", err))
		}
	}()

	orgs, err := api.FetchEnterpriseOrgs(ctx, graphClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("failed to fetch enterprise orgs: %w", err)
	}

	const maxWorkers = 50
	repoChan := make(chan *CollaboratorReport, maxWorkers)
	resultsChan := make(chan *CollaboratorReport, maxWorkers)

	var wg sync.WaitGroup
	var totalCollaborators int64

	// start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go processRepoCollaborators(ctx, &wg, &totalCollaborators, repoChan, resultsChan, restClient)
	}

	// enqueue repos
	go func() {
		for _, org := range orgs {
			repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
			if err != nil {
				slog.Error("failed to fetch repositories for org",
					slog.String("org", org.GetLogin()),
					slog.Any("err", err),
				)
				continue
			}
			for _, repo := range repos {
				slog.Debug("queuing repository", slog.String("repository", repo.GetFullName()))
				repoChan <- &CollaboratorReport{
					Repository: repo,
				}
			}
		}
		close(repoChan)
	}()

	// wait for workers then close results
	go func() {
		wg.Wait()
		slog.Info("processing repository collaborators complete", slog.Int64("total", atomic.LoadInt64(&totalCollaborators)))
		close(resultsChan)
	}()

	// write CSV

	for result := range resultsChan {
		record := make([]string, 0, len(result.Collaborators)+1)
		record = append(record, result.Repository.GetFullName())
		for _, collaborator := range result.Collaborators {
			collabJSON, err := json.Marshal(collaborator)
			if err != nil {
				slog.Error("failed to marshal collaborator info",
					slog.String("repository", result.Repository.GetFullName()),
					slog.Any("collaborator", collaborator),
					slog.Any("err", err),
				)
				continue
			}
			record = append(record, string(collabJSON))
		}
		// Write the record to the CSV file

		if err := writer.Write(record); err != nil {
			slog.Error("failed to write collaborators to csv",
				slog.String("repository", record[0]),
				slog.Any("err", err),
			)
		} else {
			slog.Debug("collaborators written to csv", slog.String("repository", record[0]))
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush failure: %w", err)
	}
	slog.Info("collaborators report complete", slog.String("filename", filename))
	return nil
}

// processRepoCollaborators fetches collaborators for a single repo and sends a RepoReport.
func processRepoCollaborators(ctx context.Context, wg *sync.WaitGroup, counter *int64, in <-chan *CollaboratorReport, out chan<- *CollaboratorReport,
	restClient *github.Client,
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case repo, ok := <-in:
			if !ok {
				return
			}
			repoCollaborators, err := api.FetchRepoCollaborators(ctx, restClient, repo.Repository)
			if err != nil {
				slog.Debug("skipping repository",
					slog.String("repo", repo.Repository.GetFullName()),
					slog.Any("err", err),
				)
				continue
			}
			var collaborators []CollaboratorInfo
			for _, collaborator := range repoCollaborators {
				collaborators = append(collaborators, CollaboratorInfo{
					Login:      collaborator.GetLogin(),
					ID:         collaborator.GetID(),
					Permission: getHighestPermission(collaborator.GetPermissions()),
				})
			}
			atomic.AddInt64(counter, 1)
			slog.Info("processing repository collaborators",
				slog.String("repo", repo.Repository.GetFullName()),
			)

			repo.Collaborators = collaborators

			out <- repo
		}
	}
}
