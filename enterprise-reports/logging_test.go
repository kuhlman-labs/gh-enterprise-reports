// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
package enterprisereports

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestMultiHandlerEnabled(t *testing.T) {
	// Create handlers with different log levels to test Enabled functionality
	buf1 := &bytes.Buffer{}
	handler1 := slog.NewJSONHandler(buf1, &slog.HandlerOptions{Level: slog.LevelInfo})

	buf2 := &bytes.Buffer{}
	handler2 := slog.NewJSONHandler(buf2, &slog.HandlerOptions{Level: slog.LevelWarn})

	multiH := &multiHandler{
		handlers: []slog.Handler{handler1, handler2},
	}

	// Test cases for different log levels
	testCases := []struct {
		level    slog.Level
		expected bool
	}{
		{slog.LevelDebug, false}, // Both handlers reject debug
		{slog.LevelInfo, true},   // First handler accepts info
		{slog.LevelWarn, true},   // Both handlers accept warn
		{slog.LevelError, true},  // Both handlers accept error
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.level.String(), func(t *testing.T) {
			result := multiH.Enabled(ctx, tc.level)
			if result != tc.expected {
				t.Errorf("Enabled(%v) = %v, want %v", tc.level, result, tc.expected)
			}
		})
	}
}

func TestMultiHandlerHandle(t *testing.T) {
	// Create test buffers to capture output
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	handler1 := slog.NewJSONHandler(buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler2 := slog.NewJSONHandler(buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	multiH := &multiHandler{
		handlers: []slog.Handler{handler1, handler2},
	}

	// Create a test record
	record := slog.Record{}
	record.Level = slog.LevelInfo
	record.Message = "test message"
	record.AddAttrs(slog.String("key", "value"))

	// Handle the record
	err := multiH.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Handle() error = %v", err)
	}

	// Verify both handlers received the record
	for i, buf := range []*bytes.Buffer{buf1, buf2} {
		var data map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
			t.Errorf("Handler %d: Failed to parse JSON: %v", i+1, err)
			continue
		}

		if msg, ok := data["msg"].(string); !ok || msg != "test message" {
			t.Errorf("Handler %d: Expected message 'test message', got %v", i+1, data["msg"])
		}

		if val, ok := data["key"].(string); !ok || val != "value" {
			t.Errorf("Handler %d: Expected attribute key='value', got %v", i+1, data["key"])
		}
	}
}

func TestMultiHandlerWithAttrs(t *testing.T) {
	// Create a test buffer
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})

	multiH := &multiHandler{
		handlers: []slog.Handler{handler},
	}

	// Add attributes
	attrs := []slog.Attr{slog.String("attr1", "value1"), slog.Int("attr2", 42)}
	newHandler := multiH.WithAttrs(attrs)

	// Create a record and handle it
	record := slog.Record{}
	record.Level = slog.LevelInfo
	record.Message = "test message"

	err := newHandler.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Handle() error = %v", err)
	}

	// Verify attributes were added
	var data map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
		return
	}

	if val, ok := data["attr1"].(string); !ok || val != "value1" {
		t.Errorf("Expected attr1='value1', got %v", data["attr1"])
	}

	if val, ok := data["attr2"].(float64); !ok || int(val) != 42 {
		t.Errorf("Expected attr2=42, got %v", data["attr2"])
	}
}

func TestMultiHandlerWithGroup(t *testing.T) {
	// Create a test buffer
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})

	multiH := &multiHandler{
		handlers: []slog.Handler{handler},
	}

	// Add a group
	groupName := "test_group"
	newHandler := multiH.WithGroup(groupName)

	// Add an attribute within the group and handle a record
	withAttr := newHandler.WithAttrs([]slog.Attr{slog.String("key", "value")})

	record := slog.Record{}
	record.Level = slog.LevelInfo
	record.Message = "test message"

	err := withAttr.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Handle() error = %v", err)
	}

	// Verify the group was created
	var data map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
		return
	}

	// Check if group exists and contains our attribute
	if group, ok := data[groupName].(map[string]interface{}); ok {
		if val, ok := group["key"].(string); !ok || val != "value" {
			t.Errorf("Expected %s.key='value', got %v", groupName, group["key"])
		}
	} else {
		t.Errorf("Expected group '%s' in output, not found in %v", groupName, data)
	}
}

