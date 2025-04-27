package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
)

// getHighestPermission returns the highest permission level from the provided permissions map.
func getHighestPermission(permissions map[string]bool) string {
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

// validateFilePath ensures the directory for the given file path exists
// and the path itself is non‐empty.
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		return fmt.Errorf("error accessing directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path %s is not a directory", dir)
	}
	return nil
}

// createCSVFileWithHeader creates the CSV file at path, writes the header, and returns the file & writer.
func createCSVFileWithHeader(path string, header []string) (*os.File, *csv.Writer, error) {
	// validate the path first
	if err := validateFilePath(path); err != nil {
		return nil, nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSV file %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		if cerr := f.Close(); cerr != nil {
			slog.Error("failed to close file after header write error", slog.Any("err", cerr))
		}
		return nil, nil, fmt.Errorf("failed to write header to file %s: %w", path, err)
	}
	return f, w, nil
}

// isDormant determines if a user is dormant by verifying events, contributions, and recent login activity.
func isDormant(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, user string, since time.Time, recentLogin bool) (bool, error) {
	slog.Debug("checking dormant status", "user", user)

	// Check for recent REST events.
	recentEvents, err := api.HasRecentEvents(ctx, restClient, user, since)
	if err != nil {
		return false, fmt.Errorf("checking recent events for %q: %w", user, err)
	}

	// Check for recent contributions.
	recentContribs, err := api.HasRecentContributions(ctx, graphQLClient, user, since)
	if err != nil {
		return false, fmt.Errorf("checking recent contributions for %q: %w", user, err)
	}

	// If the user has neither recent events nor contributions, and no recent login, they are dormant.
	dormant := !recentEvents && !recentContribs && !recentLogin

	// Report final dormant check outcome.
	slog.Debug("dormant check result",
		"user", user,
		"recentEvents", recentEvents,
		"recentContribs", recentContribs,
		"recentLogin", recentLogin,
		"dormant", dormant,
	)

	return dormant, nil
}
