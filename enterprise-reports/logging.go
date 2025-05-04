// Package enterprisereports provides functionality for generating reports about GitHub Enterprise resources.
// It handles configuration parsing, client initialization, and report generation for organizations,
// repositories, teams, collaborators, and users across a GitHub Enterprise instance.
package enterprisereports

import (
	"context"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

// multiHandler fans out logging records to multiple slog.Handlers.
// This allows logging to both a file (in JSON format) and the terminal (in colored text format)
// with a single log call.
type multiHandler struct {
	handlers []slog.Handler // The collection of handlers to fan out to
}

// Enabled implements slog.Handler.Enabled and returns true if any of the
// underlying handlers are enabled for the given level.
func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle implements slog.Handler.Handle by forwarding the record to all underlying handlers.
// Returns the first error encountered, if any.
func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	for _, h := range m.handlers {
		if e := h.Handle(ctx, r); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// WithAttrs implements slog.Handler.WithAttrs by creating a new multiHandler
// with the specified attributes added to all underlying handlers.
func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := &multiHandler{}
	for _, h := range m.handlers {
		out.handlers = append(out.handlers, h.WithAttrs(attrs))
	}
	return out
}

// WithGroup implements slog.Handler.WithGroup by creating a new multiHandler
// with the specified group name added to all underlying handlers.
func (m *multiHandler) WithGroup(name string) slog.Handler {
	out := &multiHandler{}
	for _, h := range m.handlers {
		out.handlers = append(out.handlers, h.WithGroup(name))
	}
	return out
}

// NewMultiHandler creates a handler that writes JSON logs to the provided file
// and colored text logs to stderr, both at the specified log level.
//
// Parameters:
//   - file: Open file handle to write JSON logs to
//   - level: Minimum log level to process
//
// Returns a slog.Handler that writes to both outputs simultaneously.
func NewMultiHandler(file *os.File, level slog.Level) slog.Handler {
	fileH := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level})
	consoleH := tint.NewHandler(os.Stderr, &tint.Options{Level: level})
	return &multiHandler{handlers: []slog.Handler{consoleH, fileH}}
}

// SetupLogging configures the global slog logger to write to both file and terminal.
// It replaces the default logger with a new logger that writes formatted logs
// to both destinations at the specified log level.
//
// Parameters:
//   - file: Open file handle to write JSON logs to
//   - level: Minimum log level to process
func SetupLogging(file *os.File, level slog.Level) {
	h := NewMultiHandler(file, level)
	logger := slog.New(h)
	slog.SetDefault(logger)
}
