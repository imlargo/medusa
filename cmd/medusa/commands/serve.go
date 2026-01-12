package commands

import (
	"github.com/imlargo/medusa/cmd/medusa/utils"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start development server with hot reload",
	Run: func(cmd *cobra.Command, args []string) {
		utils.PrintWarning("Serve functionality coming soon...")
		utils.PrintInfo("For now, use: make dev or air")
	},
}
