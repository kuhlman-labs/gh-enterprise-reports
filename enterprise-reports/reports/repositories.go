package reports

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

type RepoReport struct {
	*github.Repository
	Teams            []*repoTeam
	CustomProperties []*github.CustomPropertyValue
}

type repoTeam struct {
	*github.Team
	ExternalGroups *github.ExternalGroupList
}

// runRepositoryReport generates a CSV report for repositories, including repository details, teams, and custom properties.
func RepositoryReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting repository report", slog.String("enterprise", enterpriseSlug), slog.String("filename", filename))

	// Create CSV file to write the report
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
	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	defer file.Close()

	// Get all organizations in the enterprise
	organizations, err := api.FetchEnterpriseOrgs(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return err
	}

	// Set up concurrency limits
	maxWorkers := 10
	resultBufferSize := 100
	// limit to 1 request every 10ms which is 100 requests per second
	limiter := rate.NewLimiter(rate.Every(10*time.Millisecond), 1)

	// Create a channels for repository processing
	repoChan := make(chan *RepoReport, resultBufferSize)
	resultsChan := make(chan *RepoReport, resultBufferSize)

	// Start repo workers
	var repoWg sync.WaitGroup
	var repoCount int64
	for i := 0; i < maxWorkers; i++ {
		repoWg.Add(1)
		go processRepository(ctx, &repoCount, &repoWg, repoChan, resultsChan, restClient)
	}

	// Close the results channel when all workers are done
	go func() {
		repoWg.Wait()
		slog.Info("processing repositories complete", slog.Int64("total", atomic.LoadInt64(&repoCount)))
		close(resultsChan)
	}()

	// Enque repositories for processing
	go func() {
		defer close(repoChan)
		for _, org := range organizations {
			// before each API call
			if err := limiter.Wait(ctx); err != nil {
				slog.Warn("rate limiter interrupted", "err", err)
				return
			}

			repos, err := api.FetchOrganizationRepositories(ctx, restClient, org.GetLogin())
			if err != nil {
				slog.Error("failed to fetch repositories", "organization", org.GetLogin(), "error", err)
				continue
			}
			for _, repo := range repos {
				repoChan <- &RepoReport{Repository: repo}
			}
		}
	}()

	// Write rows from resultsChan to CSV
	for repo := range resultsChan {
		// format custom properties into "Name=Value" strings
		propStrs := make([]string, 0, len(repo.CustomProperties))
		for _, cp := range repo.CustomProperties {
			propStrs = append(propStrs, fmt.Sprintf("%s=%v", cp.PropertyName, cp.Value))
		}

		rowData := []string{
			repo.GetOwner().GetLogin(),
			repo.GetName(),
			fmt.Sprintf("%t", repo.GetArchived()),
			repo.GetVisibility(),
			repo.GetPushedAt().String(),
			repo.GetCreatedAt().String(),
			fmt.Sprintf("%v", repo.Topics),
			strings.Join(propStrs, ","),
		}
		var teams []string
		for _, team := range repo.Teams {
			teamName := team.GetSlug()
			if team.ExternalGroups != nil {
				for _, group := range team.ExternalGroups.Groups {
					teamName += fmt.Sprintf(" (%s)", group.GetGroupName())
				}
			}
			teams = append(teams, teamName)
		}
		rowData = append(rowData, strings.Join(teams, ","))
		if err := writer.Write(rowData); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
		slog.Debug("wrote repository data to CSV", slog.String("repository", repo.GetFullName()))
	}

	// Ensure all CSV data is written out
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	slog.Info("completed repository report", slog.String("filename", filename))
	return nil
}

func processRepository(ctx context.Context, count *int64, wg *sync.WaitGroup, in <-chan *RepoReport, out chan<- *RepoReport, restClient *github.Client) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			slog.Warn("context cancelled, stopping repository processing")
			return
		case repo, ok := <-in:
			if !ok {
				slog.Debug("no more repositories to process")
				return
			}
			teams, err := api.FetchTeams(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
			if err != nil {
				slog.Debug("failed to get teams", "repository", repo.GetFullName(), "error", err)
			}
			for _, team := range teams {
				externalGroups, err := api.FetchExternalGroups(ctx, restClient, repo.GetOwner().GetLogin(), team.GetSlug())
				if err != nil {
					slog.Debug("failed to get external groups", "repository", repo.GetFullName(), "error", err)
				}
				if externalGroups == nil {
					externalGroups = &github.ExternalGroupList{}
				}
				repoTeam := &repoTeam{
					Team:           team,
					ExternalGroups: externalGroups,
				}
				// Add the team to the repository
				repo.Teams = append(repo.Teams, repoTeam)
			}
			customProperties, err := api.FetchCustomProperties(ctx, restClient, repo.GetOwner().GetLogin(), repo.GetName())
			if err != nil {
				slog.Debug("failed to get custom properties", "repository", repo.GetFullName(), "error", err)
			}

			if customProperties == nil {
				customProperties = []*github.CustomPropertyValue{}
			}

			repo.CustomProperties = customProperties

			out <- repo
			atomic.AddInt64(count, 1)
			slog.Info("processing repository", "repository", repo.GetFullName())
		}
	}
}
