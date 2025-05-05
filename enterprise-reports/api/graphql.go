// Package api provides functionality for interacting with GitHub's REST and GraphQL APIs.
// It includes rate limiting, client wrapper methods, and utilities for efficient API consumption.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
)

// rateLimitQuery is a struct to hold the rate limit information.
type rateLimitQuery struct {
	Cost      int
	Limit     int
	Remaining int
	ResetAt   githubv4.DateTime
}

// FetchOrganizationMembershipsWithRole fetches all organization memberships with roles
// in the given organization using the GraphQL API with pagination.
// It returns a list of users with their login, name, database ID, and role in the organization.
func FetchOrganizationMembershipsWithRole(ctx context.Context, graphQLClient *githubv4.Client, orgLogin string) ([]*github.User, error) {
	slog.Debug("fetching organization memberships", "organization", orgLogin)
	// Define the GraphQL query to fetch organization memberships.
	// The query fetches the first 100 members with their roles and uses pagination to retrieve all members.
	// It also checks for rate limits after each request.
	var query struct {
		Organization struct {
			MembersWithRole struct {
				Edges []struct {
					Role string
					Node struct {
						Name       string
						Login      string
						DatabaseID int64
					}
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				}
			} `graphql:"membersWithRole(first: 100, after: $cursor)"`
		} `graphql:"organization(login: $login)"`
		RateLimit rateLimitQuery
	}
	variables := map[string]interface{}{
		"login":  githubv4.String(orgLogin),
		"cursor": (*githubv4.String)(nil),
	}

	memberMap := make(map[string]*github.User)
	for {
		err := graphQLClient.Query(ctx, &query, variables)
		if err != nil {
			return nil, fmt.Errorf("query organization memberships for %q failed: %w", orgLogin, err)
		}
		for _, edge := range query.Organization.MembersWithRole.Edges {
			memberMap[edge.Node.Login] = &github.User{
				Login:    &edge.Node.Login,
				Name:     &edge.Node.Name,
				ID:       &edge.Node.DatabaseID,
				RoleName: &edge.Role,
			}
		}
		// Check rate limit
		handleGraphQLRateLimit(ctx, &query.RateLimit)

		// Check if there are more pages
		if !query.Organization.MembersWithRole.PageInfo.HasNextPage {
			break
		}
		variables["cursor"] = query.Organization.MembersWithRole.PageInfo.EndCursor
	}
	allMemberships := make([]*github.User, 0, len(memberMap))
	for _, member := range memberMap {
		allMemberships = append(allMemberships, member)
	}
	slog.Debug("fetched all memberships",
		"organization", orgLogin,
		"totalMemberships", len(allMemberships),
	)
	return allMemberships, nil
}

// FetchEnterpriseUsers retrieves all enterprise cloud users via the GraphQL API.
// It uses pagination to fetch all users associated with the specified enterprise slug
// and includes their login, name, database ID, and creation date.
func FetchEnterpriseUsers(ctx context.Context, graphQLClient *githubv4.Client, enterpriseSlug string) ([]*github.User, error) {
	slog.Debug("fetching enterprise users", "enterprise", enterpriseSlug)
	// Define the GraphQL query to fetch enterprise users.
	// The query fetches the first 100 Cloud members and uses pagination to retrieve all users.
	// It also checks for rate limits after each request.
	var query struct {
		Enterprise struct {
			Members struct {
				Nodes []struct {
					EnterpriseUserAccount struct {
						Login     string
						Name      string
						CreatedAt github.Timestamp
						User      struct {
							DatabaseID int64
						}
					} `graphql:"... on EnterpriseUserAccount"`
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				}
			} `graphql:"members(first: 100, deployment: CLOUD, after: $cursor)"`
		} `graphql:"enterprise(slug: $enterpriseSlug)"`
		RateLimit rateLimitQuery
	}

	var allUsers []*github.User

	variables := map[string]interface{}{
		"enterpriseSlug": githubv4.String(enterpriseSlug),
		"cursor":         (*githubv4.String)(nil), // nil for first request.
	}

	for {
		err := graphQLClient.Query(ctx, &query, variables)
		if err != nil {
			return nil, fmt.Errorf("query enterprise users for %q failed: %w", enterpriseSlug, err)
		}

		// Log the number of users fetched in this page, and pagination info.
		slog.Debug("fetched enterprise users",
			"pageSize", len(query.Enterprise.Members.Nodes),
			"hasNextPage", query.Enterprise.Members.PageInfo.HasNextPage,
			"endCursor", query.Enterprise.Members.PageInfo.EndCursor,
		)

		// Append current page of users.
		for _, node := range query.Enterprise.Members.Nodes {
			allUsers = append(allUsers, &github.User{
				Login:     &node.EnterpriseUserAccount.Login,
				Name:      &node.EnterpriseUserAccount.Name,
				ID:        &node.EnterpriseUserAccount.User.DatabaseID,
				CreatedAt: &node.EnterpriseUserAccount.CreatedAt,
			})
		}

		// Check for rate limits.
		handleGraphQLRateLimit(ctx, &query.RateLimit)

		// If there is no next page, break out.
		if !query.Enterprise.Members.PageInfo.HasNextPage {
			break
		}

		// Update cursor to the end cursor of the current page.
		variables["cursor"] = &query.Enterprise.Members.PageInfo.EndCursor
	}

	slog.Debug("fetched all enterprise cloud users", "users", len(allUsers))
	return allUsers, nil
}

