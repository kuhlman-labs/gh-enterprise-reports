// Package reports implements various report generation functionalities for GitHub Enterprise.
// This file contains tests for the teams report functionality.
package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTeamsReport_FileCreationError tests that the TeamsReport function
// returns an error when given an invalid output file path.
func TestTeamsReport_FileCreationError(t *testing.T) {
	err := TeamsReport(context.Background(), nil, nil, "ent", "/no/such/dir/out.csv", 1) // Add workerCount=1
	require.Error(t, err)
}

// TestTeamsReport_GraphQLFetchError tests that the TeamsReport function
// properly propagates errors when the GraphQL API call fails.
func TestTeamsReport_GraphQLFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	err := TeamsReport(context.Background(), nil, graphClient, "ent", filepath.Join(t.TempDir(), "out.csv"), 1) // Add workerCount=1
	require.Error(t, err)
}

// TestTeamsReport_NoTeams tests that the TeamsReport function generates
// a valid CSV file with only the header row when no teams are found.
func TestTeamsReport_NoTeams(t *testing.T) {
	mux := http.NewServeMux()
	// GraphQL returns no orgs
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	restClient := github.NewClient(srv.Client())
	baseURL, _ := url.Parse(srv.URL + "/")
	restClient.BaseURL = baseURL

	out := filepath.Join(t.TempDir(), "out.csv")
	err := TeamsReport(context.Background(), restClient, graphClient, "ent", out, 1) // Add workerCount=1
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t,
		"Team ID,Owner,Team Name,Team Slug,External Group,Members",
		lines[0],
	)
}

// TestTeamsReport_SingleTeam tests that the TeamsReport function correctly
// processes a single team with members and external groups, generating
// a properly formatted CSV file with the expected data.
func TestTeamsReport_SingleTeam(t *testing.T) {
	mux := http.NewServeMux()
	// GraphQL: one org
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[{"login":"org1"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	// REST: list teams for org
	mux.HandleFunc("/orgs/org1/teams", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `[{"id":1,"slug":"team1","name":"Team One"}]`)
	})
	// REST: list members by slug
	mux.HandleFunc("/orgs/org1/teams/team1/members", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `[{"login":"user1"}]`)
	})
	// REST: list external groups
	mux.HandleFunc("/orgs/org1/teams/team1/external-groups", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"groups":[{"group_name":"groupX"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cli := srv.Client()
	restClient := github.NewClient(cli)
	baseURL, _ := url.Parse(srv.URL + "/")
	restClient.BaseURL = baseURL
	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())

	out := filepath.Join(t.TempDir(), "out.csv")
	err := TeamsReport(context.Background(), restClient, graphClient, "ent", out, 1) // Add workerCount=1
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	// header
	assert.Equal(t,
		"Team ID,Owner,Team Name,Team Slug,External Group,Members",
		lines[0],
	)
	// record
	r := csv.NewReader(strings.NewReader(lines[1]))
	record, err := r.Read()
	require.NoError(t, err)
	require.Len(t, record, 6)
	assert.Equal(t, "1", record[0])
	assert.Equal(t, "org1", record[1])
	assert.Equal(t, "Team One", record[2])
	assert.Equal(t, "team1", record[3])
	assert.Equal(t, "groupX", record[4])
	assert.Equal(t, "user1", record[5])
}
