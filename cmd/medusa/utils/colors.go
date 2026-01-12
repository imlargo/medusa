package utils

import (
	"fmt"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

func PrintBanner() {
	fmt.Println(ColorCyan + `
🐍 Medusa Framework` + ColorReset)
}

func PrintSuccess(msg string) {
	fmt.Print(ColorGreen + msg + ColorReset)
}

func PrintError(msg string) {
	fmt.Print(ColorRed + "✗ " + msg + ColorReset + "\n")
}

func PrintWarning(msg string) {
	fmt.Print(ColorYellow + "⚠ " + msg + ColorReset + "\n")
}

func PrintInfo(msg string) {
	fmt.Print(ColorBlue + msg + ColorReset + "\n")
}

func PrintStep(msg string) {
	fmt.Print(ColorCyan + "→ " + msg + "..." + ColorReset + "\n")
}

func PrintCheckmark(msg string) {
	fmt.Print(ColorGreen + "✓ " + msg + ColorReset + "\n")
}

func PrintBold(msg string) {
	fmt.Print(ColorBold + msg + ColorReset + "\n")
}
