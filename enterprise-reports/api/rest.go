// Package api provides functionality for interacting with GitHub's REST and GraphQL APIs.
// It includes rate limiting, client wrapper methods, and utilities for efficient API consumption.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
)

// HasRecentEvents determines whether a user has any recent events after the specified time.
// It checks the user's public event stream and returns true if any events are found after the given time.
func HasRecentEvents(ctx context.Context, restClient *github.Client, user string, since time.Time) (bool, error) {
	slog.Debug("checking recent events", "user", user, "since", since)

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    1,
	}

	events, _, err := restClient.Activity.ListEventsPerformedByUser(ctx, user, false, opts)
	if err != nil {
		return false, fmt.Errorf("failed to fetch events for %q: %w", user, err)
	}

	for _, event := range events {
		if event.GetCreatedAt().After(since) {
			slog.Debug("detected recent activity", "user", user, "event_type", event.GetType(), "event_time", event.GetCreatedAt())
			return true, nil
		}
	}

	return false, nil
}

// FetchUserLogins retrieves audit log events for user login actions over the past 90 days
// and returns a mapping of login names to their most recent login times.
// This helps identify recently active vs dormant users.
func FetchUserLogins(ctx context.Context, restClient *github.Client, enterpriseSlug string, referenceTime time.Time) (map[string]time.Time, error) {
	slog.Debug("fetching user login audit logs", "enterprise", enterpriseSlug)

	// Only pull login events on or after the reference time
	phrase := fmt.Sprintf("action:user.login created:>=%s", referenceTime.Format(time.RFC3339))
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
			return nil, fmt.Errorf("get audit log for enterprise %q failed: %w", enterpriseSlug, err)
		}

		// Log added after fetching a page of audit logs.
		slog.Debug("fetched audit logs page", "count", len(auditLogs), "after_cursor", resp.After)

		allAuditLogs = append(allAuditLogs, auditLogs...)

		// Check rate limits after fetching a page of audit logs.
		handleRESTRateLimit(ctx, &resp.Rate)

		if resp.After == "" {
			break
		}

		// Update the cursor for the next page.
		opts.After = resp.After

	}

	slog.Debug("fetched all audit logs", "count", len(allAuditLogs))

	loginMap := make(map[string]time.Time)
	for _, logEntry := range allAuditLogs {
		// Ensure both Actor and CreatedAt are non-nil.
		if logEntry.Actor == nil || logEntry.CreatedAt == nil {
			continue
		}
		actor := *logEntry.Actor
		eventTime := logEntry.CreatedAt.UTC() // Ensure UTC
		// Store the latest event per user.
		if existing, found := loginMap[actor]; !found || eventTime.After(existing) {
			loginMap[actor] = eventTime
		}
	}

	slog.Debug("mapped audit logs to user logins", "unique_user_logins", len(loginMap))
	return loginMap, nil
}

// FetchTeamsForOrganizations retrieves all teams for the specified organization.
// The results are paginated and combined, with rate limit handling.
func FetchTeamsForOrganizations(ctx context.Context, restClient *github.Client, org string) ([]*github.Team, error) {
	slog.Debug("fetching teams", "organization", org)

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    1,
	}
	allTeams := []*github.Team{}

	for {
		teams, resp, err := restClient.Teams.ListTeams(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("get teams for organization %q failed: %w", org, err)
		}
		slog.Debug("fetched teams page", "count", len(teams))
		allTeams = append(allTeams, teams...)

		// Check rate limit
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Debug("found teams", "count", len(allTeams))
	return allTeams, nil
}

