// Package reports implements various report generation functionalities for GitHub Enterprise.
package reports

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/utils"
	"github.com/xuri/excelize/v2"
)

// ReportFormat represents the format of a report.
type ReportFormat string

const (
	// FormatCSV is the CSV format.
	FormatCSV ReportFormat = "csv"
	// FormatJSON is the JSON format.
	FormatJSON ReportFormat = "json"
	// FormatExcel is the Excel format.
	FormatExcel ReportFormat = "xlsx"
)

// ReportWriter is an interface for writing reports in different formats.
type ReportWriter interface {
	// WriteHeader writes the header of the report.
	WriteHeader(header []string) error
	// WriteRow writes a row of data to the report.
	WriteRow(row []string) error
	// Close finalizes the report and closes any open resources.
	Close() error
}

// CSVReportWriter implements ReportWriter for CSV format.
type CSVReportWriter struct {
	file   *os.File
	writer *csv.Writer
}

// NewCSVReportWriter creates a new CSV report writer.
func NewCSVReportWriter(path string) (*CSVReportWriter, error) {
	if err := utils.ValidateFilePath(path); err != nil {
		return nil, err
	}

	// #nosec G304  // safe: path has been validated by validateFilePath
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSV file %s: %w", path, err)
	}

	return &CSVReportWriter{
		file:   f,
		writer: csv.NewWriter(f),
	}, nil
}

// WriteHeader implements ReportWriter.WriteHeader.
func (w *CSVReportWriter) WriteHeader(header []string) error {
	if err := w.writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	return nil
}

// WriteRow implements ReportWriter.WriteRow.
func (w *CSVReportWriter) WriteRow(row []string) error {
	if err := w.writer.Write(row); err != nil {
		return fmt.Errorf("failed to write row: %w", err)
	}
	return nil
}

// Close implements ReportWriter.Close.
func (w *CSVReportWriter) Close() error {
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		return fmt.Errorf("error flushing CSV writer: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("error closing CSV file: %w", err)
	}
	return nil
}

// JSONReportWriter implements ReportWriter for JSON format.
type JSONReportWriter struct {
	file    *os.File
	header  []string
	records []map[string]string
}

// NewJSONReportWriter creates a new JSON report writer.
func NewJSONReportWriter(path string) (*JSONReportWriter, error) {
	if err := utils.ValidateFilePath(path); err != nil {
		return nil, err
	}

	// #nosec G304  // safe: path has been validated by validateFilePath
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON file %s: %w", path, err)
	}

	return &JSONReportWriter{
		file:    f,
		records: make([]map[string]string, 0),
	}, nil
}

// WriteHeader implements ReportWriter.WriteHeader.
func (w *JSONReportWriter) WriteHeader(header []string) error {
	w.header = header
	return nil
}

// WriteRow implements ReportWriter.WriteRow.
func (w *JSONReportWriter) WriteRow(row []string) error {
	if len(row) != len(w.header) {
		return fmt.Errorf("row length (%d) does not match header length (%d)", len(row), len(w.header))
	}

	record := make(map[string]string)
	for i, value := range row {
		record[w.header[i]] = value
	}
	w.records = append(w.records, record)
	return nil
}

// Close implements ReportWriter.Close.
func (w *JSONReportWriter) Close() error {
	encoder := json.NewEncoder(w.file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(w.records); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("error closing JSON file: %w", err)
	}
	return nil
}

// ExcelReportWriter implements ReportWriter for Excel format.
type ExcelReportWriter struct {
	path      string
	file      *excelize.File
	sheetName string
	rowIndex  int
}

// NewExcelReportWriter creates a new Excel report writer.
func NewExcelReportWriter(path string) (*ExcelReportWriter, error) {
	if err := utils.ValidateFilePath(path); err != nil {
		return nil, err
	}

	f := excelize.NewFile()

	// Use filename (without extension) as sheet name
	baseName := filepath.Base(path)
	ext := filepath.Ext(baseName)
	sheetName := strings.TrimSuffix(baseName, ext)
	if sheetName == "" {
		sheetName = "Report"
	}

	// Rename the default sheet
	defaultSheet := f.GetSheetName(0)
	err := f.SetSheetName(defaultSheet, sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to set sheet name: %w", err)
	}

	return &ExcelReportWriter{
		path:      path,
		file:      f,
		sheetName: sheetName,
		rowIndex:  1, // Excel is 1-indexed
	}, nil
}

// WriteHeader implements ReportWriter.WriteHeader.
func (w *ExcelReportWriter) WriteHeader(header []string) error {
	// Set header style with bold font and light gray background
	headerStyle, err := w.file.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#E0E0E0"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "bottom", Color: "#000000", Style: 1},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}

	// Write header cells
	for colIndex, cellValue := range header {
		cell, err := excelize.CoordinatesToCellName(colIndex+1, w.rowIndex)
		if err != nil {
			return fmt.Errorf("failed to convert coordinates to cell name: %w", err)
		}
		err = w.file.SetCellValue(w.sheetName, cell, cellValue)
		if err != nil {
			return fmt.Errorf("failed to set header cell value: %w", err)
		}

		// Apply style to header cell
		err = w.file.SetCellStyle(w.sheetName, cell, cell, headerStyle)
		if err != nil {
			slog.Warn("failed to set header cell style", "error", err)
		}
	}
	w.rowIndex++
	return nil
}

// WriteRow implements ReportWriter.WriteRow.
func (w *ExcelReportWriter) WriteRow(row []string) error {
	for colIndex, cellValue := range row {
		cell, err := excelize.CoordinatesToCellName(colIndex+1, w.rowIndex)
		if err != nil {
			return fmt.Errorf("failed to convert coordinates to cell name: %w", err)
		}
		err = w.file.SetCellValue(w.sheetName, cell, cellValue)
		if err != nil {
			return fmt.Errorf("failed to set cell value: %w", err)
		}
	}
	w.rowIndex++
	return nil
}

// Close implements ReportWriter.Close.
func (w *ExcelReportWriter) Close() error {
	// Auto-fit columns
	for i := 1; i <= 20; i++ { // Assume max 20 columns
		colName, err := excelize.ColumnNumberToName(i)
		if err != nil {
			continue
		}
		w.file.SetColWidth(w.sheetName, colName, colName, 20) // Set reasonable default width
	}

	if err := w.file.SaveAs(w.path); err != nil {
		return fmt.Errorf("failed to save Excel file: %w", err)
	}
	return nil
}

// NewReportWriter creates a new report writer based on the file extension.
func NewReportWriter(path string) (ReportWriter, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv":
		return NewCSVReportWriter(path)
	case ".json":
		return NewJSONReportWriter(path)
	case ".xlsx":
		return NewExcelReportWriter(path)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}
