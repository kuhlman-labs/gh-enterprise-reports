package reports

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

// getHighestPermission returns the highest permission level from the provided permissions map.
func getHighestPermission(permissions map[string]bool) string {
	switch {
	case permissions["admin"]:
		return "admin"
	case permissions["maintain"]:
		return "maintain"
	case permissions["push"]:
		return "push"
	case permissions["triage"]:
		return "triage"
	case permissions["pull"]:
		return "pull"
	default:
		return "none"
	}
}

// validateFilePath ensures the directory for the given file path exists
// and the path itself is non‐empty.
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	dir := filepath.Dir(path)
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

// createCSVFileWithHeader creates the CSV file at path, writes the header, and returns the file & writer.
func createCSVFileWithHeader(path string, header []string) (*os.File, *csv.Writer, error) {
	// validate the path first
	if err := validateFilePath(path); err != nil {
		return nil, nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSV file %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("failed to write header to file %s: %w", path, err)
	}
	return f, w, nil
}
