package enterprisereports

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
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
			log.Warn().Err(err).Msgf("Retrying rate limit check (%d/3)", i+1)
			time.Sleep(2 * time.Second)
		}
	}
	return nil, fmt.Errorf("failed to fetch rate limits after retries: %w", err)
}

// waitForLimitReset waits until the given limit resets.
// It logs the wait duration formatted as minutes and seconds and the reset time in UTC.
func waitForLimitReset(ctx context.Context, name string, remaining int, limit int, resetTime time.Time) {
	now := time.Now().UTC() // Ensure UTC
	waitDuration := resetTime.Sub(now) + time.Second
	if waitDuration > 0 {
		log.Warn().
			Str("API", name).
			Int("Remaining", remaining).
			Int("Limit", limit).
			Str("WaitDuration", waitDuration.Truncate(time.Second).String()).
			Str("ResetTime (UTC)", resetTime.Format(time.RFC3339)).
			Msgf("%s rate limit low, waiting until reset", name)

		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			log.Info().Msgf("Context canceled, stopping wait for %s rate limit reset", name)
			return
		case <-timer.C:
			log.Info().Msgf("%s rate limit reset", name)
			return
		}
	}
}

// EnsureRateLimits checks the REST, GraphQL, and Audit Log rate limits and waits if limits are low.
func EnsureRateLimits(ctx context.Context, restClient *github.Client) {
	rl, err := checkRateLimit(ctx, restClient)
	if err != nil {
		log.Error().Err(err).Msg("Error fetching rate limits")
		return
	}

	if core := rl.GetCore(); core != nil && core.Remaining < RESTRateLimitThreshold {
		waitForLimitReset(ctx, "REST", core.Remaining, core.Limit, core.Reset.Time)
	}

	if gql := rl.GetGraphQL(); gql != nil && gql.Remaining < GraphQLRateLimitThreshold {
		waitForLimitReset(ctx, "GraphQL", gql.Remaining, gql.Limit, gql.Reset.Time)
	}

	if audit := rl.GetAuditLog(); audit != nil && audit.Remaining < AuditLogRateLimitThreshold {
		waitForLimitReset(ctx, "Audit Log", audit.Remaining, audit.Limit, audit.Reset.Time)
	}
}

// MonitorRateLimits periodically checks and logs REST, GraphQL, and Audit Log API rate limits.
func MonitorRateLimits(ctx context.Context, restClient *github.Client, graphQLClient *githubv4.Client, interval time.Duration) {
	log.Info().Dur("Interval", interval).Msg("Starting rate limit monitoring...")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Rate limit monitoring stopped")
			return
		case <-ticker.C:
			rateLimits, err := checkRateLimit(ctx, restClient)
			if err != nil {
				log.Error().Err(err).Msg("Error fetching REST API rate limits")
				continue
			}

			core := rateLimits.GetCore()
			gql := rateLimits.GetGraphQL()
			audit := rateLimits.GetAuditLog()

			msg := fmt.Sprintf(
				"Rate Limits | REST: %d/%d (Reset: %s) | GraphQL: %d/%d (Reset: %s) | Audit Log: %d/%d (Reset: %s)",
				getRemaining(core), getLimit(core), getResetTime(core),
				getRemaining(gql), getLimit(gql), getResetTime(gql),
				getRemaining(audit), getLimit(audit), getResetTime(audit),
			)
			log.Info().Msg(msg)
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
