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

// Helpers return zero/N/A when passed nil.
func TestGetRemainingNil(t *testing.T) {
	if got := getRemaining(nil); got != 0 {
		t.Errorf("getRemaining(nil) = %d; want %d", got, 0)
	}
}

func TestGetLimitNil(t *testing.T) {
	if got := getLimit(nil); got != 0 {
		t.Errorf("getLimit(nil) = %d; want %d", got, 0)
	}
}

func TestGetResetTimeNil(t *testing.T) {
	if got := getResetTime(nil); got != "N/A" {
		t.Errorf("getResetTime(nil) = %q; want %q", got, "N/A")
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
	fakeService := &fakeRateLimitService{
		RateLimitService: &github.RateLimitService{}, // embed must be non-nil
		rateLimits:       rl,
	}
	ctx := context.Background()

	// pass fakeService (implements Get) directly
	got, err := checkRateLimit(ctx, fakeService)
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
	ctx := context.Background()

	// first, persistent error
	_, err := checkRateLimit(ctx, fakeService)
	if err == nil {
		t.Fatalf("checkRateLimit() expected error due to persistent error")
	}

	// now transient: clear error after first call
	fakeService.callCount = 0
	fakeService.err = errors.New("temporary error")
	go func() {
		time.Sleep(1 * time.Second)
		fakeService.err = nil
	}()
	got, err := checkRateLimit(ctx, fakeService)
	if err != nil {
		t.Fatalf("checkRateLimit() unexpected error on retry: %v", err)
	}
	if got != rl {
		t.Errorf("checkRateLimit() returned unexpected result on retry")
	}
}

// checkRateLimit returns context error if ctx is canceled mid‑retry.
func TestCheckRateLimitContextCanceled(t *testing.T) {
	// service always errors
	fakeSvc := &fakeRateLimitService{
		RateLimitService: &github.RateLimitService{},
		err:              errors.New("oops"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := checkRateLimit(ctx, fakeSvc)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("checkRateLimit didn’t return context error; got %v", err)
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

// waitForLimitReset returns immediately if resetTime is in the past.
func TestWaitForLimitResetNoWait(t *testing.T) {
	start := time.Now()
	waitForLimitReset(context.Background(), "test", 1, 10, time.Now().Add(-1*time.Second).UTC())
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waitForLimitReset should return immediately; took %v", elapsed)
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
	fakeService := &fakeRateLimitService{
		RateLimitService: &github.RateLimitService{},
		rateLimits:       rl,
	}
	graphQLClient := &githubv4.Client{} // not used here
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1500 * time.Millisecond)
		cancel()
	}()
	// now pass fakeService instead of *github.Client
	MonitorRateLimits(ctx, fakeService, graphQLClient, 1*time.Second)
	// test passes if no panic
}

// handleRESTRateLimit does nothing when remaining above threshold.
func TestHandleRESTRateLimitNoWait(t *testing.T) {
	start := time.Now()
	r := github.Rate{Remaining: RESTRateLimitThreshold + 1, Limit: 100, Reset: github.Timestamp{Time: time.Now().Add(1 * time.Second)}}
	handleRESTRateLimit(context.Background(), r)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("handleRESTRateLimit above threshold should return immediately; took %v", elapsed)
	}
}
