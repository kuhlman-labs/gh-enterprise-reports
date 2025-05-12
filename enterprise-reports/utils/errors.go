// Package utils provides utility functions and types for the GitHub Enterprise Reports application.
package utils

import (
	"fmt"
	"time"
)

// ErrorType represents different categories of errors that can occur in the application.
type ErrorType int

const (
	// ErrorTypeGeneral represents a general error.
	ErrorTypeGeneral ErrorType = iota
	// ErrorTypeAPI represents an API-related error.
	ErrorTypeAPI
	// ErrorTypeRateLimit represents a rate limit error.
	ErrorTypeRateLimit
	// ErrorTypeAuth represents an authentication error.
	ErrorTypeAuth
	// ErrorTypeConfig represents a configuration error.
	ErrorTypeConfig
	// ErrorTypeIO represents an I/O error.
	ErrorTypeIO
)

// AppError represents an application-specific error with type classification.
type AppError struct {
	Type      ErrorType
	Message   string
	Cause     error
	Retryable bool
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the wrapped error.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NewAppError creates a new application error with the given type, message, and cause.
func NewAppError(errType ErrorType, message string, cause error) *AppError {
	// Determine if error is retryable based on type
	retryable := errType == ErrorTypeRateLimit || errType == ErrorTypeAPI

	return &AppError{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Retryable: retryable,
	}
}

// WithRetry marks an error as retryable or not.
func (e *AppError) WithRetry(retryable bool) *AppError {
	e.Retryable = retryable
	return e
}

// IsRetryable returns whether the error is retryable.
func IsRetryable(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Retryable
	}
	return false
}

// RetryWithBackoff retries a function with exponential backoff.
// It returns the result of the function if it succeeds, or the last error if it fails.
// maxRetries: maximum number of retries
// initialBackoff: initial backoff duration in milliseconds
// fn: function to retry
func RetryWithBackoff(maxRetries int, initialBackoff time.Duration, fn func() error) error {
	var err error
	backoff := initialBackoff

	// First attempt
	if err = fn(); err == nil {
		return nil
	}

	// If error is not retryable, return immediately
	if !IsRetryable(err) {
		return err
	}

	// Retry up to maxRetries times
	for i := 0; i < maxRetries; i++ {
		// Wait before retrying
		time.Sleep(backoff)

		// Exponential backoff with jitter to avoid thundering herd
		jitter := time.Duration(float64(backoff) * (0.5 + 0.5*float64(time.Now().Nanosecond())/float64(1e9)))
		backoff = backoff*2 + jitter

		// Try again
		if err = fn(); err == nil {
			return nil
		}

		// If error is not retryable, return immediately
		if !IsRetryable(err) {
			return err
		}
	}

	return fmt.Errorf("max retries reached: %w", err)
}
