package report

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProvider is a mock implementation of config.Provider for testing.
type MockProvider struct {
	mock.Mock
}

func (m *MockProvider) GetEnterpriseSlug() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetWorkers() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockProvider) GetOutputFormat() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetOutputDir() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetLogLevel() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetBaseURL() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) ShouldRunOrganizationsReport() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ShouldRunRepositoriesReport() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ShouldRunTeamsReport() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ShouldRunCollaboratorsReport() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ShouldRunUsersReport() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) GetAuthMethod() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetToken() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetAppID() int64 {
	args := m.Called()
	return args.Get(0).(int64)
}

func (m *MockProvider) GetAppPrivateKeyFile() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) GetAppInstallationID() int64 {
	args := m.Called()
	return args.Get(0).(int64)
}

func (m *MockProvider) CreateFilePath(reportType string) string {
	args := m.Called(reportType)
	return args.String(0)
}

func (m *MockProvider) Validate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockProvider) CreateRESTClient() (*github.Client, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*github.Client), args.Error(1)
}

func (m *MockProvider) CreateGraphQLClient() (*githubv4.Client, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*githubv4.Client), args.Error(1)
}

// MockReportRunner is a mock implementation of ReportRunner for testing
type MockReportRunner struct {
	mock.Mock
}

func (m *MockReportRunner) Run(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client,
	outputFilename string, workers int, cache *utils.SharedCache) error {
	args := m.Called(ctx, restClient, graphQLClient, outputFilename, workers, cache)
	return args.Error(0)
}

func (m *MockReportRunner) Name() string {
	args := m.Called()
	return args.String(0)
}

func TestNewReportExecutor(t *testing.T) {
	mockProvider := new(MockProvider)
	executor := NewReportExecutor(mockProvider)
	assert.NotNil(t, executor)
	assert.Equal(t, mockProvider, executor.config)
	assert.NotNil(t, executor.cache)
}

