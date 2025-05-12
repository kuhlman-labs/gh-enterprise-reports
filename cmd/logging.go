package cmd

import (
	"log/slog"
	"os"

	"github.com/kuhlman-labs/gh-enterprise-reports/enterprise-reports/logging"
)

// SetupLogging initializes logging to file and terminal
func SetupLogging() error {
	var err error
	// Open log file in append mode.
	logFile, err = os.OpenFile("gh-enterprise-reports.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	// initialize slog to file+terminal at Info level
	logging.SetupLogging(logFile, slog.LevelInfo)
	return nil
}

// CloseLogging ensures the log file is closed
func CloseLogging() {
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			slog.Error("failed to close log file", "error", err)
		}
	}
}

// setLogLevel updates the current log level
func setLogLevel(level slog.Level) {
	logging.SetupLogging(logFile, level)
}
