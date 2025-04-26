package reports

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetEmail ensures setEmail assigns the provided value.
func TestSetEmail(t *testing.T) {
	u := &UserReport{User: &github.User{}}
	u.setEmail("foo@bar.com")
	assert.Equal(t, "foo@bar.com", u.GetEmail())
}

// TestSetLastLogin ensures setLastLogin assigns the provided time.
func TestSetLastLogin(t *testing.T) {
	ts := time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)
	u := &UserReport{}
	u.setLastLogin(ts)
	assert.True(t, u.LastLogin.Equal(ts))
}

// TestSetDormant ensures setDormant assigns the provided boolean.
func TestSetDormant(t *testing.T) {
	u := &UserReport{}
	u.setDormant(true)
	assert.True(t, u.Dormant)
	u.setDormant(false)
	assert.False(t, u.Dormant)
}

// TestUsersReport_FileCreationError should error on invalid path.
func TestUsersReport_FileCreationError(t *testing.T) {
	err := UsersReport(context.Background(), nil, nil, "ent", "/no/such/dir/out.csv")
	require.Error(t, err)
}

// TestUsersReport_NoUsers should produce only header when no users.
func TestUsersReport_NoUsers(t *testing.T) {
	// stub GraphQL server returning no enterprise members
	muxG := http.NewServeMux()
	muxG.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"data":{"enterprise":{"members":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	gSrv := httptest.NewServer(muxG)
	defer gSrv.Close()

	// stub REST server returning empty audit-log
	muxR := http.NewServeMux()
	muxR.HandleFunc("/enterprises/ent/audit-log", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	rSrv := httptest.NewServer(muxR)
	defer rSrv.Close()

	// assemble clients
	restClient := github.NewClient(rSrv.Client())
	baseURL, _ := url.Parse(rSrv.URL + "/")
	restClient.BaseURL = baseURL
	graphClient := githubv4.NewEnterpriseClient(gSrv.URL+"/graphql", gSrv.Client())

	out := filepath.Join(t.TempDir(), "users.csv")
	err := UsersReport(context.Background(), restClient, graphClient, "ent", out)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)
	assert.Equal(t,
		"ID,Login,Name,Email,Last Login(90 days),Dormant?",
		lines[0],
	)
}

func TestUsersReport_SingleUser(t *testing.T) {
	// stub GraphQL server returning one enterprise member
	muxG := http.NewServeMux()
	muxG.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"data":{"enterprise":{"members":{"nodes":[{"login":"user1","name":"User One","createdAt":"2022-01-01T00:00:00Z","user":{"databaseId":1}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	gSrv := httptest.NewServer(muxG)
	defer gSrv.Close()

	// stub REST server returning a single login event at now
	now := time.Now().UTC().Format(time.RFC3339)
	muxR := http.NewServeMux()
	muxR.HandleFunc("/enterprises/ent/audit-log", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"action":"user.login","actor":"user1","created_at":"%s"}]`, now)
	})
	rSrv := httptest.NewServer(muxR)
	defer rSrv.Close()

	// assemble clients
	restClient := github.NewClient(rSrv.Client())
	baseURL, _ := url.Parse(rSrv.URL + "/")
	restClient.BaseURL = baseURL
	graphClient := githubv4.NewEnterpriseClient(gSrv.URL+"/graphql", gSrv.Client())

	// run report
	out := filepath.Join(t.TempDir(), "users.csv")
	err := UsersReport(context.Background(), restClient, graphClient, "ent", out)
	require.NoError(t, err)

	// verify CSV contents
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "ID,Login,Name,Email,Last Login(90 days),Dormant?", lines[0])
	expected := fmt.Sprintf("1,user1,User One,N/A,%s,false", now)
	assert.Equal(t, expected, lines[1])
}
