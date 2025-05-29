// Package config provides configuration interfaces and implementations for the GitHub Enterprise Reports tool.
package config

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// StandardProvider implements the Provider interface using an internal configuration structure.
// It no longer depends directly on the legacy Config struct.
type StandardProvider struct {
	config *Config
}

// NewStandardProviderWithInternalConfig creates a new StandardProvider using an internal config.
func NewStandardProviderWithInternalConfig(config *Config) *StandardProvider {
	return &StandardProvider{
		config: config,
	}
}

// NewStandardProviderWithConfig creates a new StandardProvider using the internal config structure directly.
// This is an alias for NewStandardProviderWithInternalConfig for better naming clarity.
func NewStandardProviderWithConfig(config *Config) *StandardProvider {
	return &StandardProvider{
		config: config,
	}
}

// GetEnterpriseSlug returns the enterprise slug.
func (p *StandardProvider) GetEnterpriseSlug() string {
	return p.config.EnterpriseSlug
}

// GetWorkers returns the number of workers.
func (p *StandardProvider) GetWorkers() int {
	return p.config.Workers
}

// GetOutputFormat returns the output format.
func (p *StandardProvider) GetOutputFormat() string {
	return p.config.OutputFormat
}

// GetOutputDir returns the output directory.
func (p *StandardProvider) GetOutputDir() string {
	return p.config.OutputDir
}

// GetLogLevel returns the log level.
func (p *StandardProvider) GetLogLevel() string {
	return p.config.LogLevel
}

// GetBaseURL returns the base URL.
func (p *StandardProvider) GetBaseURL() string {
	return p.config.BaseURL
}

// ShouldRunOrganizationsReport returns whether to run the organizations report.
func (p *StandardProvider) ShouldRunOrganizationsReport() bool {
	return p.config.Organizations
}

// ShouldRunRepositoriesReport returns whether to run the repositories report.
func (p *StandardProvider) ShouldRunRepositoriesReport() bool {
	return p.config.Repositories
}

// ShouldRunTeamsReport returns whether to run the teams report.
func (p *StandardProvider) ShouldRunTeamsReport() bool {
	return p.config.Teams
}

// ShouldRunCollaboratorsReport returns whether to run the collaborators report.
func (p *StandardProvider) ShouldRunCollaboratorsReport() bool {
	return p.config.Collaborators
}

// ShouldRunUsersReport returns whether to run the users report.
func (p *StandardProvider) ShouldRunUsersReport() bool {
	return p.config.Users
}

// ShouldRunActiveRepositoriesReport returns whether to run the active repositories report.
func (p *StandardProvider) ShouldRunActiveRepositoriesReport() bool {
	return p.config.ActiveRepositories
}

// GetAuthMethod returns the authentication method.
func (p *StandardProvider) GetAuthMethod() string {
	return p.config.AuthMethod
}

// GetToken returns the token.
func (p *StandardProvider) GetToken() string {
	return p.config.Token
}

// GetAppID returns the GitHub App ID.
func (p *StandardProvider) GetAppID() int64 {
	return p.config.GithubAppID
}

// GetAppPrivateKeyFile returns the GitHub App private key file.
func (p *StandardProvider) GetAppPrivateKeyFile() string {
	return p.config.GithubAppPrivateKey
}

// GetAppInstallationID returns the GitHub App installation ID.
func (p *StandardProvider) GetAppInstallationID() int64 {
	return p.config.GithubAppInstallationID
}

// CreateFilePath creates a file path for a report.
func (p *StandardProvider) CreateFilePath(reportType string) string {
	timestamp := time.Now().Format("2006-01-02_15-04")
	filename := fmt.Sprintf("%s_%s_%s.%s", p.GetEnterpriseSlug(), reportType, timestamp, p.GetOutputFormat())
	return filepath.Join(p.GetOutputDir(), filename)
}

// Validate validates the configuration.
func (p *StandardProvider) Validate() error {
	return p.config.Validate()
}

// CreateRESTClient creates a GitHub REST client.
func (p *StandardProvider) CreateRESTClient() (*github.Client, error) {
	ctx := context.Background()
	var client *github.Client

	switch p.GetAuthMethod() {
	case "token":
		if p.GetToken() == "" {
			return nil, fmt.Errorf("token required for token authentication")
		}
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: p.GetToken()},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
	case "app":
		// GitHub App authentication
		if p.GetAppID() == 0 {
			return nil, fmt.Errorf("app-id is required for GitHub App authentication")
		}
		if p.GetAppPrivateKeyFile() == "" {
			return nil, fmt.Errorf("app-private-key-file is required for GitHub App authentication")
		}
		if p.GetAppInstallationID() == 0 {
			return nil, fmt.Errorf("app-installation-id is required for GitHub App authentication")
		}

		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			p.GetAppID(),
			p.GetAppInstallationID(),
			p.GetAppPrivateKeyFile(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub App installation transport: %w", err)
		}
		client = github.NewClient(&http.Client{Transport: itr})
	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", p.GetAuthMethod())
	}

	// Set custom base URL if provided
	if p.GetBaseURL() != "" {
		baseURL, err := url.Parse(p.GetBaseURL())
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		client.BaseURL = baseURL
	}

	return client, nil
}

// CreateGraphQLClient creates a GitHub GraphQL client.
func (p *StandardProvider) CreateGraphQLClient() (*githubv4.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var client *githubv4.Client

	switch p.GetAuthMethod() {
	case "token":
		if p.GetToken() == "" {
			return nil, fmt.Errorf("token required for token authentication")
		}
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.GetToken()})
		httpClient := oauth2.NewClient(ctx, src)

		// If a custom base URL is specified, use it with the GraphQL client
		if p.GetBaseURL() != "" {
			baseURL, err := url.Parse(p.GetBaseURL())
			if err != nil {
				return nil, fmt.Errorf("invalid base URL: %w", err)
			}

			// Construct GraphQL API endpoint (usually the REST API base URL with /graphql appended)
			graphqlURL := fmt.Sprintf("%s://%s/graphql", baseURL.Scheme, baseURL.Host)
			client = githubv4.NewEnterpriseClient(graphqlURL, httpClient)
		} else {
			// Use the standard GitHub GraphQL endpoint
			client = githubv4.NewClient(httpClient)
		}

	case "app":
		// GitHub App authentication
		if p.GetAppID() == 0 {
			return nil, fmt.Errorf("app-id is required for GitHub App authentication")
		}
		if p.GetAppPrivateKeyFile() == "" {
			return nil, fmt.Errorf("app-private-key-file is required for GitHub App authentication")
		}
		if p.GetAppInstallationID() == 0 {
			return nil, fmt.Errorf("app-installation-id is required for GitHub App authentication")
		}

		itr, err := ghinstallation.NewKeyFromFile(
			http.DefaultTransport,
			p.GetAppID(),
			p.GetAppInstallationID(),
			p.GetAppPrivateKeyFile(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub App installation transport: %w", err)
		}
		httpClient := &http.Client{Transport: itr}

		// Handle custom base URL
		if p.GetBaseURL() != "" {
			baseURL, err := url.Parse(p.GetBaseURL())
			if err != nil {
				return nil, fmt.Errorf("invalid base URL: %w", err)
			}
			graphqlURL := fmt.Sprintf("%s://%s/graphql", baseURL.Scheme, baseURL.Host)
			client = githubv4.NewEnterpriseClient(graphqlURL, httpClient)
		} else {
			client = githubv4.NewClient(httpClient)
		}

	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", p.GetAuthMethod())
	}

	return client, nil
}
