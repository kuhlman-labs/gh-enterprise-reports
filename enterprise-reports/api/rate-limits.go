// Package api provides functionality for interacting with GitHub's REST and GraphQL APIs.
// It includes rate limiting, client wrapper methods, and utilities for efficient API consumption.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
)

// RESTRateLimitThreshold is the minimum remaining calls before waiting.
const RESTRateLimitThreshold = 15

// AuditLogRateLimitThreshold is the minimum remaining calls before waiting.
const AuditLogRateLimitThreshold = 15

// GraphQLRateLimitThreshold is the minimum remaining points before waiting.
const GraphQLRateLimitThreshold = 100

// rateLimiter is an interface for the rate limit service.
type rateLimiter interface {
	// Get retrieves the rate limits for the GitHub API.
	Get(ctx context.Context) (*github.RateLimits, *github.Response, error)
}

// checkRateLimit checks the GitHub API rate limits using the provided service.
// It attempts to handle transient errors by retrying up to maxRetries times.
func checkRateLimit(ctx context.Context, rlService rateLimiter) (*github.RateLimits, error) {
	const maxRetries = 3
	var rl *github.RateLimits
	var err error
	for i := 0; i < maxRetries; i++ {
		rl, _, err = rlService.Get(ctx)
		if err == nil {
			return rl, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during rate limit check: %w", ctx.Err())
		default:
			slog.Warn("retrying rate limit check", "error", err, "attempt", i)
			time.Sleep(2 * time.Second)
		}
	}
	return nil, err
}

// waitForLimitReset waits until the given limit resets.
// It logs the wait duration formatted as minutes and seconds and the reset time in UTC.
func waitForLimitReset(ctx context.Context, name string, remaining, limit int, resetTime time.Time) {
	now := time.Now().UTC() // Ensure UTC
	waitDuration := resetTime.Sub(now) + time.Second
	if waitDuration > 0 {
		slog.Warn("rate limit low, waiting until reset",
			"api", name,
			"remaining", remaining,
			"limit", limit,
			"wait_duration", waitDuration.Truncate(time.Second).String(),
			"reset_time", resetTime.Format(time.RFC3339),
		)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			slog.Info("context canceled, stopping wait for rate limit reset", "api", name)
			return
		case <-timer.C:
			slog.Info("rate limit reset", "api", name)
			return
		}
	}
}

// handleRESTRateLimit logs a warning and waits if the REST rate limit is below the threshold.
func handleRESTRateLimit(ctx context.Context, rate *github.Rate) {
	if rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rest rate limit low", "remaining", rate.Remaining, "limit", rate.Limit)
		waitForLimitReset(ctx, "rest", rate.Remaining, rate.Limit, rate.Reset.Time)
	}
}

// handleGraphQLRateLimit logs a warning and waits if the GraphQL rate limit is below the threshold.
func handleGraphQLRateLimit(ctx context.Context, rate *rateLimitQuery) {
	if rate.Remaining < GraphQLRateLimitThreshold {
		slog.Warn("graphql rate limit low", "remaining", rate.Remaining, "limit", rate.Limit)
		waitForLimitReset(ctx, "graphql", rate.Remaining, rate.Limit, rate.ResetAt.Time)
	}
}

// EnsureRateLimits checks the REST, GraphQL, and Audit Log rate limits and waits if limits are low.
func EnsureRateLimits(ctx context.Context, restClient *github.Client) {
	rl, err := checkRateLimit(ctx, rateLimiter(restClient.RateLimit))
	if err != nil {
		return // Error already logged in checkRateLimit
	}

	if core := rl.GetCore(); core != nil && core.Remaining < RESTRateLimitThreshold {
		waitForLimitReset(ctx, "rest", core.Remaining, core.Limit, core.Reset.Time)
	}

	if gql := rl.GetGraphQL(); gql != nil && gql.Remaining < GraphQLRateLimitThreshold {
		waitForLimitReset(ctx, "graphql", gql.Remaining, gql.Limit, gql.Reset.Time)
	}

	if audit := rl.GetAuditLog(); audit != nil && audit.Remaining < AuditLogRateLimitThreshold {
		waitForLimitReset(ctx, "audit_log", audit.Remaining, audit.Limit, audit.Reset.Time)
	}
}

// MonitorRateLimits periodically checks and logs GitHub API rate limits.
// It takes a rateLimiter for REST API rate limits, a GraphQL client, and a checking interval.
func MonitorRateLimits(ctx context.Context, restSvc rateLimiter, graphQLClient *githubv4.Client, interval time.Duration) {
	slog.Info("starting rate limit monitoring", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	type rateInfo struct {
		name  string
		used  int
		max   int
		reset string
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("rate limit monitoring stopped")
			return
		case <-ticker.C:
			rateLimits, err := checkRateLimit(ctx, restSvc)
			if err != nil {
				slog.Warn("error fetching rest api rate limits", "error", err)
				continue
			}

			core := rateLimits.GetCore()
			gql := rateLimits.GetGraphQL()
			audit := rateLimits.GetAuditLog()

			rates := []rateInfo{
				{
					name:  "rest",
					used:  getLimit(core) - getRemaining(core),
					max:   getLimit(core),
					reset: getResetTime(core),
				},
				{
					name:  "graphql",
					used:  getLimit(gql) - getRemaining(gql),
					max:   getLimit(gql),
					reset: getResetTime(gql),
				},
				{
					name:  "audit_log",
					used:  getLimit(audit) - getRemaining(audit),
					max:   getLimit(audit),
					reset: getResetTime(audit),
				},
			}

			var kv []any
			for _, rate := range rates {
				// e.g. "rest" -> "123/5000"
				kv = append(kv, rate.name, fmt.Sprintf("%d/%d", rate.used, rate.max))
				// e.g. "rest_reset" -> "2025-04-20T12:34:56Z"
				kv = append(kv, rate.name+"_reset", rate.reset)
			}

			slog.Info("rate limits", kv...)
		}
	}
}

// Helper functions to safely access rate limit fields.

// getRemaining safely returns the Remaining value from a Rate.
// Returns 0 if the Rate is nil.
func getRemaining(rl *github.Rate) int {
	if rl == nil {
		return 0
	}
	return rl.Remaining
}

// getLimit safely returns the Limit value from a Rate.
// Returns 0 if the Rate is nil.
func getLimit(rl *github.Rate) int {
	if rl == nil {
		return 0
	}
	return rl.Limit
}

// getResetTime safely returns the Reset time from a Rate as a formatted string.
// Returns "N/A" if the Rate is nil.
func getResetTime(rl *github.Rate) string {
	if rl == nil {
		return "N/A"
	}
	return rl.Reset.Time.Format(time.RFC3339)
}
