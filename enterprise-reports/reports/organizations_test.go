// Package reports implements various report generation functionalities for GitHub Enterprise.
// This file contains tests for the organizations report functionality.
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

// TestOrganizationsReport_FileCreationError tests that the OrganizationsReport function
// returns an error when given an invalid output file path.
func TestOrganizationsReport_FileCreationError(t *testing.T) {
	ctx := context.Background()
	invalidPath := "/this/path/does/not/exist/report.csv"
	err := OrganizationsReport(ctx, nil, nil, "ent", invalidPath, 1)
	require.Error(t, err)
}

// TestOrganizationsReport_GraphQLFetchError tests that the OrganizationsReport function
// properly propagates errors when the GraphQL API call to fetch organizations fails.
func TestOrganizationsReport_GraphQLFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	restClient := github.NewClient(nil)

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "out.csv")

	err := OrganizationsReport(context.Background(), graphClient, restClient, "ent", filePath, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to fetch organizations")
}

// TestOrganizationsReport_NoOrgs tests that the OrganizationsReport function generates
// a valid CSV file with only the header row when no organizations are found.
func TestOrganizationsReport_NoOrgs(t *testing.T) {
	mux := http.NewServeMux()

	// GraphQL fetch no organizations
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// GraphQL client pointing at our server
	graphClient := githubv4.NewEnterpriseClient(srv.URL+"/graphql", srv.Client())
	restClient := github.NewClient(nil)

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "out.csv")

	err := OrganizationsReport(context.Background(), graphClient, restClient, "ent", filePath, 1)
	require.NoError(t, err)

	data, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t, "Organization,Organization ID,Organization Default Repository Permission,Members,Total Members", lines[0])
}

// TestOrganizationsReport_SingleOrgSingleMember tests that the OrganizationsReport function
// correctly processes a single organization with one member, generating a properly formatted
// CSV file with the expected organization and member details.
func TestOrganizationsReport_SingleOrgSingleMember(t *testing.T) {
	mux := http.NewServeMux()

	// GraphQL: one org
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"data":{"enterprise":{"organizations":{"nodes":[{"login":"org1","id":"ORG1ID"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	// REST: get organization details
	mux.HandleFunc("/orgs/org1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"login":"org1","id":321,"default_repository_permission":"admin"}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	// REST: list members
	mux.HandleFunc("/orgs/org1/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `[{"login":"user1","id":123}]`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	// REST: get membership for user1
	mux.HandleFunc("/orgs/org1/memberships/user1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"role":"admin"}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	// REST: get user by id
	mux.HandleFunc("/users/123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"login":"user1","name":"User One"}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	// alias path matching code’s fetch URL (/user/123)
	mux.HandleFunc("/user/123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"login":"user1","name":"User One"}`); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
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

	err := OrganizationsReport(context.Background(), graphClient, restClient, "ent", filePath, 1)
	require.NoError(t, err)

	data, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "Organization,Organization ID,Organization Default Repository Permission,Members,Total Members", lines[0])

	// parse the record line
	reader := csv.NewReader(strings.NewReader(lines[1]))
	record, parseErr := reader.Read()
	require.NoError(t, parseErr)
	assert.Equal(t, "org1", record[0])
	assert.Equal(t, "321", record[1])
	assert.Equal(t, "admin", record[2])

	var members []struct {
		Login    string `json:"login"`
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		RoleName string `json:"roleName"`
	}
	jsonErr := json.Unmarshal([]byte(record[3]), &members)
	require.NoError(t, jsonErr)
	require.Len(t, members, 1)
	assert.Equal(t, "user1", members[0].Login)
	assert.Equal(t, int64(123), members[0].ID)
	assert.Equal(t, "User One", members[0].Name)
	assert.Equal(t, "admin", members[0].RoleName)
	assert.Equal(t, "1", record[4])
}
