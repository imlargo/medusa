package main

import (
	"fmt"
	"os"

	"github.com/imlargo/medusa/cmd/medusa/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
