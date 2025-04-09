package enterprisereports

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
)

type Organization struct {
	Login string
}

type Members struct {
	Login string
	ID    string
	Name  string
	Type  string
}

// getEnterpriseOrgs retrieves all organizations for the specified enterprise.
func getEnterpriseOrgs(ctx context.Context, graphQLClient *githubv4.Client, config *Config) ([]Organization, error) {
	log.Info().Str("EnterpriseSlug", config.EnterpriseSlug).Msg("Fetching organizations for enterprise.")
	var query struct {
		Enterprise struct {
			Organizations struct {
				Nodes    []Organization
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				} `graphql:"pageInfo"`
			} `graphql:"organizations(first: 100, after: $cursor)"`
		} `graphql:"enterprise(slug: $enterpriseSlug)"`
		RateLimit struct {
			Cost      int
			Limit     int
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}
	variables := map[string]interface{}{
		"enterpriseSlug": githubv4.String(config.EnterpriseSlug),
		"cursor":         (*githubv4.String)(nil),
	}

	orgs := make([]Organization, 0, 100)
	for {
		err := graphQLClient.Query(ctx, &query, variables)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch organizations.")
			return nil, fmt.Errorf("failed to fetch organizations: %w", err)
		}
		log.Debug().Int("OrganizationsFetched", len(query.Enterprise.Organizations.Nodes)).
			Bool("HasNextPage", query.Enterprise.Organizations.PageInfo.HasNextPage).
			Str("EndCursor", string(query.Enterprise.Organizations.PageInfo.EndCursor)).
			Msg("Fetched a page of organizations.")
		orgs = append(orgs, query.Enterprise.Organizations.Nodes...)

		// Check rate limit
		if query.RateLimit.Remaining < GraphQLRateLimitThreshold {
			log.Warn().Int("Remaining", query.RateLimit.Remaining).Int("Limit", query.RateLimit.Limit).Msg("Rate limit low, waiting until reset")
			waitForLimitReset(ctx, "GraphQL", query.RateLimit.Remaining, query.RateLimit.Limit, query.RateLimit.ResetAt.Time)
		}

		// Check if there are more pages
		if !query.Enterprise.Organizations.PageInfo.HasNextPage {
			break
		}
		variables["cursor"] = query.Enterprise.Organizations.PageInfo.EndCursor
	}
	log.Info().Int("TotalOrganizations", len(orgs)).Msg("Successfully fetched all organizations.")
	return orgs, nil
}

// getOrganization fetches the details for the specified organization.
func getOrganization(ctx context.Context, restClient *github.Client, orgLogin string) (*github.Organization, error) {
	log.Debug().Str("OrganizationLogin", orgLogin).Msg("Fetching organization details.")

	org, resp, err := restClient.Organizations.Get(ctx, orgLogin)
	if err != nil {
		log.Error().Str("OrganizationLogin", orgLogin).Err(err).Msg("Failed to fetch organization details.")
		return nil, fmt.Errorf("failed to fetch organization details for %s: %w", orgLogin, err)
	}
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low, waiting until reset")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}
	log.Debug().Str("OrganizationLogin", org.GetLogin()).Msg("Successfully fetched organization details.")
	return org, err
}

// writeCSVHeader writes the header row to the CSV file.
func writeCSVHeader(writer *csv.Writer, headers []string) error {
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}
	return nil
}

/*
// getOrganizationMemberships fetches all organization memberships in the given organization using pagination.
func getOrganizationMemberships(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, orgLogin string) ([]*github.User, error) {
	log.Info().Str("Organization", orgLogin).Msg("Fetching organization memberships.")
	var query struct {
		Organization struct {
			MembersWithRole struct {
				Edges []struct {
					Role string
					Node struct {
						Name  string
						Login string
						ID    string
					}
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				}
			} `graphql:"membersWithRole(first: 100, after: $cursor)"`
		} `graphql:"organization(login: $login)"`
		RateLimit struct {
			Cost      int
			Limit     int
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}
	variables := map[string]interface{}{
		"login":  githubv4.String(orgLogin),
		"cursor": (*githubv4.String)(nil),
	}

	memberMap := make(map[string]*github.User)
	for {
		err := graphQLClient.Query(ctx, &query, variables)
		if err != nil {
			log.Error().Err(err).Str("Organization", orgLogin).Msg("Failed to fetch memberships.")
			return nil, fmt.Errorf("failed to fetch memberships for organization %s: %w", orgLogin, err)
		}
		for _, edge := range query.Organization.MembersWithRole.Edges {
			memberMap[edge.Node.Login] = &github.User{
				Login:    &edge.Node.Login,
				Name:     &edge.Node.Name,
				NodeID:   &edge.Node.ID,
				RoleName: &edge.Role,
			}
		}
		// Check rate limit
		if query.RateLimit.Remaining < 10 {
			log.Warn().Int("Remaining", query.RateLimit.Remaining).Int("Limit", query.RateLimit.Limit).Msg("Rate limit low, waiting until reset")
			waitForLimitReset(ctx, "GraphQL", query.RateLimit.Remaining, query.RateLimit.Limit, query.RateLimit.ResetAt.Time)
		}
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
	log.Info().Str("OrganizationLogin", orgLogin).Int("TotalMemberships", len(allMemberships)).Msg("Successfully fetched all memberships.")
	return allMemberships, nil
}
*/

