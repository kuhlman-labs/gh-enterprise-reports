// Package reports implements various report generation functionalities for GitHub Enterprise.
// This file contains tests for the repositories report functionality.
package reports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRepositoryReport_FileCreationError tests that the RepositoryReport function
// returns an error when given an invalid output file path.
func TestRepositoryReport_FileCreationError(t *testing.T) {
	cache := utils.NewSharedCache()
	err := RepositoryReport(context.Background(), nil, nil, "ent", "/no/such/dir/out.csv", 1, cache)
	require.Error(t, err)
}

// TestRepositoryReport_GraphQLFetchError tests that the RepositoryReport function
// properly propagates errors when the GraphQL API call to fetch organizations fails.
func TestRepositoryReport_GraphQLFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	cache := utils.NewSharedCache()
	err := RepositoryReport(context.Background(), nil, graphClient, "ent", filepath.Join(t.TempDir(), "out.csv"), 1, cache)
	require.Error(t, err)
}

// TestRepositoryReport_NoRepos tests that the RepositoryReport function generates
// a valid CSV file with only the header row when no repositories are found.
func TestRepositoryReport_NoRepos(t *testing.T) {
	mux := http.NewServeMux()
	// GraphQL returns no orgs
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
		require.NoError(t, err)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	restClient := github.NewClient(srv.Client())
	// before: baseURL, _ := url.Parse(srv.URL)
	baseURL, _ := url.Parse(srv.URL + "/")
	restClient.BaseURL = baseURL

	out := filepath.Join(t.TempDir(), "out.csv")
	cache := utils.NewSharedCache()
	err := RepositoryReport(context.Background(), restClient, graphClient, "ent", out, 1, cache)
	require.NoError(t, err)

	bs, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(bs)), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t,
		"Owner,Repository,Archived,Visibility,Pushed_At,Created_At,Topics,Custom_Properties,Teams",
		lines[0],
	)
}

// TestRepositoryReport_SingleRepoSingleTeamSingleCustomProp tests that the RepositoryReport function
// correctly processes a repository with its teams, external groups, and custom properties,
// generating a properly formatted CSV file with the expected data.
func TestRepositoryReport_SingleRepoSingleTeamSingleCustomProp(t *testing.T) {
	mux := http.NewServeMux()
	// GraphQL: one org
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[{"login":"org1","id":"ORG1ID"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	// REST: list repos
	mux.HandleFunc("/orgs/org1/repos", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `[{ 
			"name":"repo1","full_name":"org1/repo1","archived":false,"visibility":"public",
			"owner":{"login":"org1"},
			"pushed_at":"2020-01-01T00:00:00Z","created_at":"2020-01-01T00:00:00Z",
			"topics":["topic1","topic2"]
		}]`)
	})
	// REST: list teams for the repository
	mux.HandleFunc("/repos/org1/repo1/teams", func(w http.ResponseWriter, r *http.Request) {
		// This endpoint is called by api.FetchTeams within processRepository
		_, _ = fmt.Fprintln(w, `[{"slug":"team1", "name": "Team One", "id": 1}]`)
	})
	// REST: list external groups
	mux.HandleFunc("/orgs/org1/teams/team1/external-groups", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"groups": [{"group_name":"group1","group_id":42}]}`)
	})
	// REST: list custom properties
	mux.HandleFunc("/repos/org1/repo1/properties/values", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock handler hit for custom properties: %s", r.URL.Path)
		_, _ = fmt.Fprintln(w, `[{"property_name":"prop1","value":"val1"}]`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// wrap the server client to inject rate‐limit headers
	cli := srv.Client()
	cli.Transport = headerRT{base: http.DefaultTransport}

	restClient := github.NewClient(cli)
	// before: baseURL, _ := url.Parse(srv.URL)
	baseURL, _ := url.Parse(srv.URL + "/")
	restClient.BaseURL = baseURL
	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())

	out := filepath.Join(t.TempDir(), "out.csv")
	cache := utils.NewSharedCache()
	err := RepositoryReport(context.Background(), restClient, graphClient, "ent", out, 1, cache)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	// header
	assert.Equal(t,
		"Owner,Repository,Archived,Visibility,Pushed_At,Created_At,Topics,Custom_Properties,Teams",
		lines[0],
	)
	// record
	r := csv.NewReader(strings.NewReader(lines[1]))
	record, err := r.Read()
	require.NoError(t, err)
	require.Len(t, record, 9)

	assert.Equal(t, "org1", record[0])
	assert.Equal(t, "repo1", record[1])
	assert.Equal(t, "false", record[2])
	assert.Equal(t, "public", record[3])
	// times are parsed into a non-empty string starting with the date
	assert.True(t, strings.HasPrefix(record[4], "2020-01-01"))
	assert.True(t, strings.HasPrefix(record[5], "2020-01-01"))
	assert.Equal(t, "[topic1 topic2]", record[6])
	// Assert the formatted custom property string
	assert.Equal(t, `prop1=val1`, record[7])
	// Assert the formatted team string with external group
	assert.Equal(t, `team1 (group1)`, record[8])
	var cpvs []github.CustomPropertyValue
	err = json.Unmarshal([]byte(record[7]), &cpvs)

	_ = err
}

// headerRT is a custom http.RoundTripper that injects rate limit headers
// into every API response for testing rate limit handling.
type headerRT struct {
	base http.RoundTripper // The underlying transport to delegate actual requests to
}

// RoundTrip implements the http.RoundTripper interface by delegating to the base
// transport and adding GitHub rate limit headers to the response.
func (h headerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := h.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resp.Header.Set("X-RateLimit-Remaining", "100")
	resp.Header.Set("X-RateLimit-Limit", "200")
	return resp, nil
}
