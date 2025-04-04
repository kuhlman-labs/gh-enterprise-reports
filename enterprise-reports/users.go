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
}

// getEnterpriseUsers queries the GraphQL API and returns a slice of EnterpriseUser.
// It paginates until all the nodes are retrieved.
func getEnterpriseUsers(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, slug string) ([]EnterpriseUser, error) {
	log.Info().Str("Enterprise", slug).Msg("Fetching enterprise cloud users.")

	var allUsers []EnterpriseUser
	var cursor *githubv4.String

	for {
		// Ensure rate limits before GraphQL call.
		EnsureRateLimits(ctx, restClient)

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

// hasRecentEvents checks if the user has any Public events more recent than the provided time.
func hasRecentEvents(ctx context.Context, client *github.Client, user string, since time.Time) (bool, error) {
	// Ensure rate limits before REST call.
	EnsureRateLimits(ctx, client)

	events, _, err := client.Activity.ListEventsPerformedByUser(ctx, user, false, nil)
	if err != nil {
		return false, err
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

// hasRecentContributions checks if the user has any contributions since the provided time.
func hasRecentContributions(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, user string, since time.Time) (bool, error) {
	log.Debug().Str("User", user).Msgf("Checking recent contributions since %s", since)

	// Ensure rate limits before GraphQL call.
	EnsureRateLimits(ctx, restClient)

	var query struct {
		User struct {
			ContributionsCollection struct {
				TotalCommitContributions            int
				TotalIssueContributions             int
				TotalPullRequestContributions       int
				TotalPullRequestReviewContributions int
			} `graphql:"contributionsCollection(from: $since)"`
		} `graphql:"user(login: $login)"`
	}

	vars := map[string]interface{}{
		"login": githubv4.String(user),
		"since": githubv4.DateTime{Time: since},
	}

	if err := graphQLClient.Query(ctx, &query, vars); err != nil {
		return false, err
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

// IsDormantUser returns true if the user shows no recent events or contributions
// since the specified time.
func isDormant(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, user string, since time.Time, recentLogin bool) (bool, error) {
	log.Debug().Str("User", user).Msg("Checking dormant status for user.")

	// Check for recent REST events.
	recentEvents, err := hasRecentEvents(ctx, restClient, user, since)
	if err != nil {
		return false, fmt.Errorf("error checking recent events for user %s: %w", user, err)
	}

	// Check for recent contributions.
	recentContribs, err := hasRecentContributions(ctx, restClient, graphQLClient, user, since)
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

// getUserLogins fetches all audit log events for user.login for the past 90 days.
func getUserLogins(ctx context.Context, client *github.Client, enterpriseSlug string) (map[string]time.Time, error) {
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
		// Ensure rate limits before REST call.
		EnsureRateLimits(ctx, client)

		// Fetch audit logs with pagination.
		auditLogs, resp, err := client.Enterprise.GetAuditLog(ctx, enterpriseSlug, opts)
		if err != nil {
			log.Error().Str("Enterprise", enterpriseSlug).Err(err).Msg("Failed to fetch audit logs.")
			return nil, fmt.Errorf("failed to query audit logs: %w", err)
		}

		// Log added after fetching a page of audit logs.
		log.Debug().Int("PageAuditLogCount", len(auditLogs)).
			Str("AfterCursor", resp.After).
			Msg("Fetched a page of audit logs.")

		allAuditLogs = append(allAuditLogs, auditLogs...)

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

// getUserEmail queries the enterprise GraphQL API for the user's email.
func getUserEmail(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, slug string, user string) (string, error) {
	log.Debug().Str("User", user).Msg("Fetching email for user.")

	// Ensure rate limits before GraphQL call.
	EnsureRateLimits(ctx, restClient)

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
	}
	variables := map[string]interface{}{
		"slug":  githubv4.String(slug),
		"login": githubv4.String(user),
	}
	if err := graphQLClient.Query(ctx, &query, variables); err != nil {
		log.Error().Str("Enterprise", slug).Str("User", user).Err(err).Msg("Failed to fetch email for user.")
		return "", fmt.Errorf("failed to query external identities: %w", err)
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

// runUsersReport creates a CSV report with columns: ID, Login, Name, Email, Last Login, Dormant?
func runUsersReport(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, enterpriseSlug string, filename string) error {
	log.Info().Str("Filename", filename).Msg("Creating users report.")

	// Open or create the CSV file.
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			log.Error().Err(err).Msg("Error flushing CSV writer.")
		}
	}()

	// Write header row.
	header := []string{"ID", "Login", "Name", "Email", "Last Login", "Dormant?"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Define inactivity threshold (e.g., 90 days).
	referenceTime := time.Now().UTC().AddDate(0, 0, -90) // Ensure UTC

	userLogins, err := getUserLogins(ctx, restClient, enterpriseSlug)
	if err != nil {
		log.Warn().Err(err).Msg("Could not retrieve user login events.")
		userLogins = make(map[string]time.Time)
	}

	users, err := getEnterpriseUsers(ctx, restClient, graphQLClient, enterpriseSlug)
	if err != nil {
		return fmt.Errorf("error fetching users: %w", err)
	}

	// Use a buffered channel to collect rows for CSV writing.
	rowsCh := make(chan []string, len(users))
	semaphore := make(chan struct{}, 10) // limit concurrent requests to 10
	var wg sync.WaitGroup

	for _, u := range users {
		wg.Add(1)
		go func(u EnterpriseUser) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire semaphore token
			defer func() { <-semaphore }() // release token

			log.Debug().Str("User", u.Login).Msg("Processing user.")

			email, err := getUserEmail(ctx, restClient, graphQLClient, enterpriseSlug, u.Login)
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

			// Log processed user details.
			log.Debug().Str("User", u.Login).
				Str("Email", email).
				Str("LastLogin", lastLoginStr).
				Bool("Dormant", dormant).
				Msg("User processed.")

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

	// Close rowsCh after all goroutines finish.
	go func() {
		wg.Wait()
		close(rowsCh)
	}()

	// Write CSV rows sequentially.
	for row := range rowsCh {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row for user: %w", err)
		}
	}

	log.Info().Str("Filename", filename).Msg("Users report completed successfully.")
	return nil
}
