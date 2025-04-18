package enterprisereports

import (
	"context"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

// multiHandler fans out records to multiple slog.Handlers.
type multiHandler struct{ handlers []slog.Handler }

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	for _, h := range m.handlers {
		if e := h.Handle(ctx, r); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := &multiHandler{}
	for _, h := range m.handlers {
		out.handlers = append(out.handlers, h.WithAttrs(attrs))
	}
	return out
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	out := &multiHandler{}
	for _, h := range m.handlers {
		out.handlers = append(out.handlers, h.WithGroup(name))
	}
	return out
}

// NewMultiHandler creates a handler that writes JSON to file and text to stderr.
func NewMultiHandler(file *os.File, level slog.Level) slog.Handler {
	fileH := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level})
	consoleH := tint.NewHandler(os.Stderr, &tint.Options{Level: level})
	return &multiHandler{handlers: []slog.Handler{consoleH, fileH}}
}

// SetupLogging configures the global slog logger to write to both file and terminal.
func SetupLogging(file *os.File, level slog.Level) {
	h := NewMultiHandler(file, level)
	logger := slog.New(h)
	slog.SetDefault(logger)
}