// getOrganizationMemberships retrieves all organization memberships for the specified organization using the REST API.
func getOrganizationMemberships(ctx context.Context, restClient *github.Client, orgLogin string) ([]*github.User, error) {
	log.Info().Str("Organization", orgLogin).Msg("Fetching organization memberships.")

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
			log.Error().Err(err).Str("Organization", orgLogin).Msg("Failed to fetch memberships.")
			return nil, fmt.Errorf("failed to fetch memberships for organization %s: %w", orgLogin, err)
		}

		allMemberships = append(allMemberships, memberships...)

		// Check rate limit
		if resp.Rate.Remaining < RESTRateLimitThreshold {
			log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low, waiting until reset")
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
		memberRole, err := getOrganizationMember(ctx, restClient, orgLogin, member.GetLogin())
		if err != nil {
			log.Error().Err(err).Str("Organization", orgLogin).Msg("Failed to fetch user details.")
			return nil, fmt.Errorf("failed to fetch user details for %s: %w", member.GetLogin(), err)
		}

		user, err := getUserById(ctx, restClient, member.GetID())
		if err != nil {
			log.Error().Err(err).Int64("User", member.GetID()).Msg("Failed to fetch user details.")
			return nil, fmt.Errorf("failed to fetch user details for %d: %w", member.GetID(), err)
		}

		member := &github.User{
			Login:    user.Login,
			Name:     user.Name,
			NodeID:   user.NodeID,
			RoleName: memberRole.Role,
		}

		membershipList = append(membershipList, member)
	}

	log.Info().Str("OrganizationLogin", orgLogin).Int("TotalMemberships", len(membershipList)).Msg("Successfully fetched all memberships.")
	return membershipList, nil
}

