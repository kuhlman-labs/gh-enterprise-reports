package api

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
)

// RESTRateLimitThreshold is the minimum remaining calls before waiting.
const RESTRateLimitThreshold = 15

// AuditLogRateLimitThreshold is the minimum remaining calls before waiting.
const AuditLogRateLimitThreshold = 15

// GraphQLRateLimitThreshold is the minimum remaining points before waiting.
const GraphQLRateLimitThreshold = 100

// checkRateLimit fetches the current rate limits for the client.
func checkRateLimit(ctx context.Context, client *github.Client) (*github.RateLimits, error) {
	var rl *github.RateLimits
	var err error

	// Retry logic for rate limit checks.
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during rate limit check: %w", ctx.Err())
		default:
			rl, _, err = client.RateLimit.Get(ctx)
			if err == nil {
				return rl, nil
			}
			slog.Warn("retrying rate limit check", "error", err, "attempt", i+1)
			time.Sleep(2 * time.Second)
		}
	}
	return nil, fmt.Errorf("failed to fetch rate limits after retries: %w", err)
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
			"reset_time", resetTime.UTC().Format(time.RFC3339),
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
func handleRESTRateLimit(ctx context.Context, rate github.Rate) {
	if rate.Remaining < RESTRateLimitThreshold {
		slog.Warn("rest rate limit low", "remaining", rate.Remaining, "limit", rate.Limit)
		waitForLimitReset(ctx, "rest", rate.Remaining, rate.Limit, rate.Reset.Time)
	}
}

// EnsureRateLimits checks the REST, GraphQL, and Audit Log rate limits and waits if limits are low.
func EnsureRateLimits(ctx context.Context, restClient *github.Client) {
	rl, err := checkRateLimit(ctx, restClient)
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

// MonitorRateLimits periodically checks and logs REST, GraphQL, and Audit Log API rate limits.
func MonitorRateLimits(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, interval time.Duration) {
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
			rateLimits, err := checkRateLimit(ctx, restClient)
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
func getRemaining(rl *github.Rate) int {
	if rl == nil {
		return 0
	}
	return rl.Remaining
}

func getLimit(rl *github.Rate) int {
	if rl == nil {
		return 0
	}
	return rl.Limit
}

func getResetTime(rl *github.Rate) string {
	if rl == nil {
		return "N/A"
	}
	return rl.Reset.Time.UTC().Format(time.RFC3339)
}
