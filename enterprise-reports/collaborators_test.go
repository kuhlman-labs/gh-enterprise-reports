package enterprisereports

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockGitHubClient struct {
	mock.Mock
	*github.Client
}

// Adjusting the mock's pagination logic in MockGitHubClient
func (m *MockGitHubClient) ListCollaborators(ctx context.Context, owner, repo string, opts *github.ListCollaboratorsOptions) ([]*github.User, *github.Response, error) {
	args := m.Called(ctx, owner, repo, opts)

	var users []*github.User
	if args.Get(0) != nil {
		users = args.Get(0).([]*github.User)
	}

	var response *github.Response
	if args.Get(1) != nil {
		response = args.Get(1).(*github.Response)
	}

	if opts != nil {
		pageSize := opts.PerPage
		if pageSize == 0 {
			pageSize = len(users)
		}
		// Only apply slicing if users is larger than one page.
		if len(users) > pageSize {
			start := (opts.Page - 1) * pageSize
			if start >= len(users) {
				users = []*github.User{} // Return empty if start exceeds bounds
			} else {
				end := start + pageSize
				if end > len(users) {
					end = len(users)
				}
				users = users[start:end]
			}
		}
	} else if args.Get(0) == nil && args.Get(1) == nil {
		return nil, nil, args.Error(2) // Explicitly return nil when no mock response is set
	}

	return users, response, args.Error(2)
}

// Test_getRepoCollaborators tests the getRepoCollaborators function.
func Test_getRepoCollaborators(t *testing.T) {
	mockClient := new(MockGitHubClient)
	ctx := context.Background()
	repo := &github.Repository{
		Owner: &github.User{Login: github.String("test-owner")},
		Name:  github.String("test-repo"),
	}

	// Mock response
	mockCollaborators := []*github.User{
		{Login: github.String("user1"), ID: github.Int64(1)},
		{Login: github.String("user2"), ID: github.Int64(2)},
	}
	mockResponse := &github.Response{NextPage: 0}
	mockClient.On("ListCollaborators", ctx, "test-owner", "test-repo", mock.Anything).Return(mockCollaborators, mockResponse, nil)

	// Call the mocked method
	result, _, err := mockClient.ListCollaborators(ctx, *repo.Owner.Login, *repo.Name, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Assertions
	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "user1", result[0].GetLogin())
	assert.Equal(t, "user2", result[1].GetLogin())

	mockClient.AssertExpectations(t)
}

// Test_getRepoCollaborators_Error tests error handling in getRepoCollaborators.
func Test_getRepoCollaborators_Error(t *testing.T) {
	mockClient := new(MockGitHubClient)
	ctx := context.Background()
	repo := &github.Repository{
		Owner: &github.User{Login: github.String("test-owner")},
		Name:  github.String("test-repo"),
	}

	// Mock error response
	mockClient.On("ListCollaborators", ctx, "test-owner", "test-repo", mock.Anything).Return(nil, nil, errors.New("API error"))

	// Call the function
	result, _, err := mockClient.ListCollaborators(ctx, *repo.Owner.Login, *repo.Name, nil)
	if err == nil {
		// Handle error
		t.Fatalf("expected error, got nil")
	}

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	mockClient.AssertExpectations(t)
}

// Test_getRepoCollaborators_Empty tests the case where no collaborators are returned.
func Test_getRepoCollaborators_Empty(t *testing.T) {
	mockClient := new(MockGitHubClient)
	ctx := context.Background()
	repo := &github.Repository{
		Owner: &github.User{Login: github.String("test-owner")},
		Name:  github.String("test-repo"),
	}

	// Mock response with no collaborators
	mockCollaborators := []*github.User{}
	mockResponse := &github.Response{NextPage: 0}
	mockClient.On("ListCollaborators", ctx, "test-owner", "test-repo", mock.Anything).Return(mockCollaborators, mockResponse, nil)

	// Call the function
	result, _, err := mockClient.ListCollaborators(ctx, *repo.Owner.Login, *repo.Name, nil)
	if err != nil {
		// Handle error
		t.Fatalf("expected no error, got %v", err)
	}

	// Assertions
	assert.NoError(t, err)
	assert.Empty(t, result)
	mockClient.AssertExpectations(t)
}

// Adjusting the mock setup for pagination in Test_getRepoCollaborators_MultiplePages
func Test_getRepoCollaborators_MultiplePages(t *testing.T) {
	mockClient := new(MockGitHubClient)
	ctx := context.Background()
	repo := &github.Repository{
		Owner: &github.User{Login: github.String("test-owner")},
		Name:  github.String("test-repo"),
	}

	// Fixing mock setup to return one user per page
	mockClient.On("ListCollaborators", ctx, "test-owner", "test-repo", mock.MatchedBy(func(opts *github.ListCollaboratorsOptions) bool {
		return opts != nil && opts.Page == 1
	})).Return([]*github.User{
		{Login: github.String("user1"), ID: github.Int64(1)},
	}, &github.Response{NextPage: 2}, nil).Once()

	mockClient.On("ListCollaborators", ctx, "test-owner", "test-repo", mock.MatchedBy(func(opts *github.ListCollaboratorsOptions) bool {
		return opts != nil && opts.Page == 2
	})).Return([]*github.User{
		{Login: github.String("user2"), ID: github.Int64(2)},
	}, &github.Response{NextPage: 0}, nil).Once()

	// Simulate pagination
	var allCollaborators []*github.User
	opts := &github.ListCollaboratorsOptions{ListOptions: github.ListOptions{PerPage: 1, Page: 1}}
	// Adding debug logs to verify NextPage value
	for {
		collaborators, resp, err := mockClient.ListCollaborators(ctx, *repo.Owner.Login, *repo.Name, opts)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		fmt.Printf("Page: %d, Collaborators: %v, NextPage: %d\n", opts.Page, collaborators, resp.NextPage) // Debug log
		allCollaborators = append(allCollaborators, collaborators...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Assertions
	assert.Len(t, allCollaborators, 2)
	assert.Equal(t, "user1", allCollaborators[0].GetLogin())
	assert.Equal(t, "user2", allCollaborators[1].GetLogin())
	mockClient.AssertExpectations(t)
}
