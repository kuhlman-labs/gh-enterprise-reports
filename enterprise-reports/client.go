// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
// It handles configuration parsing, client initialization, and report generation for organizations,
// repositories, teams, collaborators, and users across a GitHub Enterprise instance.
package enterprisereports

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// NewRESTClient creates a new REST client configured based on the chosen authentication method.
// It supports both personal access token and GitHub App authentication methods.
//
// Parameters:
//   - ctx: Context for authentication operations
//   - conf: Configuration containing auth method and credentials
//
// Returns a configured GitHub REST API client or an error if authentication fails.
// For token authentication, it uses the provided token directly.
// For GitHub App authentication, it loads the private key from the specified file.
func NewRESTClient(ctx context.Context, conf *Config) (*github.Client, error) {
	switch conf.AuthMethod {
	case "token":
		client := github.NewClient(nil).WithAuthToken(conf.Token)
		return client, nil
	case "app":
		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			conf.GithubAppID,
			conf.GithubAppInstallationID,
			conf.GithubAppPrivateKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create installation transport: %w", err)
		}
		client := github.NewClient(&http.Client{Transport: itr})
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported auth-method %q: please use 'token' or 'app'", conf.AuthMethod)
	}
}

// NewGraphQLClient creates a new GraphQL client configured based on the chosen authentication method.
// It supports both personal access token and GitHub App authentication methods.
//
// Parameters:
//   - ctx: Context for authentication operations with a 30-second timeout
//   - conf: Configuration containing auth method and credentials
//
// Returns a configured GitHub GraphQL API client or an error if authentication fails.
// For token authentication, it uses oauth2 token source.
// For GitHub App authentication, it loads the private key from the specified file.
func NewGraphQLClient(ctx context.Context, conf *Config) (*githubv4.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch conf.AuthMethod {
	case "token":
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: conf.Token})
		httpClient := oauth2.NewClient(ctx, src)
		client := githubv4.NewClient(httpClient)
		return client, nil
	case "app":
		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			conf.GithubAppID,
			conf.GithubAppInstallationID,
			conf.GithubAppPrivateKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create installation transport: %w", err)
		}
		httpClient := &http.Client{Transport: itr}
		client := githubv4.NewClient(httpClient)
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported auth-method %q: please use 'token' or 'app'", conf.AuthMethod)
	}
}
