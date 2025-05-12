// Package config provides configuration interfaces and implementations for the GitHub Enterprise Reports tool.
package config

import (
	"fmt"
	"strings"
)

// Config holds the configuration for the GitHub Enterprise Reports tool.
// It includes options for report types, authentication methods, and output settings.
// The struct is designed to be used with command-line flags and environment variables.
// It provides validation methods to ensure that the configuration is complete and correct.
type Config struct {
	Organizations           bool
	Repositories            bool
	Teams                   bool
	Collaborators           bool
	Users                   bool
	Workers                 int
	AuthMethod              string
	Token                   string
	GithubAppID             int64
	GithubAppPrivateKey     string
	GithubAppInstallationID int64
	EnterpriseSlug          string
	LogLevel                string
	BaseURL                 string
	OutputFormat            string
	OutputDir               string
}

// Validate checks for required flags based on the chosen authentication method.
func (c *Config) Validate() error {
	var errs []error

	// default empty log‐level to "info"
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	// default empty output format to "csv"
	if c.OutputFormat == "" {
		c.OutputFormat = "csv"
	}

	// default output dir to current directory if empty
	if c.OutputDir == "" {
		c.OutputDir = "."
	}

	if c.EnterpriseSlug == "" {
		errs = append(errs, fmt.Errorf("enterprise slug is required"))
	}

	// If no report types are selected, report an error
	if !c.Organizations && !c.Repositories && !c.Teams && !c.Collaborators && !c.Users {
		errs = append(errs, fmt.Errorf("at least one report type must be selected"))
	}

	// Validate authentication method and required parameters
	switch c.AuthMethod {
	case "token":
		if c.Token == "" {
			errs = append(errs, fmt.Errorf("token is required when auth-method=token"))
		}
	case "app":
		if c.GithubAppID == 0 {
			errs = append(errs, fmt.Errorf("github app id is required when auth-method=app"))
		}
		if c.GithubAppPrivateKey == "" {
			errs = append(errs, fmt.Errorf("github app private key file is required when auth-method=app"))
		}
		if c.GithubAppInstallationID == 0 {
			errs = append(errs, fmt.Errorf("github app installation id is required when auth-method=app"))
		}
	default:
		errs = append(errs, fmt.Errorf("invalid auth-method: %q (must be 'token' or 'app')", c.AuthMethod))
	}

	// Default to 5 workers if not specified or negative
	if c.Workers <= 0 {
		c.Workers = 5
	}

	if len(errs) > 0 {
		errStrings := make([]string, len(errs))
		for i, err := range errs {
			errStrings[i] = err.Error()
		}
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errStrings, "; "))
	}

	return nil
}
