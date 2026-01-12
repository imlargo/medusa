package commands

import (
	"fmt"
	"runtime"

	"github.com/imlargo/medusa/cmd/medusa/utils"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run:   runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	utils.PrintBanner()
	fmt.Println()
	utils.PrintInfo(fmt.Sprintf("Version:      %s", version))
	utils.PrintInfo(fmt.Sprintf("Go Version:   %s", runtime.Version()))
	utils.PrintInfo(fmt.Sprintf("OS/Arch:     %s/%s", runtime.GOOS, runtime.GOARCH))
	fmt.Println()
}
