// Package reports implements various report generation functionalities for GitHub Enterprise.
package reports

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"golang.org/x/time/rate"
)

// RunReportWithWriter runs a report and outputs the results using the provided writer.
// It uses the ReportWriter interface instead of directly writing to a CSV file,
// which allows for multiple output formats.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - items: Slice of items to process
//   - processor: Function to process each item
//   - formatter: Function to format a processed item into a row
//   - limiter: Rate limiter to avoid overwhelming APIs
//   - workers: Number of concurrent workers
//   - reportWriter: The report writer to use for output
//
// This function handles concurrency, rate limiting, and error handling during report generation.
func RunReportWithWriter[T any, R any](
	ctx context.Context,
	items []T,
	processor func(context.Context, T) (R, error),
	formatter func(R) []string,
	limiter *rate.Limiter,
	workers int,
	reportWriter ReportWriter,
) error {
	if len(items) == 0 {
		slog.Info("no items to process")
		return nil
	}

	// Set up concurrency control
	var wg sync.WaitGroup
	itemChan := make(chan T)
	resultChan := make(chan []string)
	errorsChan := make(chan error)
	doneChan := make(chan struct{})
	var processedCount atomic.Int32
	var errorCount atomic.Int32

	// Launch workers to process items
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			for item := range itemChan {
				// Rate limiting respecting context cancellation
				if err := limiter.Wait(ctx); err != nil {
					if ctx.Err() != nil {
						// Context cancelled, stop processing
						return
					}
					// Other rate limiting error
					errorsChan <- utils.NewAppError(utils.ErrorTypeAPI,
						"rate limit error", err)
					continue
				}

				// Process the item
				result, err := processor(ctx, item)
				if err != nil {
					errorCount.Add(1)
					errorsChan <- err
					continue
				}

				// Format the result into a CSV row and send it to the result channel
				row := formatter(result)
				processedCount.Add(1)
				select {
				case resultChan <- row:
					// Row sent successfully
				case <-ctx.Done():
					// Context cancelled, stop processing
					return
				}
			}
		}(i)
	}

	// Launch goroutine to send items to workers
	go func() {
		defer close(itemChan)
		for _, item := range items {
			select {
			case itemChan <- item:
				// Item sent successfully
			case <-ctx.Done():
				// Context cancelled, stop sending items
				return
			}
		}
	}()

	// Launch goroutine to collect results and write to report
	go func() {
		defer close(doneChan)

		for row := range resultChan {
			if err := reportWriter.WriteRow(row); err != nil {
				errorsChan <- utils.NewAppError(utils.ErrorTypeIO,
					"failed to write row", err).WithRetry(false)
			}
		}
	}()

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect any errors and wait for completion
	var lastErr error
	errorsCollected := 0

	// Error collection loop
	for {
		select {
		case err := <-errorsChan:
			errorsCollected++
			lastErr = err
			// Log the error but continue processing other items
			slog.Error("error during report generation", "error", err, "total_errors", errorsCollected)

		case <-doneChan:
			// All results processed
			slog.Info("report generation completed",
				"total", len(items),
				"processed", processedCount.Load(),
				"errors", errorCount.Load())

			if lastErr != nil {
				return fmt.Errorf("completed with %d errors, last error: %w",
					errorsCollected, lastErr)
			}
			return nil

		case <-ctx.Done():
			// Context cancelled
			return ctx.Err()
		}
	}
}
