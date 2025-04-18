package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/shurcooL/githubv4"
)

// Updated fakeRateLimitService embeds a non-nil *github.RateLimitService.
type fakeRateLimitService struct {
	*github.RateLimitService // Embed the original RateLimitService
	callCount                int
	rateLimits               *github.RateLimits
	err                      error
}

func (f *fakeRateLimitService) Get(ctx context.Context) (*github.RateLimits, *github.Response, error) {
	f.callCount++
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.rateLimits, nil, nil
}

// Test helper functions.
func TestGetRemaining(t *testing.T) {
	rate := &github.Rate{Remaining: 5}
	if got := getRemaining(rate); got != 5 {
		t.Errorf("getRemaining() = %d; want %d", got, 5)
	}
}

func TestGetLimit(t *testing.T) {
	rate := &github.Rate{Limit: 10}
	if got := getLimit(rate); got != 10 {
		t.Errorf("getLimit() = %d; want %d", got, 10)
	}
}

func TestGetResetTime(t *testing.T) {
	resetTime := time.Now().UTC()
	rate := &github.Rate{Reset: github.Timestamp{Time: resetTime}}
	got := getResetTime(rate)
	want := resetTime.Format(time.RFC3339)
	if got != want {
		t.Errorf("getResetTime() = %s; want %s", got, want)
	}
}

// Test checkRateLimit successful retrieval.
func TestCheckRateLimitSuccess(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Minute).UTC()
	rl := &github.RateLimits{
		Core:     &github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: resetTime}},
		GraphQL:  &github.Rate{Limit: 5000, Remaining: 1500, Reset: github.Timestamp{Time: resetTime}},
		AuditLog: &github.Rate{Limit: 1500, Remaining: 500, Reset: github.Timestamp{Time: resetTime}},
	}
	// Initialize fakeService with an embedded, non-nil RateLimitService.
	fakeService := &fakeRateLimitService{}
	client := &github.Client{RateLimit: fakeService.RateLimitService}
	ctx := context.Background()

	got, err := checkRateLimit(ctx, client)
	if err != nil {
		t.Fatalf("checkRateLimit() unexpected error: %v", err)
	}
	if got != rl {
		t.Errorf("checkRateLimit() returned unexpected result")
	}
}

// Test checkRateLimit retries on error then succeeds.
func TestCheckRateLimitRetries(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Minute).UTC()
	rl := &github.RateLimits{
		Core:     &github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: resetTime}},
		GraphQL:  &github.Rate{Limit: 5000, Remaining: 1500, Reset: github.Timestamp{Time: resetTime}},
		AuditLog: &github.Rate{Limit: 1500, Remaining: 500, Reset: github.Timestamp{Time: resetTime}},
	}
	fakeService := &fakeRateLimitService{
		RateLimitService: &github.RateLimitService{},
		rateLimits:       rl,
		err:              errors.New("temporary error"),
	}
	client := &github.Client{RateLimit: fakeService.RateLimitService}
	ctx := context.Background()

	// Expect failure after retries.
	_, err := checkRateLimit(ctx, client)
	if err == nil {
		t.Fatalf("checkRateLimit() expected error due to persistent error")
	}

	// Now simulate transient error by clearing error after first call.
	fakeService.callCount = 0
	fakeService.err = errors.New("temporary error")
	go func() {
		time.Sleep(1 * time.Second)
		fakeService.err = nil
	}()
	got, err := checkRateLimit(ctx, client)
	if err != nil {
		t.Fatalf("checkRateLimit() unexpected error on retry: %v", err)
	}
	if got != rl {
		t.Errorf("checkRateLimit() returned unexpected result on retry")
	}
}

// Test that waitForLimitReset returns quickly when context is cancelled.
func TestWaitForLimitResetCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	resetTime := time.Now().Add(10 * time.Second).UTC()
	start := time.Now()
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	waitForLimitReset(ctx, "TEST", 1, 10, resetTime)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("waitForLimitReset() did not cancel promptly; elapsed %v", elapsed)
	}
}

// Dummy test for MonitorRateLimits; run for a single tick.
func TestMonitorRateLimits(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Minute).UTC()
	rl := &github.RateLimits{
		Core:     &github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: resetTime}},
		GraphQL:  &github.Rate{Limit: 5000, Remaining: 1500, Reset: github.Timestamp{Time: resetTime}},
		AuditLog: &github.Rate{Limit: 5000, Remaining: 500, Reset: github.Timestamp{Time: resetTime}},
	}
	fakeService := &fakeRateLimitService{rateLimits: rl}
	restClient := &github.Client{RateLimit: fakeService.RateLimitService}
	graphQLClient := &githubv4.Client{} // Not used in this test.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1500 * time.Millisecond)
		cancel()
	}()
	MonitorRateLimits(ctx, restClient, graphQLClient, 1*time.Second)
	// Test passes if no panic occurs.
}
