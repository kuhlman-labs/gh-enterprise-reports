package reports

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

type UserReport struct {
	*github.User
	LastLogin time.Time
	Dormant   bool
}

// UsersReport creates a CSV report containing enterprise user details, including email and dormant status.
func UsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string, workerCount int) error {
	// Validate output path early to catch file creation errors before API calls
	if err := validateFilePath(filename); err != nil {
		return err
	}
	slog.Info("starting users report", "enterprise", enterpriseSlug, "filename", filename, "workers", workerCount)
	header := []string{
		"ID",
		"Login",
		"Name",
		"Email",
		"Last Login(90 days)",
		"Dormant?",
	}

	// Inactivity threshold and fetch user logins
	const inactivityThreshold = 90 * 24 * time.Hour
	referenceTime := time.Now().UTC().Add(-inactivityThreshold)
	userLogins, err := api.FetchUserLogins(ctx, restClient, enterpriseSlug, referenceTime)
	if err != nil {
		return fmt.Errorf("fetching user logins for enterprise %q: %w", enterpriseSlug, err)
	}

	// Fetch enterprise users
	users, err := api.FetchEnterpriseUsers(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("fetching enterprise users for %q: %w", enterpriseSlug, err)
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
		recent := lastLogin.After(referenceTime)
		// Dormant
		dormant, err := isDormant(ctx, restClient, graphQLClient, u.GetLogin(), referenceTime, recent)
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

	return RunReport(ctx, users, processor, formatter, limiter, workerCount, filename, header)
}
