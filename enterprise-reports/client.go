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
