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

type UserReport struct {
	*github.User
	LastLogin time.Time
	Dormant   bool
}

func (u *UserReport) setEmail(email string) {
	u.Email = &email
}

func (u *UserReport) setLastLogin(lastLogin time.Time) {
	u.LastLogin = lastLogin
}

func (u *UserReport) setDormant(dormant bool) {
	u.Dormant = dormant
}

// UsersReport creates a CSV report containing enterprise user details, including email and dormant status.
func UsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug, filename string) error {
	slog.Info("starting users report", "filename", filename)

	// Create CSV file to write the report
	header := []string{
		"ID",
		"Login",
		"Name",
		"Email",
		"Last Login(90 days)",
		"Dormant?",
	}

	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	defer file.Close()

	// Define inactivity threshold for dormancy check
	const inactivityThreshold = 90 * 24 * time.Hour
	referenceTime := time.Now().UTC().Add(-inactivityThreshold)

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

	// Channels for user processing
	usersChan := make(chan *UserReport, 200)
	resultsCh := make(chan *UserReport, 200)

	// Process user
	var userWg sync.WaitGroup
	var userCount int64
	for i := 0; i < 20; i++ {
		userWg.Add(1)
		go processUser(ctx, &userCount, &userWg, usersChan, resultsCh, restClient, graphQLClient, referenceTime, userLogins, enterpriseSlug)
	}

	// Close resultsCh after all goroutines finish
	go func() {
		userWg.Wait()
		slog.Info("processing users complete", "total", atomic.LoadInt64(&userCount))
		close(resultsCh)
	}()

	// Enqueue user processing
	go func() {
		defer close(usersChan)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for _, user := range users {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				usersChan <- &UserReport{User: user}
			}
		}
	}()

	for user := range resultsCh {
		rowData := []string{
			fmt.Sprintf("%d", user.GetID()),
			user.GetLogin(),
			user.GetName(),
			user.GetEmail(),
			user.LastLogin.UTC().Format(time.RFC3339),
			fmt.Sprintf("%t", user.Dormant),
		}
		if err := writer.Write(rowData); err != nil {
			return fmt.Errorf("write CSV row to %q failed: %w", filename, err)
		}
	}

	// Ensure all CSV data is written out
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	slog.Info("completed users report", "filename", filename)
	return nil
}

// processUser processes a single user, fetching their email and checking for dormancy.
func processUser(ctx context.Context, count *int64, wg *sync.WaitGroup, in <-chan *UserReport, out chan<- *UserReport, restClient *github.Client, graphQLClient *githubv4.Client, referenceTime time.Time, userLogins map[string]time.Time, enterpriseSlug string) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			slog.Debug("context cancelled, stopping user processing")
			return
		case user, ok := <-in:
			if !ok {
				slog.Debug("no more users to process")
				return
			}

			slog.Info("processing user", "user", user.GetLogin())
			atomic.AddInt64(count, 1)

			email, err := api.FetchUserEmail(ctx, graphQLClient, enterpriseSlug, user.GetLogin())
			if err != nil {
				slog.Debug("fetching user email", "user", user.GetLogin(), "error", err)
				email = "N/A"
			}
			user.setEmail(email)

			// Last login & recent check
			var lastLogin time.Time
			var recent bool

			if last, ok := userLogins[user.GetLogin()]; ok {
				lastLogin = last
				recent = last.After(referenceTime)
			} else {
				slog.Debug("user not found in user logins", "user", user.GetLogin())
			}

			user.setLastLogin(lastLogin)

			// Dormancy
			dormant, err := isDormant(ctx, restClient, graphQLClient, user.GetLogin(), referenceTime, recent)
			if err != nil {
				slog.Debug("dormancy check failed", "user", user.GetLogin(), "err", err)
			}
			user.setDormant(dormant)

			out <- user

		}
	}
}
