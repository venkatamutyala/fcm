package main

import (
	"errors"
	"fmt"
	"os"

	fcmerrors "fcm.dev/fcm-cli/internal/errors"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		var he *fcmerrors.HintError
		if errors.As(err, &he) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", he.Err)
			fmt.Fprintf(os.Stderr, "%s\n", he.Hint)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}
