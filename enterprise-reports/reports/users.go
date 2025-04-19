package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
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
	// Standardize context timeout and logging for report generation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	slog.Info("starting users report", "filename", filename)

	// Create and open the CSV file
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating report file %q: %w", filename, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Error("closing csv file", "error", err)
		}
	}()

	writer := csv.NewWriter(f)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			slog.Warn("flushing csv writer", "error", err)
		}
	}()

	// Write header row
	header := []string{"ID", "Login", "Name", "Email", "Last Login", "Dormant?"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("writing header to %q: %w", filename, err)
	}

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

	slog.Info("users report generated", "filename", filename)
	return nil
}

// isDormant determines if a user is dormant by verifying events, contributions, and recent login activity.
func isDormant(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, user string, since time.Time, recentLogin bool) (bool, error) {
	slog.Debug("checking dormant status", "user", user)

	// Check for recent REST events.
	recentEvents, err := api.HasRecentEvents(ctx, restClient, user, since)
	if err != nil {
		return false, fmt.Errorf("checking recent events for %q: %w", user, err)
	}

	// Check for recent contributions.
	recentContribs, err := api.HasRecentContributions(ctx, graphQLClient, user, since)
	if err != nil {
		return false, fmt.Errorf("checking recent contributions for %q: %w", user, err)
	}

	// If the user has neither recent events nor contributions, the user is dormant.
	dormant := !(recentEvents || recentContribs || recentLogin)

	// report final dormant check outcome.
	slog.Debug("dormant check result",
		"user", user,
		"recentEvents", recentEvents,
		"recentContribs", recentContribs,
		"recentLogin", recentLogin,
		"dormant", dormant,
	)

	return dormant, nil
}
