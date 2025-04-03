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
const RESTRateLimitThreshold = 10

// AuditLogRateLimitThreshold is the minimum remaining calls before waiting.
const AuditLogRateLimitThreshold = 20

// GraphQLRateLimitThreshold is the minimum remaining points before waiting.
const GraphQLRateLimitThreshold = 100

// checkRateLimit fetches the current rate limits for the client.
func checkRateLimit(ctx context.Context, client *github.Client) (*github.RateLimits, error) {
	rl, _, err := client.RateLimit.Get(ctx)
	if err != nil {
		return nil, err
	}
	return rl, nil
}

// waitForLimitReset waits until the given limit resets.
// It logs the wait duration formatted as minutes and seconds and the reset time in UTC.
func waitForLimitReset(ctx context.Context, name string, remaining int, limit int, resetTime time.Time) {
	now := time.Now().UTC()
	waitDuration := resetTime.UTC().Sub(now) + time.Second // add 1 second cushion
	if waitDuration > 0 {
		minutes := int(waitDuration.Minutes())
		seconds := int(waitDuration.Seconds()) % 60
		log.Warn().
			Str("API", name).
			Int("Remaining", remaining).
			Int("Limit", limit).
			Str("WaitDuration", fmt.Sprintf("%dm %ds", minutes, seconds)).
			Str("ResetTime (UTC)", resetTime.UTC().Format(time.RFC3339)).
			Msgf("%s rate limit low, waiting until reset", name)
		select {
		case <-ctx.Done():
			log.Info().Msgf("Context done, stopping wait for %s rate limit reset", name)
			return
		case <-time.After(waitDuration):
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

	core := rl.GetCore()
	if core.Remaining < RESTRateLimitThreshold {
		waitForLimitReset(ctx, "REST", core.Remaining, core.Limit, core.Reset.Time)
	}

	gql := rl.GetGraphQL()
	if gql.Remaining < GraphQLRateLimitThreshold {
		waitForLimitReset(ctx, "GraphQL", gql.Remaining, gql.Limit, gql.Reset.Time)
	}

	audit := rl.GetAuditLog()
	if audit.Remaining < AuditLogRateLimitThreshold {
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
			// Check API rate limits.
			rateLimits, err := checkRateLimit(ctx, restClient)
			if err != nil {
				log.Error().Err(err).Msg("Error fetching REST API rate limits")
			} else {
				core := rateLimits.GetCore()
				gql := rateLimits.GetGraphQL()
				audit := rateLimits.GetAuditLog()
				// Build a single formatted message that prints all limits evenly.
				msg := fmt.Sprintf(
					"Rate Limits | REST: %d/%d (Reset: %s) | GraphQL: %d/%d (Reset: %s) | Audit Log: %d/%d (Reset: %s)",
					core.Remaining, core.Limit, core.Reset.Time.UTC().Format(time.RFC3339),
					gql.Remaining, gql.Limit, gql.Reset.Time.UTC().Format(time.RFC3339),
					audit.Remaining, audit.Limit, audit.Reset.Time.UTC().Format(time.RFC3339),
				)
				log.Info().Msg(msg)
			}
		}
	}
}
