// cmd/medusa/commands/root.go
package commands

import (
	"github.com/spf13/cobra"
)

const version = "0.2.0"

var rootCmd = &cobra.Command{
	Use:   "medusa",
	Short: "🐍 Medusa - A batteries-included Go framework",
	Long: `Medusa is a production-ready framework for building modern, scalable backends in Go. 

Complete documentation is available at https://github.com/imlargo/medusa`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add subcommands
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
}
