package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new configuration file",
	Run: func(cmd *cobra.Command, args []string) {
		outputPath, _ := cmd.Flags().GetString("output")
		configFile := outputPath
		if configFile == "" {
			configFile = "config.yml"
		}

		if _, err := os.Stat(configFile); err == nil {
			slog.Error("configuration file already exists", "file", configFile)
			os.Exit(1)
		}

		templateData, err := os.ReadFile("config-template.yml")
		if err != nil {
			slog.Error("failed to read template file", "error", err)
			os.Exit(1)
		}

		if err := os.WriteFile(configFile, templateData, 0600); err != nil {
			slog.Error("failed to write configuration file", "error", err)
			os.Exit(1)
		}

		slog.Info("created new configuration file", "file", configFile)
		slog.Info("please edit the file to add your GitHub Enterprise details")
	},
}

func init() {
	// Add output flag to init command
	initCmd.Flags().StringP("output", "o", "", "Output path for the generated configuration file")
}
