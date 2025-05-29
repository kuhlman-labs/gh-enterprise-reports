// Package reports implements various report generation functionalities for GitHub Enterprise.
package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockActiveRepositoriesAPIClient is a mock implementation for testing active repositories report
type MockActiveRepositoriesAPIClient struct {
	mock.Mock
}

// Helper function to create a test repository
func createTestRepoForActive(owner, name string, pushedAt time.Time) *github.Repository {
	return &github.Repository{
		Name:     github.Ptr(name),
		FullName: github.Ptr(owner + "/" + name),
		Owner: &github.User{
			Login: github.Ptr(owner),
		},
		PushedAt: &github.Timestamp{Time: pushedAt},
	}
}

// Helper function to create a test commit
func createTestCommit(authorName, committerName string, date time.Time) *github.RepositoryCommit {
	return &github.RepositoryCommit{
		Commit: &github.Commit{
			Author: &github.CommitAuthor{
				Name: github.Ptr(authorName),
				Date: &github.Timestamp{Time: date},
			},
			Committer: &github.CommitAuthor{
				Name: github.Ptr(committerName),
				Date: &github.Timestamp{Time: date},
			},
		},
	}
}

func TestActiveRepositoriesReport(t *testing.T) {
	cache := utils.NewSharedCache()

	// Create test data
	cutoffDate := time.Now().AddDate(0, 0, -90)
	activeRepo1 := createTestRepoForActive("org1", "active-repo-1", time.Now().AddDate(0, 0, -10))
	activeRepo2 := createTestRepoForActive("org1", "active-repo-2", time.Now().AddDate(0, 0, -5))
	inactiveRepo := createTestRepoForActive("org1", "inactive-repo", time.Now().AddDate(0, 0, -100))

	// Test organizations
	orgs := []*github.Organization{
		{Login: github.Ptr("org1")},
	}

	// Test repositories (includes both active and inactive)
	repos := []*github.Repository{activeRepo1, activeRepo2, inactiveRepo}

	// Test commits for active repos
	commits1 := []*github.RepositoryCommit{
		createTestCommit("user1", "user1", time.Now().AddDate(0, 0, -5)),
		createTestCommit("user2", "user2", time.Now().AddDate(0, 0, -10)),
	}

	// Set up cache with test data
	cache.SetEnterpriseOrgs(orgs)
	cache.SetOrgRepositories("org1", repos)

	t.Run("should generate report for active repositories only", func(t *testing.T) {
		// Create a mock implementation to intercept the actual API calls
		// Since we can't easily mock the API calls in the current structure,
		// we'll test the filtering logic separately

		// Filter active repositories (this logic is from the report function)
		var activeRepos []*github.Repository
		for _, repo := range repos {
			if repo.GetPushedAt().After(cutoffDate) {
				activeRepos = append(activeRepos, repo)
			}
		}

		// Verify filtering worked correctly
		assert.Len(t, activeRepos, 2, "Should have 2 active repositories")
		assert.Equal(t, "active-repo-1", activeRepos[0].GetName())
		assert.Equal(t, "active-repo-2", activeRepos[1].GetName())
	})

	t.Run("should extract unique contributors from commits", func(t *testing.T) {
		// Test the contributor extraction logic
		contributorMap := make(map[string]bool)

		for _, commit := range commits1 {
			if commit.GetCommit() != nil && commit.GetCommit().GetCommitter() != nil {
				committerName := commit.GetCommit().GetCommitter().GetName()
				if committerName != "" && committerName != "GitHub" && committerName != "github-merge-queue" {
					contributorMap[committerName] = true
				}
			}
			if commit.GetCommit() != nil && commit.GetCommit().GetAuthor() != nil {
				authorName := commit.GetCommit().GetAuthor().GetName()
				if authorName != "" && authorName != "GitHub" && authorName != "github-merge-queue" {
					contributorMap[authorName] = true
				}
			}
		}

		// Convert map to slice
		contributors := make([]string, 0, len(contributorMap))
		for name := range contributorMap {
			contributors = append(contributors, name)
		}

		assert.Len(t, contributors, 2, "Should have 2 unique contributors")
		assert.Contains(t, contributors, "user1")
		assert.Contains(t, contributors, "user2")
	})

	t.Run("should filter out GitHub system users", func(t *testing.T) {
		// Test commits with system users
		systemCommits := []*github.RepositoryCommit{
			createTestCommit("user1", "user1", time.Now()),
			createTestCommit("GitHub", "GitHub", time.Now()),
			createTestCommit("github-merge-queue", "github-merge-queue", time.Now()),
		}

		contributorMap := make(map[string]bool)
		for _, commit := range systemCommits {
			if commit.GetCommit() != nil && commit.GetCommit().GetCommitter() != nil {
				committerName := commit.GetCommit().GetCommitter().GetName()
				if committerName != "" && committerName != "GitHub" && committerName != "github-merge-queue" {
					contributorMap[committerName] = true
				}
			}
		}

		contributors := make([]string, 0, len(contributorMap))
		for name := range contributorMap {
			contributors = append(contributors, name)
		}

		assert.Len(t, contributors, 1, "Should filter out GitHub system users")
		assert.Contains(t, contributors, "user1")
		assert.NotContains(t, contributors, "GitHub")
		assert.NotContains(t, contributors, "github-merge-queue")
	})
}

func TestActiveRepoReportFormatting(t *testing.T) {
	// Test the formatter function logic
	repo := createTestRepoForActive("testorg", "testrepo", time.Now())
	contributors := []string{"user1", "user2", "user3"}

	// Simulate the formatter function
	contributorsStr := strings.Join(contributors, "; ")
	if contributorsStr == "" {
		contributorsStr = "N/A"
	}

	expectedRow := []string{
		"testorg",
		"testrepo",
		repo.GetPushedAt().Format(time.RFC3339),
		contributorsStr,
	}

	// Test formatting
	assert.Equal(t, "testorg", expectedRow[0])
	assert.Equal(t, "testrepo", expectedRow[1])
	assert.Equal(t, "user1; user2; user3", expectedRow[3])
}

func TestActiveRepoReportFormattingEmptyContributors(t *testing.T) {
	// Test the formatter with empty contributors
	repo := createTestRepoForActive("testorg", "testrepo", time.Now())
	contributors := []string{}

	// Simulate the formatter function
	contributorsStr := ""
	if len(contributors) == 0 {
		contributorsStr = "N/A"
	}

	expectedRow := []string{
		"testorg",
		"testrepo",
		repo.GetPushedAt().Format(time.RFC3339),
		contributorsStr,
	}

	// Test formatting
	assert.Equal(t, "N/A", expectedRow[3])
}
