package reports

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportWriters(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		name       string
		filename   string
		writerType string
		header     []string
		rows       [][]string
	}{
		{
			name:       "CSV Writer",
			filename:   tempDir + "/test.csv",
			writerType: "csv",
			header:     []string{"Col1", "Col2", "Col3"},
			rows: [][]string{
				{"Row1Val1", "Row1Val2", "Row1Val3"},
				{"Row2Val1", "Row2Val2", "Row2Val3"},
			},
		},
		{
			name:       "JSON Writer",
			filename:   tempDir + "/test.json",
			writerType: "json",
			header:     []string{"Col1", "Col2", "Col3"},
			rows: [][]string{
				{"Row1Val1", "Row1Val2", "Row1Val3"},
				{"Row2Val1", "Row2Val2", "Row2Val3"},
			},
		},
		{
			name:       "Excel Writer",
			filename:   tempDir + "/test.xlsx",
			writerType: "xlsx",
			header:     []string{"Col1", "Col2", "Col3"},
			rows: [][]string{
				{"Row1Val1", "Row1Val2", "Row1Val3"},
				{"Row2Val1", "Row2Val2", "Row2Val3"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new report writer
			writer, err := NewReportWriter(tc.filename)
			require.NoError(t, err)
			require.NotNil(t, writer)

			// Write header
			err = writer.WriteHeader(tc.header)
			require.NoError(t, err)

			// Write rows
			for _, row := range tc.rows {
				err = writer.WriteRow(row)
				require.NoError(t, err)
			}

			// Close the writer
			err = writer.Close()
			require.NoError(t, err)

			// Verify file exists
			_, err = os.Stat(tc.filename)
			assert.NoError(t, err)

			// Verify file is not empty
			info, err := os.Stat(tc.filename)
			assert.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0))

			// For more thorough testing, we could read back the files and
			// verify their contents match expectations, but that would require
			// format-specific parsing logic
		})
	}
}

func TestNewReportWriter(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		name        string
		filename    string
		expectError bool
	}{
		{
			name:        "CSV Extension",
			filename:    tempDir + "/test.csv",
			expectError: false,
		},
		{
			name:        "JSON Extension",
			filename:    tempDir + "/test.json",
			expectError: false,
		},
		{
			name:        "Excel Extension",
			filename:    tempDir + "/test.xlsx",
			expectError: false,
		},
		{
			name:        "Unknown Extension",
			filename:    tempDir + "/test.unknown",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writer, err := NewReportWriter(tc.filename)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, writer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, writer)

				// Clean up if writer was created
				if writer != nil {
					err = writer.Close()
					assert.NoError(t, err)
				}
			}
		})
	}
}
