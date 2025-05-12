package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
)

// MockError implements error for testing
type MockError struct {
	message string
}

func (e MockError) Error() string {
	return e.message
}

func TestRetryableRESTClient(t *testing.T) {
	// Create a client with a small initial backoff for faster tests
	client := NewRetryableRESTClient(&github.Client{}, 3, 10*time.Millisecond)

	t.Run("SuccessfulExecution", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteRESTWithRetry(context.Background(), func() error {
			attempts++
			return nil // Success on first attempt
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts, "Should execute exactly once for successful calls")
	})

	t.Run("EventualSuccess", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteRESTWithRetry(context.Background(), func() error {
			attempts++
			if attempts < 3 {
				return utils.NewAppError(utils.ErrorTypeAPI, "temporary error", nil)
			}
			return nil // Success on third attempt
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts, "Should retry until success")
	})

	t.Run("NonRetryableError", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteRESTWithRetry(context.Background(), func() error {
			attempts++
			return utils.NewAppError(utils.ErrorTypeConfig, "non-retryable error", nil).WithRetry(false)
		})

		assert.Error(t, err)
		assert.Equal(t, 1, attempts, "Should not retry on non-retryable errors")
	})

	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteRESTWithRetry(context.Background(), func() error {
			attempts++
			return utils.NewAppError(utils.ErrorTypeRateLimit, "rate limit exceeded", nil)
		})

		assert.Error(t, err)
		assert.Equal(t, 4, attempts, "Should attempt exactly maxRetries+1 times")
		assert.Contains(t, err.Error(), "max retries reached")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		attempts := 0
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after first attempt
		go func() {
			time.Sleep(15 * time.Millisecond) // Wait a bit
			cancel()
		}()

		err := client.ExecuteRESTWithRetry(ctx, func() error {
			attempts++
			time.Sleep(20 * time.Millisecond) // Longer than the cancel delay
			return utils.NewAppError(utils.ErrorTypeAPI, "temporary error", nil)
		})

		assert.Error(t, err)
		assert.LessOrEqual(t, attempts, 2, "Should not continue retrying after context cancellation")
	})
}

func TestRetryableGraphQLClient(t *testing.T) {
	// Create a client with a small initial backoff for faster tests
	client := NewRetryableGraphQLClient(&githubv4.Client{}, 3, 10*time.Millisecond)

	t.Run("SuccessfulExecution", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteGraphQLWithRetry(context.Background(), func() error {
			attempts++
			return nil // Success on first attempt
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts, "Should execute exactly once for successful calls")
	})

	t.Run("EventualSuccess", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteGraphQLWithRetry(context.Background(), func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary API error")
			}
			return nil // Success on third attempt
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts, "Should retry until success")
	})

	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		attempts := 0
		err := client.ExecuteGraphQLWithRetry(context.Background(), func() error {
			attempts++
			return errors.New("rate limit exceeded")
		})

		assert.Error(t, err)
		assert.Equal(t, 4, attempts, "Should attempt exactly maxRetries+1 times")
		assert.Contains(t, err.Error(), "max retries reached")
	})
}
