package config

import (
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Valid token auth",
			config: &Config{
				EnterpriseSlug: "test-enterprise",
				Organizations:  true,
				AuthMethod:     "token",
				Token:          "test-token",
			},
			wantErr: false,
		},
		{
			name: "Valid GitHub App auth",
			config: &Config{
				EnterpriseSlug:          "test-enterprise",
				Organizations:           true,
				AuthMethod:              "app",
				GithubAppID:             12345,
				GithubAppPrivateKey:     "private-key.pem",
				GithubAppInstallationID: 67890,
			},
			wantErr: false,
		},
		{
			name: "Missing enterprise slug",
			config: &Config{
				Organizations: true,
				AuthMethod:    "token",
				Token:         "test-token",
			},
			wantErr: true,
		},
		{
			name: "No reports selected",
			config: &Config{
				EnterpriseSlug: "test-enterprise",
				AuthMethod:     "token",
				Token:          "test-token",
			},
			wantErr: true,
		},
		{
			name: "Missing token for token auth",
			config: &Config{
				EnterpriseSlug: "test-enterprise",
				Organizations:  true,
				AuthMethod:     "token",
			},
			wantErr: true,
		},
		{
			name: "Missing app ID for GitHub App auth",
			config: &Config{
				EnterpriseSlug:          "test-enterprise",
				Organizations:           true,
				AuthMethod:              "app",
				GithubAppPrivateKey:     "private-key.pem",
				GithubAppInstallationID: 67890,
			},
			wantErr: true,
		},
		{
			name: "Missing private key file for GitHub App auth",
			config: &Config{
				EnterpriseSlug:          "test-enterprise",
				Organizations:           true,
				AuthMethod:              "app",
				GithubAppID:             12345,
				GithubAppInstallationID: 67890,
			},
			wantErr: true,
		},
		{
			name: "Missing installation ID for GitHub App auth",
			config: &Config{
				EnterpriseSlug:      "test-enterprise",
				Organizations:       true,
				AuthMethod:          "app",
				GithubAppID:         12345,
				GithubAppPrivateKey: "private-key.pem",
			},
			wantErr: true,
		},
		{
			name: "Invalid auth method",
			config: &Config{
				EnterpriseSlug: "test-enterprise",
				Organizations:  true,
				AuthMethod:     "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		Organizations:  true,
		AuthMethod:     "token",
		Token:          "test-token",
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Unexpected error validating config: %v", err)
	}

	// Check default values
	if config.LogLevel != "info" {
		t.Errorf("Default log level should be 'info', got %q", config.LogLevel)
	}
	if config.OutputFormat != "csv" {
		t.Errorf("Default output format should be 'csv', got %q", config.OutputFormat)
	}
	if config.OutputDir != "." {
		t.Errorf("Default output directory should be '.', got %q", config.OutputDir)
	}
	if config.Workers != 5 {
		t.Errorf("Default workers should be 5, got %d", config.Workers)
	}
}

func TestNewStandardProviderWithConfig(t *testing.T) {
	config := &Config{
		EnterpriseSlug: "test-enterprise",
		Organizations:  true,
		Repositories:   true,
		Teams:          false,
		Collaborators:  true,
		Users:          false,
		Workers:        10,
		AuthMethod:     "token",
		Token:          "test-token",
		OutputFormat:   "json",
		OutputDir:      "/tmp",
	}

	provider := NewStandardProviderWithConfig(config)

	// Check that config values are properly accessible through the provider
	if provider.GetEnterpriseSlug() != "test-enterprise" {
		t.Errorf("GetEnterpriseSlug() returned %q, want %q", provider.GetEnterpriseSlug(), "test-enterprise")
	}
	if provider.GetWorkers() != 10 {
		t.Errorf("GetWorkers() returned %d, want %d", provider.GetWorkers(), 10)
	}
	if provider.GetOutputFormat() != "json" {
		t.Errorf("GetOutputFormat() returned %q, want %q", provider.GetOutputFormat(), "json")
	}
	if provider.GetOutputDir() != "/tmp" {
		t.Errorf("GetOutputDir() returned %q, want %q", provider.GetOutputDir(), "/tmp")
	}
	if !provider.ShouldRunOrganizationsReport() {
		t.Errorf("ShouldRunOrganizationsReport() returned false, want true")
	}
	if !provider.ShouldRunRepositoriesReport() {
		t.Errorf("ShouldRunRepositoriesReport() returned false, want true")
	}
	if provider.ShouldRunTeamsReport() {
		t.Errorf("ShouldRunTeamsReport() returned true, want false")
	}
	if !provider.ShouldRunCollaboratorsReport() {
		t.Errorf("ShouldRunCollaboratorsReport() returned false, want true")
	}
	if provider.ShouldRunUsersReport() {
		t.Errorf("ShouldRunUsersReport() returned true, want false")
	}
}
