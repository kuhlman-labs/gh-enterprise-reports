// Package utils provides utility functions and types for the GitHub Enterprise Reports application.
package utils

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ValidateFilePath ensures the directory for the given file path exists
// and the path itself is non‐empty, non‐absolute, and contains no parent refs.
// It returns an error if the path is invalid or if the parent directory doesn't exist.
func ValidateFilePath(path string) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid file path: contains parent directory reference")
	}
	// if it's a relative path, ensure it doesn't climb above cwd
	if !filepath.IsAbs(cleanPath) {
		if rel, err := filepath.Rel(".", cleanPath); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("file path escapes working directory: %s", cleanPath)
		}
	}
	// now ensure the parent directory exists
	dir := filepath.Dir(cleanPath)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		return fmt.Errorf("error accessing directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path %s is not a directory", dir)
	}
	return nil
}

// CreateCSVFileWithHeader creates the CSV file at path, writes the header, and returns the file & writer.
// The path is first validated using ValidateFilePath.
// Returns an error if the file cannot be created or if writing the header fails.
func CreateCSVFileWithHeader(path string, header []string) (*os.File, *csv.Writer, error) {
	// validate the path first
	if err := ValidateFilePath(path); err != nil {
		return nil, nil, err
	}

	// #nosec G304  // safe: path has been validated by ValidateFilePath
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSV file %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		if err := f.Close(); err != nil {
			slog.Error("failed to close file after header write error", "error", err)
		}
		return nil, nil, fmt.Errorf("failed to write header to file %s: %w", path, err)
	}
	return f, w, nil
}
