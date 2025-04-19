package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
)

// hasRecentEvents determines whether a user has any recent public events after the specified time.
func HasRecentEvents(ctx context.Context, restClient *github.Client, user string, since time.Time) (bool, error) {
	slog.Debug("checking recent events", "user", user, "since", since)

	events, resp, err := restClient.Activity.ListEventsPerformedByUser(ctx, user, false, nil)
	if err != nil {
		return false, fmt.Errorf("list events for user %q failed: %w", user, err)
	}

	// Check rate limits after fetching events.
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rest rate limit low", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
		waitForLimitReset(ctx, "rest", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	for _, event := range events {
		if event.CreatedAt != nil && event.CreatedAt.After(since) {
			slog.Debug("detected recent activity", "user", user, "event_type", *event.Type, "event_time", event.CreatedAt.Time)
			return true, nil
		}
	}
	return false, nil
}

// fetchUserLogins retrieves audit log events for user login actions over the past 90 days and returns a mapping of login to the most recent login time.
func FetchUserLogins(ctx context.Context, restClient *github.Client, enterpriseSlug string, referenceTime time.Time) (map[string]time.Time, error) {
	slog.Info("fetching audit logs", "enterprise", enterpriseSlug)

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
		if resp.Rate.Remaining < AuditLogRateLimitThreshold {
			slog.Warn("audit log rate limit low", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
			waitForLimitReset(ctx, "audit_log", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
		}

		if resp.After == "" {
			break
		}

		// Update the cursor for the next page.
		opts.ListCursorOptions.After = resp.After

	}

	slog.Info("fetched all audit logs", "count", len(allAuditLogs))

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

	slog.Info("mapped audit logs to user logins", "unique_user_logins", len(loginMap))
	return loginMap, nil
}

// FetchTeamsForOrganizations retrieves all teams for the specified organizations.
func FetchTeamsForOrganizations(ctx context.Context, restClient *github.Client, org string) ([]*github.Team, error) {
	slog.Info("fetching teams", "organization", org)

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
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Info("found teams", "count", len(allTeams))
	return allTeams, nil
}

// FetchTeamMembers retrieves all members for the specified team and organization.
func FetchTeamMembers(ctx context.Context, restClient *github.Client, team *github.Team, org string) ([]*github.User, error) {
	slog.Info("getting members", "team", team.GetSlug())

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
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Info("found members", "count", len(allMembers), "team", team.GetSlug())
	return allMembers, nil
}

// FetchOrganizationRepositories retrieves all repositories for the specified organization.
func FetchOrganizationRepositories(ctx context.Context, restClient *github.Client, org string) ([]*github.Repository, error) {
	slog.Info("getting repositories", "organization", org)
	slog.Debug("starting repositories retrieval", "organization", org)

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
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	slog.Info("found repositories", "count", len(allRepos), "organization", org)
	return allRepos, nil
}

// FetchTeams retrieves all teams for the specified repository.
func FetchTeams(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.Team, error) {
	slog.Info("getting teams", "repository", repo)

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
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allTeams, nil
}

// FetchExternalGroups retrieves external groups for the specified team.
func FetchExternalGroups(ctx context.Context, restClient *github.Client, owner, teamSlug string) (*github.ExternalGroupList, error) {
	slog.Info("getting external groups", "teamSlug", teamSlug)

	externalGroups, resp, err := restClient.Teams.ListExternalGroupsForTeamBySlug(ctx, owner, teamSlug)
	if err != nil {
		return nil, fmt.Errorf("get external groups for team %q/%q: %w", owner, teamSlug, err)
	}
	slog.Debug("fetched external groups", "external_groups_count", len(externalGroups.Groups))

	// Check rate limit
	handleRESTRateLimit(ctx, resp.Rate)

	return externalGroups, nil
}

// FetchCustomProperties retrieves all custom properties for the specified repository.
func FetchCustomProperties(ctx context.Context, restClient *github.Client, owner, repo string) ([]*github.CustomPropertyValue, error) {
	slog.Info("getting custom properties", "repository", repo)

	customProperties, resp, err := restClient.Repositories.GetAllCustomPropertyValues(ctx, owner, repo)
	if err != nil {
		slog.Error("get custom properties failed", "error", err, "repository", repo)
		return nil, fmt.Errorf("get custom properties for repository %q/%q: %w", owner, repo, err)
	}

	// Check rate limit
	handleRESTRateLimit(ctx, resp.Rate)

	return customProperties, nil
}

// FetchOrganizationMemberships retrieves all organization members with roles for the specified organization using the REST API.
func FetchOrganizationMemberships(ctx context.Context, restClient *github.Client, orgLogin string) ([]*github.User, error) {
	slog.Info("fetching organization memberships", "organization", orgLogin)

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
		if resp.Rate.Remaining < RESTRateLimitThreshold {
			slog.Warn("rate limit low, waiting until reset", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
			waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
		}

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

		member := &github.User{
			RoleName: memberRole.Role,
		}

		membershipList = append(membershipList, member)
	}

	slog.Info("fetched all memberships", "organizationLogin", orgLogin, "totalMemberships", len(membershipList))
	return membershipList, nil
}

// FetchOrganizationMember retrieves the membership details of a user in the given organization via the REST API.
func FetchOrganizationMember(ctx context.Context, restClient *github.Client, orgLogin, userLogin string) (*github.Membership, error) {
	slog.Debug("fetching organization membership", "organization", orgLogin, "user", userLogin)

	membership, resp, err := restClient.Organizations.GetOrgMembership(ctx, userLogin, orgLogin)
	if err != nil {
		return nil, fmt.Errorf("fetch membership failed for user %q in organization %q: %w", userLogin, orgLogin, err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rate limit low, waiting until reset", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	slog.Debug("fetched organization membership", "organization", orgLogin, "user", userLogin)
	return membership, nil
}

// FetchUserById fetches a user by their ID using the REST API.
func FetchUserById(ctx context.Context, restClient *github.Client, id int64) (*github.User, error) {
	slog.Debug("fetching user by id", "userID", id)

	user, resp, err := restClient.Users.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetch user by id %d failed: %w", id, err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rate limit low, waiting until reset", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	slog.Debug("fetched user", "userID", id)
	return user, nil
}

// FetchOrganization fetches the details for the specified organization.
func FetchOrganization(ctx context.Context, restClient *github.Client, orgLogin string) (*github.Organization, error) {
	slog.Debug("fetching organization details", "organization", orgLogin)

	org, resp, err := restClient.Organizations.Get(ctx, orgLogin)
	if err != nil {
		return nil, fmt.Errorf("fetch organization details for %q failed: %w", orgLogin, err)
	}
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rate limit low, waiting until reset", "remaining", resp.Rate.Remaining, "limit", resp.Rate.Limit)
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}
	slog.Debug("fetched organization details", "organizationLogin", org.GetLogin())
	return org, err
}

// FetchRepoCollaborators retrieves all collaborators for the specified repository.
func FetchRepoCollaborators(ctx context.Context, restClient *github.Client, repo *github.Repository) ([]*github.User, error) {
	slog.Info("fetching repository collaborators", "repository", repo.GetFullName())

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
		handleRESTRateLimit(ctx, resp.Rate)

		// Check if there are more pages.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	slog.Info("fetched repository collaborators", "totalCollaborators", len(allCollaborators), "repository", repo.GetFullName())
	return allCollaborators, nil
}
