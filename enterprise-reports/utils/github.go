// Package utils provides utility functions and types for the GitHub Enterprise Reports application.
package utils

import (
	"log/slog"
)

// GetHighestPermission returns the highest permission level from the provided permissions map.
// The permission hierarchy (from highest to lowest) is: admin, maintain, push, triage, pull, none.
func GetHighestPermission(permissions map[string]bool) string {
	switch {
	case permissions["admin"]:
		return "admin"
	case permissions["maintain"]:
		return "maintain"
	case permissions["push"]:
		return "push"
	case permissions["triage"]:
		return "triage"
	case permissions["pull"]:
		return "pull"
	default:
		return "none"
	}
}

// isDormant determines if a user is dormant by verifying events, contributions, and recent login activity.
// A user is considered dormant if they have no recent events, no recent contributions,
// and no recent login activity within the specified time period.
func IsDormant(user string, recentEvents, recentContributions, recentLogin bool) (bool, error) {
	slog.Debug("checking dormant status", "user", user)

	// If the user has neither recent events nor contributions, and no recent login, they are dormant.
	dormant := !recentEvents && !recentContributions && !recentLogin

	// Report final dormant check outcome.
	slog.Debug("dormant check result",
		"user", user,
		"recentEvents", recentEvents,
		"recentContribs", recentContributions,
		"recentLogin", recentLogin,
		"dormant", dormant,
	)

	return dormant, nil
}
