package enterprisereports

import (
	"context"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
	"github.com/shurcooL/githubv4"
)

// RESTRateLimitThreshold is the minimum remaining calls before waiting.
const RESTRateLimitThreshold = 10

// AuditLogRateLimitThreshold is the minimum remaining calls before waiting.
const AuditLogRateLimitThreshold = 10

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

// waitForRESTRateLimitReset waits until the REST rate limit resets.
func waitForRESTRateLimitReset(ctx context.Context, reset *github.RateLimits) {
	now := time.Now()
	waitDuration := reset.GetCore().Reset.Time.Sub(now) + time.Second // add 1 second cushion
	if waitDuration > 0 {
		log.Warn().
			Int("Remaining", reset.GetCore().Remaining).
			Dur("WaitDuration", waitDuration).
			Msg("REST rate limit low, waiting until reset")
		select {
		case <-ctx.Done():
			log.Info().Msg("Context done, stopping wait for REST rate limit reset")
			return
		case <-time.After(waitDuration):
			log.Info().Msg("REST rate limit reset")
			return
		}
	}
}

// waitForGraphQLRateLimitReset waits until the GraphQL rate limit resets.
func waitForGraphQLRateLimitReset(ctx context.Context, reset *github.RateLimits) {
	now := time.Now()
	waitDuration := reset.GetGraphQL().Reset.Time.Sub(now) + time.Second // add 1 second cushion
	if waitDuration > 0 {
		log.Warn().
			Int("Remaining", reset.GetGraphQL().Remaining).
			Dur("WaitDuration", waitDuration).
			Msg("GraphQL rate limit low, waiting until reset")
		select {
		case <-ctx.Done():
			log.Info().Msg("Context done, stopping wait for GraphQL rate limit reset")
			return
		case <-time.After(waitDuration):
			log.Info().Msg("GraphQL rate limit reset")
			return
		}
	}
}

// waitForAuditLogRateLimitReset waits until the Audit Log rate limit resets.
func waitForAuditLogRateLimitReset(ctx context.Context, reset *github.RateLimits) {
	now := time.Now()
	waitDuration := reset.GetAuditLog().Reset.Time.Sub(now) + time.Second // add 1 second cushion
	if waitDuration > 0 {
		log.Warn().
			Int("Remaining", reset.GetAuditLog().Remaining).
			Dur("WaitDuration", waitDuration).
			Msg("Audit rate limit low, waiting until reset")
		select {
		case <-ctx.Done():
			log.Info().Msg("Context done, stopping wait for Audit Log rate limit reset")
			return
		case <-time.After(waitDuration):
			log.Info().Msg("Audit Log rate limit reset")
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

	if rl.GetCore().Remaining < RESTRateLimitThreshold {
		waitForRESTRateLimitReset(ctx, rl)
	}

	if rl.GetGraphQL().Remaining < GraphQLRateLimitThreshold {
		waitForGraphQLRateLimitReset(ctx, rl)
	}

	if rl.GetAuditLog().Remaining < RESTRateLimitThreshold {
		waitForAuditLogRateLimitReset(ctx, rl)
	}
}

// MonitorRateLimits periodically checks the REST and GraphQL rate limits and logs the results.
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
			// Check REST API rate limits.
			rateLimits, err := checkRateLimit(ctx, restClient)
			if err != nil {
				log.Error().Err(err).Msg("Error fetching REST API rate limits")
			} else {
				log.Info().
					Int("Remaining", rateLimits.GetCore().Remaining).
					Int("Limit", rateLimits.GetCore().Limit).
					Time("Reset", rateLimits.GetCore().Reset.Time).
					Msg("REST API rate limits")
				log.Info().
					Int("Remaining", rateLimits.GetGraphQL().Remaining).
					Int("Limit", rateLimits.GetGraphQL().Limit).
					Time("Reset", rateLimits.GetGraphQL().Reset.Time).
					Msg("GraphQL API rate limits")
				log.Info().
					Int("Remaining", rateLimits.GetAuditLog().Remaining).
					Int("Limit", rateLimits.GetAuditLog().Limit).
					Time("Reset", rateLimits.GetAuditLog().Reset.Time).
					Msg("Audit Log API rate limits")
			}
		}
	}
}
