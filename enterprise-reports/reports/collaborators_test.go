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
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollaboratorsReport_FileCreationError(t *testing.T) {
	ctx := context.Background()
	// invalid directory to force createCSVFileWithHeader error
	invalidPath := "/this/path/does/not/exist/report.csv"
	err := CollaboratorsReport(ctx, nil, nil, "ent", invalidPath)
	require.Error(t, err)
}

func TestCollaboratorsReport_GraphQLFetchError(t *testing.T) {
	// server always returns 500 on /graphql
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	httpClient := srv.Client()
	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", httpClient)

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "out.csv")

	err := CollaboratorsReport(context.Background(), nil, graphClient, "ent", filePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch enterprise orgs")
}

func TestCollaboratorsReport_NoOrgs(t *testing.T) {
	// GraphQL returns empty organizations
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	}))
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	restClient := github.NewClient(nil)

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "out.csv")

	err := CollaboratorsReport(context.Background(), restClient, graphClient, "ent", filePath)
	require.NoError(t, err)

	data, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t, "Repository,Collaborators", lines[0])
}

func TestCollaboratorsReport_SingleRepoSingleCollaborator(t *testing.T) {
	mux := http.NewServeMux()

	// GraphQL: one org
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[{"login":"org1"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})

	// REST: list repos
	mux.HandleFunc("/orgs/org1/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// include owner to ensure repo.Owner is populated
		fmt.Fprintln(w, `[{"name":"repo1","full_name":"org1/repo1","owner":{"login":"org1"}}]`)
	})

	// REST: list collaborators
	mux.HandleFunc("/repos/org1/repo1/collaborators", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"login":"user1","id":123,"permissions":{"admin":true,"push":false,"pull":true}}]`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// REST client pointing at our server
	restClient := github.NewClient(srv.Client())
	baseURL, _ := url.Parse(srv.URL + "/")
	restClient.BaseURL = baseURL

	// GraphQL client pointing at our server
	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "out.csv")

	err := CollaboratorsReport(context.Background(), restClient, graphClient, "ent", filePath)
	require.NoError(t, err)

	data, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "Repository,Collaborators", lines[0])

	// parse the record line
	reader := csv.NewReader(strings.NewReader(lines[1]))
	record, parseErr := reader.Read()
	require.NoError(t, parseErr)
	assert.Equal(t, "org1/repo1", record[0])

	var info CollaboratorInfo
	jsonErr := json.Unmarshal([]byte(record[1]), &info)
	require.NoError(t, jsonErr)
	assert.Equal(t, CollaboratorInfo{
		Login:      "user1",
		ID:         123,
		Permission: "admin",
	}, info)
}
