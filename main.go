// Package main is the entry point for the GitHub Enterprise Reports application.
// It initializes the application and delegates command handling to the cmd package.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kuhlman-labs/gh-enterprise-reports/cmd"
)

// main initializes the application, sets up the configuration and logging,
// and runs the command-line interface.
func main() {
	// Initialize logging
	if err := cmd.SetupLogging(); err != nil {
		slog.Error("failed to set up logging", "error", err)
		os.Exit(1)
	}
	defer cmd.CloseLogging()

	// Add context cancellation for long-running operations.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown.
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		slog.Info("received shutdown signal, canceling context")
		cancel()
	}()

	// Initialize and execute the CLI commands
	cmd.Init()
	if err := cmd.Execute(ctx); err != nil {
		slog.Error("command execution error", "error", err)
		os.Exit(1)
	}
}
