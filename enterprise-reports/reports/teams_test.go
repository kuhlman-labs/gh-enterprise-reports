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

// TestSetMembers_Nil ensures setMembers handles nil slices.
func TestSetMembers_Nil(t *testing.T) {
	tr := &TeamReport{}
	tr.setMembers(nil)
	assert.NotNil(t, tr.Members)
	assert.Len(t, tr.Members, 0)
}

// TestSetMembers_NonNil ensures setMembers sets provided slice.
func TestSetMembers_NonNil(t *testing.T) {
	user := &github.User{Login: github.Ptr("user1")}
	tr := &TeamReport{}
	tr.setMembers([]*github.User{user})
	assert.Len(t, tr.Members, 1)
	assert.Equal(t, "user1", tr.Members[0].GetLogin())
}

// TestSetExternalGroups_Nil ensures setExternalGroups handles nil.
func TestSetExternalGroups_Nil(t *testing.T) {
	tr := &TeamReport{}
	tr.setExternalGroups(nil)
	assert.NotNil(t, tr.ExternalGroups)
	assert.Len(t, tr.ExternalGroups.Groups, 0)
}

// TestSetExternalGroups_NonNil ensures externalGroups set correctly.
func TestSetExternalGroups_NonNil(t *testing.T) {
	ext := &github.ExternalGroupList{Groups: []*github.ExternalGroup{{GroupName: github.Ptr("group1")}}}
	tr := &TeamReport{}
	tr.setExternalGroups(ext)
	assert.Len(t, tr.ExternalGroups.Groups, 1)
	assert.Equal(t, "group1", tr.ExternalGroups.Groups[0].GetGroupName())
}

// TestTeamsReport_FileCreationError should fail if output path invalid.
func TestTeamsReport_FileCreationError(t *testing.T) {
	err := TeamsReport(context.Background(), nil, nil, "ent", "/no/such/dir/out.csv")
	require.Error(t, err)
}

// TestTeamsReport_GraphQLFetchError ensures GraphQL fetch error bubbles up.
func TestTeamsReport_GraphQLFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	err := TeamsReport(context.Background(), nil, graphClient, "ent", filepath.Join(t.TempDir(), "out.csv"))
	require.Error(t, err)
}

// TestTeamsReport_NoTeams should produce only CSV header when no orgs exist.
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
	err := TeamsReport(context.Background(), restClient, graphClient, "ent", out)
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
	err := TeamsReport(context.Background(), restClient, graphClient, "ent", out)
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
