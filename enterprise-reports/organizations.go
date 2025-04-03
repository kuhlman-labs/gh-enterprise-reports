package enterprisereports

import (
	"context"

	"github.com/shurcooL/githubv4"
)

type Organization struct {
	Name string
	ID   githubv4.ID
}

func getEnterpriseOrgs(ctx context.Context, client *githubv4.Client, config *Config) ([]Organization, error) {
	var query struct {
		Enterprise struct {
			Organizations struct {
				Nodes []Organization
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				} `graphql:"pageInfo"`
			} `graphql:"organizations(first: 100, after: $cursor)"`
		} `graphql:"enterprise(slug: $enterpriseSlug)"`
	}
	variables := map[string]interface{}{
		"enterpriseSlug": githubv4.String(config.EnterpriseSlug),
		"cursor":        (*githubv4.String)(nil),
	}
	err := client.Query(ctx, &query, variables)
	if err != nil {
		return nil, err
	}
	orgs := query.Enterprise.Organizations.Nodes
	for query.Enterprise.Organizations.PageInfo.HasNextPage {
		variables["cursor"] = query.Enterprise.Organizations.PageInfo.EndCursor
		err = client.Query(ctx, &query, variables)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, query.Enterprise.Organizations.Nodes...)
	}
	return orgs, nil
}
