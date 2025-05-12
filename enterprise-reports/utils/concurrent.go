// Package utils provides utility functions and types for the GitHub Enterprise Reports application.
package utils

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"golang.org/x/time/rate"
)

// ProcessorFunc processes an input item to an output record.
// This is a generic function type used by the RunReport function to process items concurrently.
type ProcessorFunc[I any, O any] func(ctx context.Context, item I) (O, error)

// FormatterFunc formats an output record into a CSV row.
// This is a generic function type used by the RunReport function to format processed items into CSV rows.
type FormatterFunc[O any] func(output O) []string

// RunReport processes items concurrently using the provided processor and formatter,
// writing results to a CSV file with the given header. It respects the provided rate limiter.
//
// Parameters:
//   - ctx: Context for cancellation
//   - items: Slice of input items to process
//   - processor: Function to process each input item
//   - formatter: Function to format processed items into CSV rows
//   - limiter: Rate limiter to control API request frequency
//   - workerCount: Number of concurrent workers
//   - filename: Path to the output CSV file
//   - header: CSV header row
//
// The function handles graceful cancellation through context and continues processing
// other items when individual item processing fails.
func RunReport[I any, O any](
	ctx context.Context,
	items []I,
	processor ProcessorFunc[I, O],
	formatter FormatterFunc[O],
	limiter *rate.Limiter,
	workerCount int,
	filename string,
	header []string,
) error {
	slog.Info("starting report", slog.String("filename", filename))
	file, writer, err := CreateCSVFileWithHeader(filename, header)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			slog.Error("failed to close CSV file", "error", cerr)
		}
	}()

	in := make(chan I, len(items))
	out := make(chan O, len(items))
	var wg sync.WaitGroup
	var count int64

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					slog.Warn("context canceled, stopping processing")
					return
				case item, ok := <-in:
					if !ok {
						return
					}

					// Wait for the rate limiter
					err := limiter.Wait(ctx)
					if err != nil {
						slog.Warn("rate limiter wait failed", "error", err)
						// Decide if you want to return or continue based on the error
						// If context is canceled, this will return an error.
						if ctx.Err() != nil {
							return // Context was canceled while waiting
						}
						// Handle other potential limiter errors if necessary, or just log and continue
						continue
					}

					result, err := processor(ctx, item)
					if err != nil {
						slog.Warn("processing item failed", "error", err, "item", item)
						continue // Skip this item on processor error
					}
					atomic.AddInt64(&count, 1)
					// Use a select to prevent blocking indefinitely if the context is canceled
					// while waiting to send to the out channel.
					select {
					case out <- result:
					case <-ctx.Done():
						slog.Warn("context canceled, discarding result")
						return
					}
				}
			}
		}()
	}

	go func() {
	InputLoop: // Labeled break for the outer loop
		for _, item := range items {
			// Use a select to prevent blocking indefinitely if the context is canceled
			// while waiting to send to the in channel.
			select {
			case in <- item:
			case <-ctx.Done():
				slog.Warn("context canceled during input sending, stopping early")
				break InputLoop // Use labeled break to exit the for loop
			}
		}
		close(in) // Close in channel regardless of context cancellation
		wg.Wait()
		slog.Info("processing complete", slog.Int64("total", count))
		close(out)
	}()

	for result := range out {
		row := formatter(result)
		if err := writer.Write(row); err != nil {
			// It might be better to log the error and continue,
			// rather than failing the entire report for one write error.
			slog.Error("failed to write row to CSV", "error", err)
			continue // Continue processing other results
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		// Log the flush error, but the report might still be partially useful.
		slog.Error("failed to flush CSV writer", "error", err)
	}
	slog.Info("report complete", slog.String("filename", filename))
	return writer.Error() // Return the flush error if any occurred
}