func TestNewMultiHandler(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "log_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tmpFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

	// Redirect stderr to capture console output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	// Create the multi handler
	handler := NewMultiHandler(tmpFile, slog.LevelInfo)

	// Use the handler to log a message
	logger := slog.New(handler)
	logger.Info("test message", "key", "value")

	// Close the writer to flush all data
	if err := w.Close(); err != nil {
		t.Errorf("Failed to close pipe writer: %v", err)
	}

	// Read stderr output
	stderrOutput, _ := io.ReadAll(r)

	// Read file output
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Errorf("Failed to seek in temp file: %v", err)
	}
	fileOutput, _ := io.ReadAll(tmpFile)

	// Verify file output (JSON format)
	var fileData map[string]interface{}
	if err := json.Unmarshal(fileOutput, &fileData); err != nil {
		t.Errorf("Failed to parse file JSON output: %v", err)
	} else {
		if msg, ok := fileData["msg"].(string); !ok || msg != "test message" {
			t.Errorf("File output: Expected message 'test message', got %v", fileData["msg"])
		}
		if val, ok := fileData["key"].(string); !ok || val != "value" {
			t.Errorf("File output: Expected key='value', got %v", fileData["key"])
		}
	}

	// Verify console output (contains the message and key)
	stderrStr := string(stderrOutput)
	if !strings.Contains(stderrStr, "test message") {
		t.Errorf("Console output doesn't contain 'test message': %s", stderrStr)
	}

	// Instead of exact matching, check for the presence of both key and value in the output
	if !strings.Contains(stderrStr, "key") || !strings.Contains(stderrStr, "value") {
		t.Errorf("Console output doesn't contain both 'key' and 'value': %s", stderrStr)
	}
}

func TestSetupLogging(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "setup_log_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tmpFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

	// Save the default logger to restore it after the test
	origLogger := slog.Default()
	defer slog.SetDefault(origLogger)

	// Redirect stderr to capture console output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	// Set up logging
	SetupLogging(tmpFile, slog.LevelInfo)

	// Log a test message with the default logger
	slog.Info("setup test", "setup_key", "setup_value")

	// Close the writer to flush all data
	if err := w.Close(); err != nil {
		t.Errorf("Failed to close pipe writer: %v", err)
	}

	// Read stderr output
	stderrOutput, _ := io.ReadAll(r)

	// Read file output
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Errorf("Failed to seek in temp file: %v", err)
	}
	fileOutput, _ := io.ReadAll(tmpFile)

	// Verify file output (JSON format)
	var fileData map[string]interface{}
	if err := json.Unmarshal(fileOutput, &fileData); err != nil {
		t.Errorf("Failed to parse file JSON output: %v", err)
	} else {
		if msg, ok := fileData["msg"].(string); !ok || msg != "setup test" {
			t.Errorf("File output: Expected message 'setup test', got %v", fileData["msg"])
		}
		if val, ok := fileData["setup_key"].(string); !ok || val != "setup_value" {
			t.Errorf("File output: Expected setup_key='setup_value', got %v", fileData["setup_key"])
		}
	}

	// Verify console output
	stderrStr := string(stderrOutput)
	if !strings.Contains(stderrStr, "setup test") {
		t.Errorf("Console output doesn't contain 'setup test': %s", stderrStr)
	}

	// Instead of looking for exact string "setup_key=setup_value", check for each part separately
	if !strings.Contains(stderrStr, "setup_key") || !strings.Contains(stderrStr, "setup_value") {
		t.Errorf("Console output doesn't contain both 'setup_key' and 'setup_value': %s", stderrStr)
	}
}
