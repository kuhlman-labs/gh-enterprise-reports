// Package reports implements various report generation functionalities for GitHub Enterprise.
// It provides utilities and specific report types for organizations, repositories, teams,
// collaborators, and user data, with results exported as CSV files.
package reports

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// UserReport contains user information along with additional metadata
// about their activity status, including their last login time and dormancy flag.
type UserReport struct {
	*github.User
	LastLogin time.Time // Last login time within the last 90 days
	Dormant   bool      // Whether the user is considered dormant
}

// UsersReport creates a CSV report containing enterprise user details, including email and dormant status.
// It fetches all enterprise users, their email addresses, last login times, and determines dormancy
// based on login activity, contributions, and events within the inactivity threshold (90 days).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - restClient: GitHub REST API client
//   - graphQLClient: GitHub GraphQL API client
//   - enterpriseSlug: Enterprise identifier
//   - filename: Output CSV file path
//   - workerCount: Number of concurrent workers for processing users
//
// The report includes user ID, login name, display name, email address, last login time,
// and dormancy status.
func UsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string, workerCount int, cache *utils.SharedCache) error {
	slog.Info("starting users report", "enterprise", enterpriseSlug, "filename", filename, "workers", workerCount)

	// Create appropriate report writer based on file extension
	reportWriter, reportErr := NewReportWriter(filename)
	if reportErr != nil {
		return reportErr
	}
	defer func() {
		if err := reportWriter.Close(); err != nil {
			slog.Error("Failed to close report writer", "error", err)
		}
	}()

	header := []string{
		"ID",
		"Login",
		"Name",
		"Email",
		"Last Login(90 days)",
		"Dormant?",
	}

	// Write header to report
	if headerErr := reportWriter.WriteHeader(header); headerErr != nil {
		return fmt.Errorf("failed to write header: %w", headerErr)
	}

	// Inactivity threshold and fetch user logins
	const inactivityThreshold = 90 * 24 * time.Hour
	referenceTime := time.Now().UTC().Add(-inactivityThreshold)

	// Fetch user logins
	slog.Info("fetching user logins", "enterprise", enterpriseSlug)
	userLogins, err := api.FetchUserLogins(ctx, restClient, enterpriseSlug, referenceTime)
	if err != nil {
		return fmt.Errorf("fetching user logins for enterprise %q: %w", enterpriseSlug, err)
	}

	// Check cache for enterprise users or fetch from API
	var users []*github.User

	if cachedUsers, found := cache.GetEnterpriseUsers(); found {
		slog.Info("using cached enterprise users")
		users = cachedUsers
	} else {
		// Fetch enterprise users
		slog.Info("fetching enterprise users", "enterprise", enterpriseSlug)
		users, err = api.FetchEnterpriseUsers(ctx, graphQLClient, enterpriseSlug)
		if err != nil {
			return fmt.Errorf("fetching enterprise users for %q: %w", enterpriseSlug, err)
		}
		// Store in cache
		cache.SetEnterpriseUsers(users)
	}

	// Processor: fetch email, last login, and dormancy
	processor := func(ctx context.Context, u *github.User) (*UserReport, error) {
		// Email
		slog.Info("processing user", "login", u.GetLogin())
		email, err := api.FetchUserEmail(ctx, graphQLClient, enterpriseSlug, u.GetLogin())
		if err != nil {
			slog.Debug("failed to fetch email", "user", u.GetLogin(), "error", err)
			// Continue without email
			email = ""
		}
		// Last login
		lastLogin := userLogins[u.GetLogin()]
		recentLogin := lastLogin.After(referenceTime)

		// Recent activity checks
		recentEvents, err := api.HasRecentEvents(ctx, restClient, u.GetLogin(), referenceTime)
		if err != nil {
			slog.Debug("failed to fetch recent events", "user", u.GetLogin(), "error", err)
			recentEvents = false // Default to false if error occurs
		}
		recentContributions, err := api.HasRecentContributions(ctx, graphQLClient, u.GetLogin(), referenceTime)
		if err != nil {
			slog.Debug("failed to fetch recent contributions", "user", u.GetLogin(), "error", err)
			recentContributions = false // Default to false if error occurs
		}

		// Dormancy check
		dormant, err := utils.IsDormant(u.GetLogin(), recentEvents, recentContributions, recentLogin)
		if err != nil {
			slog.Debug("dormancy check failed", "user", u.GetLogin(), "error", err)
			dormant = false // Default to false if error occurs
		}
		u.Email = &email // Set email directly on the User struct
		report := &UserReport{
			User:      u,
			LastLogin: lastLogin,
			Dormant:   dormant,
		}
		return report, nil
	}

	// Formatter: build CSV row
	formatter := func(r *UserReport) []string {
		// Use GetEmail() which accesses the embedded User's email field
		emailStr := r.GetEmail()
		if emailStr == "" {
			emailStr = "N/A" // Ensure N/A if email is empty/nil
		}
		return []string{
			fmt.Sprintf("%d", r.GetID()),
			r.GetLogin(),
			r.GetName(),
			emailStr,
			r.LastLogin.UTC().Format(time.RFC3339),
			fmt.Sprintf("%t", r.Dormant),
		}
	}

	// Create a limiter for rate limiting - aiming for ~10 users/sec
	// (10 REST points/sec < 15, 20 GQL points/sec < 33)
	// Burst matches worker count for responsiveness.
	limiter := rate.NewLimiter(rate.Limit(10), workerCount) // e.g., 10 requests/sec, burst of workerCount

	// Run the report using the new report writer interface
	return RunReportWithWriter(ctx, users, processor, formatter, limiter, workerCount, reportWriter)
}
