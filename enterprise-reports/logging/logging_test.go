// Package logging provides logging functionality for the GitHub Enterprise Reports tool.
package logging

import (
	"bytes"
	"log/slog"
	"os"
	"testing"
)

func TestSetupLogging(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "test-log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Set up logging with the temporary file
	SetupLogging(tmpFile, slog.LevelDebug)

	// Log a test message
	testMessage := "test log message"
	slog.Info(testMessage)

	// Check if the file contains the log message
	fileContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if !bytes.Contains(fileContent, []byte(testMessage)) {
		t.Errorf("Log file does not contain the expected message. Got: %s", string(fileContent))
	}
}

func TestMultiHandler(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "test-multihandler")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Create a multi handler
	handler := NewMultiHandler(tmpFile, slog.LevelDebug)

	// Create a logger with the handler
	logger := slog.New(handler)

	// Log a test message
	testMessage := "test multihandler"
	logger.Info(testMessage)

	// Check if the file contains the log message
	fileContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if !bytes.Contains(fileContent, []byte(testMessage)) {
		t.Errorf("Log file does not contain the expected message. Got: %s", string(fileContent))
	}
}
