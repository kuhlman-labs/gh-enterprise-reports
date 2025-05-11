// Package api provides functionality for interacting with GitHub's REST and GraphQL APIs.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
)

const (
	// DefaultRetryCount is the default number of retries for API calls
	DefaultRetryCount = 3

	// DefaultInitialBackoff is the default initial backoff duration
	DefaultInitialBackoff = 500 * time.Millisecond
)

// RetryableRESTClient wraps the GitHub REST client with retry functionality
type RetryableRESTClient struct {
	Client         *github.Client
	MaxRetries     int
	InitialBackoff time.Duration
}

// RetryableGraphQLClient wraps the GitHub GraphQL client with retry functionality
type RetryableGraphQLClient struct {
	Client         *githubv4.Client
	MaxRetries     int
	InitialBackoff time.Duration
}

// NewRetryableRESTClient creates a new retryable REST client
func NewRetryableRESTClient(client *github.Client, maxRetries int, initialBackoff time.Duration) *RetryableRESTClient {
	if maxRetries <= 0 {
		maxRetries = DefaultRetryCount
	}
	if initialBackoff <= 0 {
		initialBackoff = DefaultInitialBackoff
	}
	return &RetryableRESTClient{
		Client:         client,
		MaxRetries:     maxRetries,
		InitialBackoff: initialBackoff,
	}
}

// NewRetryableGraphQLClient creates a new retryable GraphQL client
func NewRetryableGraphQLClient(client *githubv4.Client, maxRetries int, initialBackoff time.Duration) *RetryableGraphQLClient {
	if maxRetries <= 0 {
		maxRetries = DefaultRetryCount
	}
	if initialBackoff <= 0 {
		initialBackoff = DefaultInitialBackoff
	}
	return &RetryableGraphQLClient{
		Client:         client,
		MaxRetries:     maxRetries,
		InitialBackoff: initialBackoff,
	}
}

// ExecuteRESTWithRetry executes a REST API call with retry logic
func (c *RetryableRESTClient) ExecuteRESTWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return utils.NewAppError(utils.ErrorTypeGeneral, "Context canceled", ctx.Err()).WithRetry(false)
		default:
			// Continue with the retry
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Log and determine if we should retry
		retryable := true

		// Check if error is from GitHub API
		if ghErr, ok := err.(*github.ErrorResponse); ok {
			// Categorize error type
			switch ghErr.Response.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				slog.Warn("authentication error", "error", err)
				return utils.NewAppError(utils.ErrorTypeAuth, "GitHub API authentication error", err).WithRetry(false)
			case http.StatusTooManyRequests, http.StatusServiceUnavailable:
				// Get retry-after header if available
				retryAfter := ghErr.Response.Header.Get("Retry-After")
				slog.Warn("rate limited by GitHub API", "retry_after", retryAfter)
				lastErr = utils.NewAppError(utils.ErrorTypeRateLimit, "GitHub API rate limit exceeded", err)
			default:
				// General API error, may be retryable
				lastErr = utils.NewAppError(utils.ErrorTypeAPI, "Error during API call", err)
			}
		} else if appErr, ok := err.(*utils.AppError); ok {
			lastErr = appErr
			retryable = appErr.Retryable
		} else {
			// Other errors
			lastErr = utils.NewAppError(utils.ErrorTypeGeneral, "Error during API call", err)
		}

		// If error is not retryable or this was our last attempt, return
		if !retryable || attempt >= c.MaxRetries {
			if attempt >= c.MaxRetries {
				return fmt.Errorf("max retries reached: %w", lastErr)
			}
			return lastErr
		}

		// Calculate backoff for next retry
		backoff := c.InitialBackoff * (1 << attempt)
		jitter := time.Duration(float64(backoff) * (0.5 + 0.5*float64(time.Now().Nanosecond())/float64(1e9)))

		slog.Warn("retrying API call", "error", lastErr, "attempt", attempt, "backoff", backoff+jitter)

		// Wait before retrying
		select {
		case <-ctx.Done():
			return utils.NewAppError(utils.ErrorTypeGeneral, "Context canceled during backoff", ctx.Err()).WithRetry(false)
		case <-time.After(backoff + jitter):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries reached: %w", lastErr)
}

// ExecuteGraphQLWithRetry executes a GraphQL API call with retry logic
func (c *RetryableGraphQLClient) ExecuteGraphQLWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return utils.NewAppError(utils.ErrorTypeGeneral, "Context canceled", ctx.Err()).WithRetry(false)
		default:
			// Continue with the retry
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		slog.Debug("GraphQL query error", "error", err, "attempt", attempt)

		// Determine if we should retry
		retryable := true

		// GraphQL errors are more complex to parse, but we can do some basic checks
		if appErr, ok := err.(*utils.AppError); ok {
			lastErr = appErr
			retryable = appErr.Retryable
		} else {
			errMsg := err.Error()
			if strings.Contains(errMsg, "rate limit exceeded") {
				lastErr = utils.NewAppError(utils.ErrorTypeRateLimit, "GitHub GraphQL API rate limit exceeded", err)
			} else if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403") {
				return utils.NewAppError(utils.ErrorTypeAuth, "GitHub GraphQL API authentication error", err).WithRetry(false)
			} else {
				// Default to retryable API error
				lastErr = utils.NewAppError(utils.ErrorTypeAPI, "GitHub GraphQL API error", err)
			}
		}

		// If error is not retryable or this was our last attempt, return
		if !retryable || attempt >= c.MaxRetries {
			if attempt >= c.MaxRetries {
				return fmt.Errorf("max retries reached: %w", lastErr)
			}
			return lastErr
		}

		// Calculate backoff for next retry
		backoff := c.InitialBackoff * (1 << attempt)
		jitter := time.Duration(float64(backoff) * (0.5 + 0.5*float64(time.Now().Nanosecond())/float64(1e9)))

		slog.Warn("retrying GraphQL query", "error", lastErr, "attempt", attempt, "backoff", backoff+jitter)

		// Wait before retrying
		select {
		case <-ctx.Done():
			return utils.NewAppError(utils.ErrorTypeGeneral, "Context canceled during backoff", ctx.Err()).WithRetry(false)
		case <-time.After(backoff + jitter):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries reached: %w", lastErr)
}