func TestReportExecutor_Execute(t *testing.T) {
	// Create a temp directory for outputs
	tmpDir := t.TempDir()

	testCases := []struct {
		name          string
		setupProvider func(*MockProvider)
		runReports    []string
		expectErrors  bool
	}{
		{
			name: "Run all reports successfully",
			setupProvider: func(mp *MockProvider) {
				mp.On("GetWorkers").Return(2)
				mp.On("GetOutputFormat").Return("csv")
				mp.On("GetOutputDir").Return(tmpDir)
				mp.On("GetEnterpriseSlug").Return("test-enterprise")

				mp.On("ShouldRunOrganizationsReport").Return(true)
				mp.On("ShouldRunRepositoriesReport").Return(true)
				mp.On("ShouldRunTeamsReport").Return(true)
				mp.On("ShouldRunCollaboratorsReport").Return(true)
				mp.On("ShouldRunUsersReport").Return(true)

				mp.On("CreateFilePath", "organizations").Return(filepath.Join(tmpDir, "test-enterprise_organizations.csv"))
				mp.On("CreateFilePath", "repositories").Return(filepath.Join(tmpDir, "test-enterprise_repositories.csv"))
				mp.On("CreateFilePath", "teams").Return(filepath.Join(tmpDir, "test-enterprise_teams.csv"))
				mp.On("CreateFilePath", "collaborators").Return(filepath.Join(tmpDir, "test-enterprise_collaborators.csv"))
				mp.On("CreateFilePath", "users").Return(filepath.Join(tmpDir, "test-enterprise_users.csv"))
			},
			runReports:   []string{"organizations", "repositories", "teams", "collaborators", "users"},
			expectErrors: false,
		},
		{
			name: "Run only organizations report",
			setupProvider: func(mp *MockProvider) {
				mp.On("GetWorkers").Return(2)
				mp.On("GetOutputFormat").Return("csv")
				mp.On("GetOutputDir").Return(tmpDir)
				mp.On("GetEnterpriseSlug").Return("test-enterprise")

				mp.On("ShouldRunOrganizationsReport").Return(true)
				mp.On("ShouldRunRepositoriesReport").Return(false)
				mp.On("ShouldRunTeamsReport").Return(false)
				mp.On("ShouldRunCollaboratorsReport").Return(false)
				mp.On("ShouldRunUsersReport").Return(false)

				mp.On("CreateFilePath", "organizations").Return(filepath.Join(tmpDir, "test-enterprise_organizations.csv"))
			},
			runReports:   []string{"organizations"},
			expectErrors: false,
		},
		{
			name: "Run with error in repository report",
			setupProvider: func(mp *MockProvider) {
				mp.On("GetWorkers").Return(2)
				mp.On("GetOutputFormat").Return("csv")
				mp.On("GetOutputDir").Return(tmpDir)
				mp.On("GetEnterpriseSlug").Return("test-enterprise")

				mp.On("ShouldRunOrganizationsReport").Return(false)
				mp.On("ShouldRunRepositoriesReport").Return(true)
				mp.On("ShouldRunTeamsReport").Return(false)
				mp.On("ShouldRunCollaboratorsReport").Return(false)
				mp.On("ShouldRunUsersReport").Return(false)

				mp.On("CreateFilePath", "repositories").Return(filepath.Join(tmpDir, "test-enterprise_repositories.csv"))
			},
			runReports:   []string{"repositories"},
			expectErrors: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock provider
			mockProvider := new(MockProvider)
			tc.setupProvider(mockProvider)

			// Create executor with mock provider
			executor := NewReportExecutor(mockProvider)

			// Set up mock rest and graphql clients
			restClient := &github.Client{}
			graphQLClient := &githubv4.Client{}

			// Create context
			ctx := context.Background()

			// Set up mock report runners for each enabled report
			reportRunners := make(map[string]*MockReportRunner)

			for _, reportName := range tc.runReports {
				mockRunner := new(MockReportRunner)
				mockRunner.On("Name").Return(reportName)

				outputPath := filepath.Join(tmpDir, "test-enterprise_"+reportName+".csv")

				var err error
				if tc.expectErrors && reportName == "repositories" {
					err = utils.NewAppError(utils.ErrorTypeAPI, "test error", nil)
				}

				mockRunner.On("Run",
					ctx,
					restClient,
					graphQLClient,
					outputPath,
					2,
					mock.AnythingOfType("*utils.SharedCache"),
				).Return(err)

				reportRunners[reportName] = mockRunner

				// Replace the actual constructor with our mock
				switch reportName {
				case "organizations":
					originalOrgRunner := NewOrganizationsReportRunner
					NewOrganizationsReportRunner = func(enterpriseSlug string) ReportRunner {
						return mockRunner
					}
					defer func() { NewOrganizationsReportRunner = originalOrgRunner }()
				case "repositories":
					originalRepoRunner := NewRepositoriesReportRunner
					NewRepositoriesReportRunner = func(enterpriseSlug string) ReportRunner {
						return mockRunner
					}
					defer func() { NewRepositoriesReportRunner = originalRepoRunner }()
				case "teams":
					originalTeamsRunner := NewTeamsReportRunner
					NewTeamsReportRunner = func(enterpriseSlug string) ReportRunner {
						return mockRunner
					}
					defer func() { NewTeamsReportRunner = originalTeamsRunner }()
				case "collaborators":
					originalCollabRunner := NewCollaboratorsReportRunner
					NewCollaboratorsReportRunner = func(enterpriseSlug string) ReportRunner {
						return mockRunner
					}
					defer func() { NewCollaboratorsReportRunner = originalCollabRunner }()
				case "users":
					originalUsersRunner := NewUsersReportRunner
					NewUsersReportRunner = func(enterpriseSlug string) ReportRunner {
						return mockRunner
					}
					defer func() { NewUsersReportRunner = originalUsersRunner }()
				}
			}

			// Execute reports
			executor.Execute(ctx, restClient, graphQLClient)

			// Verify all expectations
			for _, mockRunner := range reportRunners {
				mockRunner.AssertExpectations(t)
			}
			mockProvider.AssertExpectations(t)
		})
	}
}
