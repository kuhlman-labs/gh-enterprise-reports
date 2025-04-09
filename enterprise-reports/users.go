package enterprisereports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
)

// EnterpriseUser represents an enterprise user account.
type EnterpriseUser struct {
	ID        githubv4.ID
	Login     string
	Name      string
	CreatedAt time.Time
}

// enterpriseUsersQuery defines the GraphQL query structure with pagination support.
type enterpriseUsersQuery struct {
	Enterprise struct {
		Members struct {
			Nodes []struct {
				// Inline fragment to extract fields only available on EnterpriseUserAccount.
				EnterpriseUserAccount EnterpriseUser `graphql:"... on EnterpriseUserAccount"`
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   githubv4.String
			}
		} `graphql:"members(first: 100, deployment: CLOUD, after: $cursor)"`
	} `graphql:"enterprise(slug: $enterpriseSlug)"`
	RateLimit struct {
		Cost      int
		Limit     int
		Remaining int
		ResetAt   githubv4.DateTime
	}
}

// getEnterpriseUsers retrieves all enterprise users via the GraphQL API with pagination support.
func getEnterpriseUsers(ctx context.Context, graphQLClient *githubv4.Client, slug string) ([]EnterpriseUser, error) {
	log.Info().Str("Enterprise", slug).Msg("Fetching enterprise cloud users.")

	var allUsers []EnterpriseUser
	var cursor *githubv4.String

	for {
		var query enterpriseUsersQuery
		variables := map[string]interface{}{
			"enterpriseSlug": githubv4.String(slug),
			"cursor":         (*githubv4.String)(cursor), // nil for first request.
		}

		if err := graphQLClient.Query(ctx, &query, variables); err != nil {
			return nil, fmt.Errorf("failed to fetch enterprise cloud users: %w", err)
		}

		// Log the number of users fetched in this page, and pagination info.
		log.Debug().Int("PageUsers", len(query.Enterprise.Members.Nodes)).
			Bool("HasNextPage", query.Enterprise.Members.PageInfo.HasNextPage).
			Str("EndCursor", string(query.Enterprise.Members.PageInfo.EndCursor)).
			Msg("Fetched a page of enterprise users.")

		// Append current page of users.
		for _, node := range query.Enterprise.Members.Nodes {
			allUsers = append(allUsers, node.EnterpriseUserAccount)
		}

		// Check for rate limits.
		if query.RateLimit.Remaining < GraphQLRateLimitThreshold {
			log.Warn().Int("Remaining", query.RateLimit.Remaining).Int("Limit", query.RateLimit.Limit).Msg("Rate limit low")
			waitForLimitReset(ctx, "GraphQL", query.RateLimit.Remaining, query.RateLimit.Limit, query.RateLimit.ResetAt.Time)
		}

		// If there is no next page, break out.
		if !query.Enterprise.Members.PageInfo.HasNextPage {
			break
		}

		// Update cursor to the end cursor of the current page.
		cursor = &query.Enterprise.Members.PageInfo.EndCursor
	}

	log.Info().Int("Users", len(allUsers)).Msg("Successfully fetched enterprise cloud users.")
	return allUsers, nil
}

// hasRecentEvents determines whether a user has any recent public events after the specified time.
func hasRecentEvents(ctx context.Context, restClient *github.Client, user string, since time.Time) (bool, error) {

	events, resp, err := restClient.Activity.ListEventsPerformedByUser(ctx, user, false, nil)
	if err != nil {
		return false, err
	}

	// Check rate limits after fetching events.
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	for _, event := range events {
		if event.CreatedAt != nil && event.CreatedAt.After(since) {
			log.Debug().Str("User", user).
				Str("Event Type", *event.Type).
				Time("Event Time", event.CreatedAt.Time).
				Msg("Detected recent activity.")
			return true, nil
		}
	}
	return false, nil
}

