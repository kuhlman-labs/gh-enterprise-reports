// Package config provides configuration interfaces and implementations for the GitHub Enterprise Reports tool.
package config

import (
	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
)

// Provider defines an interface for accessing configuration values.
// This creates a clean abstraction for configuration that can be implemented
// by different configuration sources.
type Provider interface {
	// Core configuration methods
	GetEnterpriseSlug() string
	GetWorkers() int
	GetOutputFormat() string
	GetOutputDir() string
	GetLogLevel() string
	GetBaseURL() string

	// Report selection methods
	ShouldRunOrganizationsReport() bool
	ShouldRunRepositoriesReport() bool
	ShouldRunTeamsReport() bool
	ShouldRunCollaboratorsReport() bool
	ShouldRunUsersReport() bool
	ShouldRunActiveRepositoriesReport() bool

	// Authentication methods
	GetAuthMethod() string
	GetToken() string
	GetAppID() int64
	GetAppPrivateKeyFile() string
	GetAppInstallationID() int64

	// Utility methods
	CreateFilePath(reportType string) string
	Validate() error
	CreateRESTClient() (*github.Client, error)
	CreateGraphQLClient() (*githubv4.Client, error)
}
