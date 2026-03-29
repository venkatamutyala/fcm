package main

import (
	"encoding/json"
	"fmt"
	"os"

	fcmerrors "fcm.dev/fcm-cli/internal/errors"
	"github.com/spf13/cobra"
)

var (
	flagJSON    bool
	flagVerbose bool
)

var rootCmd = &cobra.Command{
	Use:   "fcm",
	Short: "Firecracker Machine Manager",
	Long:  "FCM is a CLI for managing Firecracker microVMs on bare metal Linux.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print fcm version",
	Run: func(cmd *cobra.Command, args []string) {
		if flagJSON {
			out, _ := json.Marshal(map[string]string{"version": Version})
			fmt.Println(string(out))
		} else {
			fmt.Printf("fcm %s\n", Version)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
	rootCmd.AddCommand(versionCmd)
}

// requireRoot checks that the process is running as root.
func requireRoot() error {
	if os.Geteuid() != 0 {
		return fcmerrors.WithPermissionHint(fmt.Errorf("fcm must be run as root"))
	}
	return nil
}