// getOrganizationMember retrieves the membership details of a user in the given organization via the REST API.
func getOrganizationMember(ctx context.Context, restClient *github.Client, orgLogin, userLogin string) (*github.Membership, error) {
	log.Debug().Str("Organization", orgLogin).Str("User", userLogin).Msg("Fetching organization membership.")

	membership, resp, err := restClient.Organizations.GetOrgMembership(ctx, userLogin, orgLogin)
	if err != nil {
		log.Error().Err(err).Str("Organization", orgLogin).Str("User", userLogin).Msg("Failed to fetch membership.")
		return nil, fmt.Errorf("failed to fetch membership for user %s in organization %s: %w", userLogin, orgLogin, err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low, waiting until reset")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	log.Debug().Str("Organization", orgLogin).Str("User", userLogin).Msg("Successfully fetched organization membership.")
	return membership, nil
}

// getUserById fetches a user by their ID using the REST API.
func getUserById(ctx context.Context, restClient *github.Client, id int64) (*github.User, error) {
	log.Debug().Int64("User", id).Msg("Fetching user by ID.")

	user, resp, err := restClient.Users.GetByID(ctx, id)
	if err != nil {
		log.Error().Err(err).Int64("User", id).Msg("Failed to fetch user.")
		return nil, fmt.Errorf("failed to fetch user by ID: %w", err)
	}

	// Check rate limit
	if resp.Rate.Remaining < RESTRateLimitThreshold {
		log.Warn().Int("Remaining", resp.Rate.Remaining).Int("Limit", resp.Rate.Limit).Msg("Rate limit low, waiting until reset")
		waitForLimitReset(ctx, "REST", resp.Rate.Remaining, resp.Rate.Limit, resp.Rate.Reset.Time)
	}

	log.Debug().Int64("User", id).Msg("Successfully fetched user.")
	return user, nil
}

// runOrganizationsReport generates a CSV report for all enterprise organizations, including organization details and memberships.
func runOrganizationsReport(ctx context.Context, graphQLClient *githubv4.Client, restClient *github.Client, config *Config, filename string) error {

	log.Info().Str("Filename", filename).Msg("Starting organizations report generation.")

	// Create and open the CSV file
	file, err := os.Create(filename)
	if err != nil {
		log.Error().Err(err).Str("Filename", filename).Msg("Failed to create report file.")
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Error().Err(err).Str("Filename", filename).Msg("Failed to close report file.")
		}
	}()

	writer := csv.NewWriter(file)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			log.Error().Err(err).Msg("Failed to flush CSV writer.")
		}
	}()

	// Write CSV header
	if err := writeCSVHeader(writer, []string{
		"Organization",
		"Organization ID",
		"Organization Default Repository Permission",
		"Members",
		"Total Members",
	}); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header.")
		return err
	}

	// Fetch organizations
	orgs, err := getEnterpriseOrgs(ctx, graphQLClient, config)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch organizations.")
		return fmt.Errorf("failed to fetch organizations: %w", err)
	}

	type orgResult struct {
		organization *github.Organization
		members      []*github.User
		err          error
	}

	// Concurrency setup
	orgChan := make(chan Organization, len(orgs))
	resultChan := make(chan orgResult, len(orgs))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit the number of concurrent workers

	// Start worker function with context cancellation checks
	worker := func() {
		defer wg.Done()
		for org := range orgChan {
			// Check for context cancellation before processing an organization.
			if ctx.Err() != nil {
				log.Warn().Str("Organization", org.Login).Msg("Context canceled, stopping worker.")
				return
			}
			semaphore <- struct{}{} // Acquire semaphore token
			organization, err := getOrganization(ctx, restClient, org.Login)
			<-semaphore // Release token
			if err != nil {
				log.Error().Err(err).Str("Organization", org.Login).Msg("Failed to fetch organization details. Marking as unavailable.")
				resultChan <- orgResult{organization: nil, members: nil, err: fmt.Errorf("details not available for %s", org.Login)}
				continue
			}
			// Check cancellation after fetching organization details.
			if ctx.Err() != nil {
				log.Warn().Str("Organization", org.Login).Msg("Context canceled after fetching details, stopping worker.")
				return
			}

			users, err := getOrganizationMemberships(ctx, restClient, organization.GetLogin())
			if err != nil {
				log.Error().Err(err).Str("Organization", org.Login).Msg("Failed to fetch memberships. Marking as unavailable.")
				resultChan <- orgResult{organization: organization, members: nil, err: fmt.Errorf("memberships not available for %s", org.Login)}
				continue
			}

			resultChan <- orgResult{organization: organization, members: users}
		}
	}

	// Start workers
	numWorkers := 5
	workerIDs := make([]int, numWorkers)
	for range workerIDs {
		wg.Add(1)
		go worker()
	}

	// Send organizations to workers
	go func() {
		for _, org := range orgs {
			orgChan <- org
		}
		close(orgChan)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

OuterLoop:
	for result := range resultChan {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context canceled, stopping result processing.")
			break OuterLoop
		default:
		}

		var orgLogin, orgID, defaultRepoPermission, membersString, totalMembers string

		if result.err != nil {
			log.Error().Err(result.err).Msg("Error processing organization. Writing placeholder information.")
			orgLogin = result.err.Error()
			orgID = "N/A"
			defaultRepoPermission = "N/A"
			membersString = "N/A"
			totalMembers = "N/A"
		} else {
			orgLogin = result.organization.GetLogin()
			orgID = result.organization.GetNodeID()
			defaultRepoPermission = result.organization.GetDefaultRepoPermission()

			var memberDetails []string
			for _, user := range result.members {
				memberDetails = append(memberDetails, fmt.Sprintf("{%s,%s,%s,%s}",
					user.GetLogin(),
					user.GetNodeID(),
					user.GetName(),
					user.GetRoleName(),
				))
			}
			membersString = strings.Join(memberDetails, ",")
			totalMembers = fmt.Sprintf("%d", len(result.members))
		}

		if err := writer.Write([]string{
			orgLogin,
			orgID,
			defaultRepoPermission,
			membersString,
			totalMembers,
		}); err != nil {
			log.Error().Err(err).Str("Organization", orgLogin).Msg("Failed to write organization to report. Skipping.")
			continue
		}
		log.Debug().Str("Organization", orgLogin).Msg("Successfully processed organization.")
	}

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		log.Error().Msg("Context deadline exceeded while generating organizations report.")
		return ctx.Err()
	}

	log.Info().Str("Filename", filename).Msg("Organizations report generated successfully.")
	return nil
}
