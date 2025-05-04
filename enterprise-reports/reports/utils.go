// Package reports implements various report generation functionalities for GitHub Enterprise.
// It provides utilities and specific report types for organizations, repositories, teams,
// collaborators, and user data, with results exported as CSV files.
package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/api"
	"github.com/shurcooL/githubv4"
	"golang.org/x/time/rate"
)

// getHighestPermission returns the highest permission level from the provided permissions map.
// The permission hierarchy (from highest to lowest) is: admin, maintain, push, triage, pull, none.
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
// and the path itself is non‐empty, non‐absolute, and contains no parent refs.
// It returns an error if the path is invalid or if the parent directory doesn't exist.
func validateFilePath(path string) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid file path: contains parent directory reference")
	}
	// if it's a relative path, ensure it doesn't climb above cwd
	if !filepath.IsAbs(cleanPath) {
		if rel, err := filepath.Rel(".", cleanPath); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("file path escapes working directory: %s", cleanPath)
		}
	}
	// now ensure the parent directory exists
	dir := filepath.Dir(cleanPath)
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
// The path is first validated using validateFilePath.
// Returns an error if the file cannot be created or if writing the header fails.
func createCSVFileWithHeader(path string, header []string) (*os.File, *csv.Writer, error) {
	// validate the path first
	if err := validateFilePath(path); err != nil {
		return nil, nil, err
	}

	// #nosec G304  // safe: path has been validated by validateFilePath
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSV file %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		if err := f.Close(); err != nil {
			slog.Error("failed to close file after header write error", "error", err)
		}
		return nil, nil, fmt.Errorf("failed to write header to file %s: %w", path, err)
	}
	return f, w, nil
}

// isDormant determines if a user is dormant by verifying events, contributions, and recent login activity.
// A user is considered dormant if they have no recent events, no recent contributions,
// and no recent login activity within the specified time period.
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

// ProcessorFunc processes an input item to an output record.
// This is a generic function type used by the RunReport function to process items concurrently.
type ProcessorFunc[I any, O any] func(ctx context.Context, item I) (O, error)

// FormatterFunc formats an output record into a CSV row.
// This is a generic function type used by the RunReport function to format processed items into CSV rows.
type FormatterFunc[O any] func(output O) []string

// RunReport processes items concurrently using the provided processor and formatter,
// writing results to a CSV file with the given header. It respects the provided rate limiter.
//
// Parameters:
//   - ctx: Context for cancellation
//   - items: Slice of input items to process
//   - processor: Function to process each input item
//   - formatter: Function to format processed items into CSV rows
//   - limiter: Rate limiter to control API request frequency
//   - workerCount: Number of concurrent workers
//   - filename: Path to the output CSV file
//   - header: CSV header row
//
// The function handles graceful cancellation through context and continues processing
// other items when individual item processing fails.
func RunReport[I any, O any](
	ctx context.Context,
	items []I,
	processor ProcessorFunc[I, O],
	formatter FormatterFunc[O],
	limiter *rate.Limiter,
	workerCount int,
	filename string,
	header []string,
) error {
	slog.Info("starting report", slog.String("filename", filename))
	file, writer, err := createCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			slog.Error("failed to close CSV file", "error", cerr)
		}
	}()

	in := make(chan I, len(items))
	out := make(chan O, len(items))
	var wg sync.WaitGroup
	var count int64

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					slog.Warn("context canceled, stopping processing")
					return
				case item, ok := <-in:
					if !ok {
						return
					}

					// Wait for the rate limiter
					err := limiter.Wait(ctx)
					if err != nil {
						slog.Warn("rate limiter wait failed", "error", err)
						// Decide if you want to return or continue based on the error
						// If context is canceled, this will return an error.
						if ctx.Err() != nil {
							return // Context was canceled while waiting
						}
						// Handle other potential limiter errors if necessary, or just log and continue
						continue
					}

					result, err := processor(ctx, item)
					if err != nil {
						slog.Warn("processing item failed", "error", err, "item", item)
						continue // Skip this item on processor error
					}
					atomic.AddInt64(&count, 1)
					// Use a select to prevent blocking indefinitely if the context is canceled
					// while waiting to send to the out channel.
					select {
					case out <- result:
					case <-ctx.Done():
						slog.Warn("context canceled, discarding result")
						return
					}
				}
			}
		}()
	}

	go func() {
	InputLoop: // Labeled break for the outer loop
		for _, item := range items {
			// Use a select to prevent blocking indefinitely if the context is canceled
			// while waiting to send to the in channel.
			select {
			case in <- item:
			case <-ctx.Done():
				slog.Warn("context canceled during input sending, stopping early")
				break InputLoop // Use labeled break to exit the for loop
			}
		}
		close(in) // Close in channel regardless of context cancellation
		wg.Wait()
		slog.Info("processing complete", slog.Int64("total", count))
		close(out)
	}()

	for result := range out {
		row := formatter(result)
		if err := writer.Write(row); err != nil {
			// It might be better to log the error and continue,
			// rather than failing the entire report for one write error.
			slog.Error("failed to write row to CSV", "error", err)
			continue // Continue processing other results
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		// Log the flush error, but the report might still be partially useful.
		slog.Error("failed to flush CSV writer", "error", err)
	}
	slog.Info("report complete", slog.String("filename", filename))
	return writer.Error() // Return the flush error if any occurred
}
