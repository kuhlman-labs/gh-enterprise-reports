// Package utils provides utility functions and types for the GitHub Enterprise Reports application.
package utils

import (
	"sync"

	"github.com/google/go-github/v70/github"
)

// SharedCache provides a thread-safe store for commonly fetched GitHub data
// to avoid duplicate API calls across different reports.
type SharedCache struct {
	mu                     sync.RWMutex
	enterpriseOrgs         []*github.Organization
	enterpriseUsers        []*github.User
	orgRepositories        map[string][]*github.Repository
	orgMembers             map[string][]*github.User
	orgTeams               map[string][]*github.Team
	repoTeams              map[string][]*github.Team
	repoCollaborators      map[string][]*github.User
	teamMembers            map[string][]*github.User
	enterpriseOrgsFetched  bool
	enterpriseUsersFetched bool
}

// NewSharedCache creates a new shared cache for GitHub data
func NewSharedCache() *SharedCache {
	return &SharedCache{
		orgRepositories:   make(map[string][]*github.Repository),
		orgMembers:        make(map[string][]*github.User),
		orgTeams:          make(map[string][]*github.Team),
		repoTeams:         make(map[string][]*github.Team),
		repoCollaborators: make(map[string][]*github.User),
		teamMembers:       make(map[string][]*github.User),
	}
}

// GetEnterpriseOrgs returns cached enterprise organizations or false if not cached
func (c *SharedCache) GetEnterpriseOrgs() ([]*github.Organization, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enterpriseOrgs, c.enterpriseOrgsFetched
}

// SetEnterpriseOrgs caches enterprise organizations
func (c *SharedCache) SetEnterpriseOrgs(orgs []*github.Organization) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enterpriseOrgs = orgs
	c.enterpriseOrgsFetched = true
}

// GetEnterpriseUsers returns cached enterprise users or false if not cached
func (c *SharedCache) GetEnterpriseUsers() ([]*github.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enterpriseUsers, c.enterpriseUsersFetched
}

// SetEnterpriseUsers caches enterprise users
func (c *SharedCache) SetEnterpriseUsers(users []*github.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enterpriseUsers = users
	c.enterpriseUsersFetched = true
}

// GetOrgRepositories returns cached repositories for an organization or false if not cached
func (c *SharedCache) GetOrgRepositories(orgName string) ([]*github.Repository, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	repos, exists := c.orgRepositories[orgName]
	return repos, exists
}

// SetOrgRepositories caches repositories for an organization
func (c *SharedCache) SetOrgRepositories(orgName string, repos []*github.Repository) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgRepositories[orgName] = repos
}

// GetOrgMembers returns cached members for an organization or false if not cached
func (c *SharedCache) GetOrgMembers(orgName string) ([]*github.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	members, exists := c.orgMembers[orgName]
	return members, exists
}

// SetOrgMembers caches members for an organization
func (c *SharedCache) SetOrgMembers(orgName string, members []*github.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgMembers[orgName] = members
}

// GetOrgTeams returns cached teams for an organization or false if not cached
func (c *SharedCache) GetOrgTeams(orgName string) ([]*github.Team, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	teams, exists := c.orgTeams[orgName]
	return teams, exists
}

// SetOrgTeams caches teams for an organization
func (c *SharedCache) SetOrgTeams(orgName string, teams []*github.Team) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgTeams[orgName] = teams
}

// GetRepoTeams returns cached teams for a repository or false if not cached
func (c *SharedCache) GetRepoTeams(repoFullName string) ([]*github.Team, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	teams, exists := c.repoTeams[repoFullName]
	return teams, exists
}

// SetRepoTeams caches teams for a repository
func (c *SharedCache) SetRepoTeams(repoFullName string, teams []*github.Team) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repoTeams[repoFullName] = teams
}

// GetRepoCollaborators returns cached collaborators for a repository or false if not cached
func (c *SharedCache) GetRepoCollaborators(repoFullName string) ([]*github.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	collaborators, exists := c.repoCollaborators[repoFullName]
	return collaborators, exists
}

// SetRepoCollaborators caches collaborators for a repository
func (c *SharedCache) SetRepoCollaborators(repoFullName string, collaborators []*github.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repoCollaborators[repoFullName] = collaborators
}

// GetTeamMembers returns cached members for a team or false if not cached
func (c *SharedCache) GetTeamMembers(teamKey string) ([]*github.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	members, exists := c.teamMembers[teamKey]
	return members, exists
}

// SetTeamMembers caches members for a team
func (c *SharedCache) SetTeamMembers(teamKey string, members []*github.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.teamMembers[teamKey] = members
}