// FetchTeamMembers retrieves all members for the specified team and organization.
// The function handles pagination and rate limiting automatically.
func FetchTeamMembers(ctx context.Context, restClient *github.Client, team *github.Team, org string) ([]*github.User, error) {
	slog.Debug("getting members", "team", team.GetSlug())

	opts := &github.TeamListTeamMembersOptions{
		Role: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	allMembers := []*github.User{}

	for {
		members, resp, err := restClient.Teams.ListTeamMembersBySlug(ctx, org, team.GetSlug(), opts)
		if err != nil {
			return nil, fmt.Errorf("get members for team %q failed: %w", team.GetSlug(), err)
		}
		slog.Debug("fetched a page of members", "members_in_page", len(members))
		allMembers = append(allMembers, members...)

		// Check rate limit
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Debug("found members", "count", len(allMembers), "team", team.GetSlug())
	return allMembers, nil
}

// FetchOrganizationRepositories retrieves all repositories for the specified organization.
// Results include all repository types (public, private, etc.) with pagination handling.
func FetchOrganizationRepositories(ctx context.Context, restClient *github.Client, org string) ([]*github.Repository, error) {
	slog.Debug("fetching organization repositories", "organization", org)

	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}
	allRepos := []*github.Repository{}

	for {
		repos, resp, err := restClient.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("get repositories for organization %q failed: %w", org, err)
		}
		slog.Debug("fetched a page of repositories", "repos_in_page", len(repos))
		allRepos = append(allRepos, repos...)

		// Check rate limit
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	slog.Debug("found repositories", "count", len(allRepos), "organization", org)
	return allRepos, nil
}

// FetchTeams retrieves all teams that have access to the specified repository.
// Results are paginated with proper rate limit handling.
func FetchTeams(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.Team, error) {
	slog.Debug("getting teams", "repository", repo)

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    1,
	}
	allTeams := []*github.Team{}

	for {
		teams, resp, err := restClient.Repositories.ListTeams(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("get teams for repository %q/%q failed: %w", owner, repo, err)
		}
		slog.Debug("fetched a page of teams", "teams_in_page", len(teams))
		allTeams = append(allTeams, teams...)

		// Check rate limit
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Debug("found teams", "count", len(allTeams), "repository", repo)

	return allTeams, nil
}

// FetchExternalGroups retrieves external groups (such as SAML identity provider groups)
// for the specified team.
func FetchExternalGroups(ctx context.Context, restClient *github.Client, owner, teamSlug string) (*github.ExternalGroupList, error) {
	slog.Debug("getting external groups", "teamSlug", teamSlug)

	externalGroups, resp, err := restClient.Teams.ListExternalGroupsForTeamBySlug(ctx, owner, teamSlug)
	if err != nil {
		return nil, fmt.Errorf("get external groups for team %q/%q: %w", owner, teamSlug, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, &resp.Rate)

	slog.Debug("fetched external groups", "count", len(externalGroups.Groups))

	return externalGroups, nil
}

// FetchCustomProperties retrieves all custom properties for the specified repository.
// Custom properties are organization-defined metadata fields attached to repositories.
func FetchCustomProperties(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.CustomPropertyValue, error) {
	slog.Debug("fetching custom properties", "repository", repo)

	customProperties, resp, err := restClient.Repositories.GetAllCustomPropertyValues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get custom properties for repository %q/%q: %w", owner, repo, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, &resp.Rate)

	slog.Debug("fetched custom properties", "count", len(customProperties))

	return customProperties, nil
}

// FetchOrganizationMemberships retrieves all organization members with their roles
// for the specified organization using the REST API.
// For each member, additional details are fetched including their role and display name.
func FetchOrganizationMemberships(ctx context.Context, restClient *github.Client, orgLogin string) ([]*github.User, error) {
	slog.Debug("fetching organization memberships", "organization", orgLogin)

	allMemberships := []*github.User{}
	opts := &github.ListMembersOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	for {

		memberships, resp, err := restClient.Organizations.ListMembers(ctx, orgLogin, opts)
		if err != nil {
			return nil, fmt.Errorf("fetch memberships for organization %q failed: %w", orgLogin, err)
		}

		allMemberships = append(allMemberships, memberships...)

		// Check rate limit
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	membershipList := make([]*github.User, 0, len(allMemberships))
	for _, member := range allMemberships {
		memberRole, err := FetchOrganizationMember(ctx, restClient, orgLogin, member.GetLogin())
		if err != nil {
			return nil, fmt.Errorf("fetch user details for %q failed: %w", member.GetLogin(), err)
		}

		memberName, err := FetchUserById(ctx, restClient, member.GetID())
		if err != nil {
			return nil, fmt.Errorf("fetch user by id %d failed: %w", member.GetID(), err)
		}

		member.RoleName = memberRole.Role
		member.Name = memberName.Name

		membershipList = append(membershipList, member)
	}

	slog.Debug("fetched all memberships", "organization", orgLogin, "count", len(membershipList))

	return membershipList, nil
}

// FetchOrganizationMember retrieves the membership details of a user in the given organization
// via the REST API, including their role (admin, member, etc.).
func FetchOrganizationMember(ctx context.Context, restClient *github.Client, orgLogin, userLogin string) (*github.Membership, error) {
	slog.Debug("fetching organization membership", "organization", orgLogin, "user", userLogin)

	membership, resp, err := restClient.Organizations.GetOrgMembership(ctx, userLogin, orgLogin)
	if err != nil {
		return nil, fmt.Errorf("fetch membership failed for user %q in organization %q: %w", userLogin, orgLogin, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, &resp.Rate)

	slog.Debug("fetched organization membership", "organization", orgLogin, "user", userLogin)

	return membership, nil
}

// FetchUserById fetches a user by their numeric ID using the REST API.
// This is useful when you need additional user details beyond what's available
// in other API responses.
func FetchUserById(ctx context.Context, restClient *github.Client, id int64) (*github.User, error) {
	slog.Debug("fetching user by id", "userID", id)

	user, resp, err := restClient.Users.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetch user by id %d failed: %w", id, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, &resp.Rate)

	slog.Debug("fetched user", "userID", id)

	return user, nil
}

// FetchOrganization fetches the details for the specified organization.
// This returns organization settings, default permissions, and other metadata.
func FetchOrganization(ctx context.Context, restClient *github.Client, orgLogin string) (*github.Organization, error) {
	slog.Debug("fetching organization details", "organization", orgLogin)

	org, resp, err := restClient.Organizations.Get(ctx, orgLogin)
	if err != nil {
		return nil, fmt.Errorf("fetch organization details for %q failed: %w", orgLogin, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, &resp.Rate)

	slog.Debug("fetched organization details", "organizationLogin", org.GetLogin())

	return org, err
}

// FetchRepoCollaborators retrieves all collaborators for the specified repository.
// This includes both direct collaborators and those with access through team memberships.
func FetchRepoCollaborators(ctx context.Context, restClient *github.Client, repo *github.Repository) ([]*github.User, error) {
	slog.Debug("fetching repository collaborators", "repository", repo.GetFullName())

	opts := &github.ListCollaboratorsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	var allCollaborators []*github.User
	for {
		collaborators, resp, err := restClient.Repositories.ListCollaborators(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
		if err != nil {
			return nil, fmt.Errorf("fetch collaborators for repository %q failed: %w", repo.GetFullName(), err)
		}
		allCollaborators = append(allCollaborators, collaborators...)

		// Check REST API rate limit.
		handleRESTRateLimit(ctx, &resp.Rate)

		// Check if there are more pages.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Debug("fetched repository collaborators", "count", len(allCollaborators), "repository", repo.GetFullName())

	return allCollaborators, nil
}