// HasRecentContributions checks if a user has any contributions since the provided time.
// It uses the GraphQL API to fetch contribution statistics for the specified user,
// including commits, issues, pull requests, and reviews.
func HasRecentContributions(ctx context.Context, graphQLClient *githubv4.Client, user string, since time.Time) (bool, error) {
	slog.Debug("checking recent contributions",
		"user", user,
		"since", since,
	)

	var query struct {
		User struct {
			ContributionsCollection struct {
				TotalCommitContributions            int
				TotalIssueContributions             int
				TotalPullRequestContributions       int
				TotalPullRequestReviewContributions int
				HasAnyContributions                 bool
				HasAnyRestrictedContributions       bool
			} `graphql:"contributionsCollection(from: $since)"`
		} `graphql:"user(login: $login)"`
		RateLimit rateLimitQuery
	}

	vars := map[string]interface{}{
		"login": githubv4.String(user),
		"since": githubv4.DateTime{Time: since},
	}

	if err := graphQLClient.Query(ctx, &query, vars); err != nil {
		return false, fmt.Errorf("query recent contributions for %q failed: %w", user, err)
	}

	// Check for rate limits.
	handleGraphQLRateLimit(ctx, &query.RateLimit)

	contrib := query.User.ContributionsCollection
	total := contrib.TotalCommitContributions +
		contrib.TotalIssueContributions +
		contrib.TotalPullRequestContributions +
		contrib.TotalPullRequestReviewContributions

	// check for contributions
	if total > 0 {
		slog.Debug("detected contributions for user",
			"user", user,
			"totalContributions", total,
		)
	}

	active := contrib.HasAnyContributions || contrib.HasAnyRestrictedContributions || total > 0

	return active, nil
}

// FetchUserEmail queries the enterprise GraphQL API to retrieve the email address for the specified user.
// It attempts to find the user's email from SAML or SCIM identity providers
// and returns "N/A" if no email is found.
func FetchUserEmail(ctx context.Context, graphQLClient *githubv4.Client, slug string, user string) (string, error) {
	slog.Debug("fetching email for user", "user", user)

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
		RateLimit rateLimitQuery
	}
	variables := map[string]interface{}{
		"slug":  githubv4.String(slug),
		"login": githubv4.String(user),
	}
	if err := graphQLClient.Query(ctx, &query, variables); err != nil {
		return "", fmt.Errorf("query user email for %q failed: %w", user, err)
	}

	// Check for rate limits.
	handleGraphQLRateLimit(ctx, &query.RateLimit)

	for _, node := range query.Enterprise.OwnerInfo.SamlIdentityProvider.ExternalIdentities.Nodes {
		if string(node.User.Login) == user {
			// Prefer SamlIdentity emails over ScimIdentity.
			if len(node.SamlIdentity.Emails) > 0 {
				slog.Debug("found email for user", "user", user, "email", node.SamlIdentity.Emails[0].Value)
				return string(node.SamlIdentity.Emails[0].Value), nil
			}
			if len(node.ScimIdentity.Emails) > 0 {
				slog.Debug("found email for user", "user", user, "email", node.ScimIdentity.Emails[0].Value)
				return string(node.ScimIdentity.Emails[0].Value), nil
			}
		}
	}

	slog.Debug("no email found for user", "user", user)

	return "N/A", nil
}

// FetchEnterpriseOrgs retrieves all organizations for the specified enterprise using the GraphQL API.
// It handles pagination and rate limiting to return a complete list of organizations
// with their login names and node IDs.
func FetchEnterpriseOrgs(ctx context.Context, graphQLClient *githubv4.Client, enterpriseSlug string) ([]*github.Organization, error) {
	slog.Debug("fetching organizations for enterprise", "enterprise", enterpriseSlug)
	var query struct {
		Enterprise struct {
			Organizations struct {
				Nodes []struct {
					Login string `graphql:"login"`
					ID    string `graphql:"id"`
				} `graphql:"nodes"`
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				} `graphql:"pageInfo"`
			} `graphql:"organizations(first: 100, after: $cursor)"`
		} `graphql:"enterprise(slug: $enterpriseSlug)"`
		RateLimit rateLimitQuery
	}
	variables := map[string]interface{}{
		"enterpriseSlug": githubv4.String(enterpriseSlug),
		"cursor":         (*githubv4.String)(nil),
	}

	orgs := make([]*github.Organization, 0, 100)
	for {
		err := graphQLClient.Query(ctx, &query, variables)
		if err != nil {
			return nil, fmt.Errorf("fetch organizations for enterprise %q failed: %w", enterpriseSlug, err)
		}
		slog.Debug("fetched a page of organizations",
			"organizationsFetched", len(query.Enterprise.Organizations.Nodes),
			"hasNextPage", query.Enterprise.Organizations.PageInfo.HasNextPage,
			"endCursor", string(query.Enterprise.Organizations.PageInfo.EndCursor),
		)

		for _, node := range query.Enterprise.Organizations.Nodes {
			orgs = append(orgs, &github.Organization{
				Login:  &node.Login,
				NodeID: &node.ID,
			})
		}

		// Check rate limit
		handleGraphQLRateLimit(ctx, &query.RateLimit)

		// Check if there are more pages
		if !query.Enterprise.Organizations.PageInfo.HasNextPage {
			break
		}
		variables["cursor"] = query.Enterprise.Organizations.PageInfo.EndCursor
	}
	slog.Debug("fetched all organizations", "total", len(orgs))
	return orgs, nil
}
