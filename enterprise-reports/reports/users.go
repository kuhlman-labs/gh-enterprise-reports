package reports

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

// UsersReport creates a CSV report containing enterprise user details, including email and dormant status.
func UsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting users report", "filename", filename)

	// Create CSV file to write the report
	header := []string{
		"ID",
		"Login",
		"Name",
		"Email",
		"Last Login",
		"Dormant?",
	}

	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	defer file.Close()

	// Define inactivity threshold (e.g., 90 days).
	referenceTime := time.Now().UTC().AddDate(0, 0, -90) // Ensure UTC

	// Fetch user logins
	userLogins, err := api.FetchUserLogins(ctx, restClient, enterpriseSlug, referenceTime)
	if err != nil {
		return fmt.Errorf("fetching user logins for enterprise %q: %w", enterpriseSlug, err)
	}

	// Fetch enterprise users
	users, err := api.FetchEnterpriseUsers(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("fetching enterprise users for %q: %w", enterpriseSlug, err)
	}

	// Concurrency setup
	resultsCh := make(chan []string, len(users))
	semaphore := make(chan struct{}, 10) // Limit concurrent requests to 10
	var wg sync.WaitGroup
	var userCount int64

	for _, u := range users {
		wg.Add(1)
		go func(u github.User) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore token
			defer func() { <-semaphore }() // Release token

			slog.Debug("processing user", "user", u.GetLogin())

			email, err := api.FetchUserEmail(ctx, graphQLClient, enterpriseSlug, u.GetLogin())
			if err != nil {
				slog.Warn("fetching user email", "user", u.GetLogin(), "error", err)
				email = "N/A"
			}

			lastLoginStr := "N/A"
			if t, ok := userLogins[u.GetLogin()]; ok {
				lastLoginStr = t.UTC().Format(time.RFC3339)
			}

			recentLogin := false
			if t, ok := userLogins[u.GetLogin()]; ok && t.After(referenceTime) {
				recentLogin = true
			}

			dormant, err := isDormant(ctx, restClient, graphQLClient, u.GetLogin(), referenceTime, recentLogin)
			if err != nil {
				slog.Warn("determining dormant status", "user", u.GetLogin(), "error", err)
				dormant = false
			}
			dormantStr := "No"
			if dormant {
				dormantStr = "Yes"
			}

			row := []string{
				fmt.Sprintf("%d", u.GetID()),
				u.GetLogin(),
				u.GetName(),
				email,
				lastLoginStr,
				dormantStr,
			}
			atomic.AddInt64(&userCount, 1)
			slog.Info("processing user", "user", u.GetLogin())
			resultsCh <- row
		}(*u)
	}

	// Close resultsCh after all goroutines finish
	go func() {
		wg.Wait()
		slog.Info("processing users complete", "total", atomic.LoadInt64(&userCount))
		close(resultsCh)
	}()

	// Write CSV rows sequentially
	for row := range resultsCh {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("runUsersReport: write CSV row to %q failed: %w", filename, err)
		}
	}

	// Ensure all CSV data is written out
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	slog.Info("users report generated", "filename", filename)
	return nil
}