// hasRecentContributions checks if a user has any contributions since the provided time.
func hasRecentContributions(ctx context.Context, graphQLClient *githubv4.Client, user string, since time.Time) (bool, error) {
	log.Debug().Str("User", user).Msgf("Checking recent contributions since %s", since)

	var query struct {
		User struct {
			ContributionsCollection struct {
				TotalCommitContributions            int
				TotalIssueContributions             int
				TotalPullRequestContributions       int
				TotalPullRequestReviewContributions int
			} `graphql:"contributionsCollection(from: $since)"`
		} `graphql:"user(login: $login)"`
		RateLimit struct {
			Cost      int
			Limit     int
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}

	vars := map[string]interface{}{
		"login": githubv4.String(user),
		"since": githubv4.DateTime{Time: since},
	}

	if err := graphQLClient.Query(ctx, &query, vars); err != nil {
		return false, err
	}

	// Check for rate limits.
	if query.RateLimit.Remaining < GraphQLRateLimitThreshold {
		log.Warn().Int("Remaining", query.RateLimit.Remaining).Int("Limit", query.RateLimit.Limit).Msg("Rate limit low")
		waitForLimitReset(ctx, "GraphQL", query.RateLimit.Remaining, query.RateLimit.Limit, query.RateLimit.ResetAt.Time)
	}

	contrib := query.User.ContributionsCollection
	total := contrib.TotalCommitContributions +
		contrib.TotalIssueContributions +
		contrib.TotalPullRequestContributions +
		contrib.TotalPullRequestReviewContributions

	// check for contributions
	if total > 0 {
		log.Debug().Str("User", user).Int("Total Contributions", total).Msg("Detected contributions for user.")
	}

	return total > 0, nil
}

// isDormant determines if a user is dormant by verifying events, contributions, and recent login activity.
func isDormant(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, user string, since time.Time, recentLogin bool) (bool, error) {
	log.Debug().Str("User", user).Msg("Checking dormant status for user.")

	// Check for recent REST events.
	recentEvents, err := hasRecentEvents(ctx, restClient, user, since)
	if err != nil {
		return false, fmt.Errorf("error checking recent events for user %s: %w", user, err)
	}

	// Check for recent contributions.
	recentContribs, err := hasRecentContributions(ctx, graphQLClient, user, since)
	if err != nil {
		return false, fmt.Errorf("error checking recent contributions for user %s: %w", user, err)
	}

	// If the user has neither recent events nor contributions, the user is dormant.
	dormant := !(recentEvents || recentContribs || recentLogin)

	// report final dormant check outcome.
	log.Debug().Str("User", user).
		Bool("RecentEvents", recentEvents).
		Bool("RecentContribs", recentContribs).
		Bool("RecentLogin", recentLogin).
		Bool("Dormant", dormant).
		Msg("Dormant check result.")

	return dormant, nil
}

// getUserLogins retrieves audit log events for user login actions over the past 90 days and returns a mapping of login to the most recent login time.
func getUserLogins(ctx context.Context, restClient *github.Client, enterpriseSlug string) (map[string]time.Time, error) {
	log.Info().Str("Enterprise", enterpriseSlug).Msg("Fetching audit logs for enterprise.")

	// Build query phrase for user.login events.
	phrase := "action:user.login"
	opts := &github.GetAuditLogOptions{
		Phrase: &phrase,
		ListCursorOptions: github.ListCursorOptions{
			After:   "",
			PerPage: 100,
		},
	}

	var allAuditLogs []*github.AuditEntry

	for {

		// Fetch audit logs with pagination.
		auditLogs, resp, err := restClient.Enterprise.GetAuditLog(ctx, enterpriseSlug, opts)
		if err != nil {
			log.Error().Str("Enterprise", enterpriseSlug).Err(err).Msg("Failed to fetch audit logs.")
			return nil, fmt.Errorf("failed to query audit logs: %w", err)
		}

		// Log added after fetching a page of audit logs.
		log.Debug().Int("PageAuditLogCount", len(auditLogs)).
			Str("AfterCursor", resp.After).
			Msg("Fetched a page of audit logs.")

		allAuditLogs = append(allAuditLogs, auditLogs...)

		// Check rate limits after fetching a page of audit logs.
		if resp.Rate.Remaining < AuditLogRateLimitThreshold {
			log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low")
			waitForLimitReset(ctx, "Audit Log", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
		}

		if resp.After == "" {
			break
		}

		// Update the cursor for the next page.
		opts.ListCursorOptions.After = resp.After

	}

	log.Info().Int("Log Count", len(allAuditLogs)).Msg("Successfully fetched audit logs.")

	loginMap := make(map[string]time.Time)
	for _, logEntry := range allAuditLogs {
		// Ensure both Actor and CreatedAt are non-nil.
		if logEntry.Actor == nil || logEntry.CreatedAt == nil {
			continue
		}
		actor := *logEntry.Actor
		eventTime := logEntry.CreatedAt.Time.UTC() // Ensure UTC
		// Store the latest event per user.
		if existing, found := loginMap[actor]; !found || eventTime.After(existing) {
			loginMap[actor] = eventTime
		}
	}

	log.Info().Int("Unique User Logins", len(loginMap)).Msg("Successfully mapped audit logs to user logins.")
	return loginMap, nil
}

// getUserEmail queries the enterprise GraphQL API to retrieve the email address for the specified user.
func getUserEmail(ctx context.Context, graphQLClient *githubv4.Client, slug string, user string) (string, error) {
	log.Debug().Str("User", user).Msg("Fetching email for user.")

	var query struct {
		Enterprise struct {
			OwnerInfo struct {
				SamlIdentityProvider struct {
					ExternalIdentities struct {
						Nodes []struct {
							User struct {
								Login githubv4.String
							}
							ScimIdentity struct {
								Username githubv4.String
								Emails   []struct {
									Value githubv4.String
								}
							}
							SamlIdentity struct {
								Username githubv4.String
								Emails   []struct {
									Value githubv4.String
								}
							}
						}
					} `graphql:"externalIdentities(first: 1, login: $login)"`
				}
			}
		} `graphql:"enterprise(slug: $slug)"`
		RateLimit struct {
			Cost      int
			Limit     int
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}
	variables := map[string]interface{}{
		"slug":  githubv4.String(slug),
		"login": githubv4.String(user),
	}
	if err := graphQLClient.Query(ctx, &query, variables); err != nil {
		log.Error().Str("Enterprise", slug).Str("User", user).Err(err).Msg("Failed to fetch email for user.")
		return "", fmt.Errorf("failed to query external identities: %w", err)
	}

	// Check for rate limits.
	if query.RateLimit.Remaining < GraphQLRateLimitThreshold {
		log.Warn().Int("Remaining", query.RateLimit.Remaining).Int("Limit", query.RateLimit.Limit).Msg("Rate limit low")
		waitForLimitReset(ctx, "GraphQL", query.RateLimit.Remaining, query.RateLimit.Limit, query.RateLimit.ResetAt.Time)
	}

	for _, node := range query.Enterprise.OwnerInfo.SamlIdentityProvider.ExternalIdentities.Nodes {
		if string(node.User.Login) == user {
			// Prefer SamlIdentity emails over ScimIdentity.
			if len(node.SamlIdentity.Emails) > 0 {
				return string(node.SamlIdentity.Emails[0].Value), nil
			}
			if len(node.ScimIdentity.Emails) > 0 {
				return string(node.ScimIdentity.Emails[0].Value), nil
			}
		}
	}
	return "N/A", nil
}

// runUsersReport creates a CSV report containing enterprise user details, including email and dormant status.
func runUsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug string, filename string) error {
	// Standardize context timeout and logging for report generation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	log.Info().Str("Filename", filename).Msg("Starting users report generation.")

	// Create and open the CSV file
	f, err := os.Create(filename)
	if err != nil {
		log.Error().Err(err).Str("Filename", filename).Msg("Failed to create report file.")
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Error().Err(err).Str("Filename", filename).Msg("Failed to close report file.")
		}
	}()

	writer := csv.NewWriter(f)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			log.Error().Err(err).Msg("Error flushing CSV writer.")
		}
	}()

	// Write header row
	header := []string{"ID", "Login", "Name", "Email", "Last Login", "Dormant?"}
	if err := writer.Write(header); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header.")
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Define inactivity threshold (e.g., 90 days).
	referenceTime := time.Now().UTC().AddDate(0, 0, -90) // Ensure UTC

	// Fetch user logins
	userLogins, err := getUserLogins(ctx, restClient, enterpriseSlug)
	if err != nil {
		log.Warn().Err(err).Msg("Could not retrieve user login events.")
		userLogins = make(map[string]time.Time)
	}

	// Fetch enterprise users
	users, err := getEnterpriseUsers(ctx, graphQLClient, enterpriseSlug)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch enterprise users.")
		return fmt.Errorf("error fetching users: %w", err)
	}

	// Concurrency setup
	rowsCh := make(chan []string, len(users))
	semaphore := make(chan struct{}, 10) // Limit concurrent requests to 10
	var wg sync.WaitGroup

	for _, u := range users {
		wg.Add(1)
		go func(u EnterpriseUser) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore token
			defer func() { <-semaphore }() // Release token

			log.Debug().Str("User", u.Login).Msg("Processing user.")

			email, err := getUserEmail(ctx, graphQLClient, enterpriseSlug, u.Login)
			if err != nil {
				log.Warn().Str("User", u.Login).Err(err).Msg("Could not retrieve email for user.")
				email = "N/A"
			}

			lastLoginStr := "N/A"
			if t, ok := userLogins[u.Login]; ok {
				lastLoginStr = t.UTC().Format(time.RFC3339) // Ensure UTC
			}

			recentLogin := false
			if t, ok := userLogins[u.Login]; ok && t.After(referenceTime) {
				recentLogin = true
			}

			dormant, err := isDormant(ctx, restClient, graphQLClient, u.Login, referenceTime, recentLogin)
			if err != nil {
				log.Warn().Str("User", u.Login).Err(err).Msg("Error determining dormant status for user.")
				dormant = false
			}
			dormantStr := "No"
			if dormant {
				dormantStr = "Yes"
			}

			row := []string{
				fmt.Sprintf("%v", u.ID),
				u.Login,
				u.Name,
				email,
				lastLoginStr,
				dormantStr,
			}
			rowsCh <- row
		}(u)
	}

	// Close rowsCh after all goroutines finish
	go func() {
		wg.Wait()
		close(rowsCh)
	}()

	// Write CSV rows sequentially
	for row := range rowsCh {
		if err := writer.Write(row); err != nil {
			log.Error().Err(err).Msg("Failed to write row to CSV.")
			return fmt.Errorf("failed to write row for user: %w", err)
		}
	}

	log.Info().Str("Filename", filename).Msg("Users report generated successfully.")
	return nil
}
