package main

import (
	"fmt"
	"os"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
