package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var uninstallConfirm bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove FCM, Firecracker, and all state",
	Long:  "Runs cleanup --confirm, then removes the fcm and firecracker binaries and /var/lib/fcm.",
	RunE:  runUninstall,
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallConfirm, "confirm", false, "Confirm uninstall")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	if !uninstallConfirm {
		return fmt.Errorf("this will remove FCM entirely — pass --confirm to proceed")
	}

	// Run cleanup first
	fmt.Println("Running fcm cleanup...")
	cleanupConfirm = true
	if err := runCleanup(cmd, args); err != nil {
		fmt.Printf("Warning: cleanup returned error: %v\n", err)
	}

	// Remove binaries and state
	paths := []string{
		"/usr/local/bin/fcm",
		"/usr/local/bin/firecracker",
		"/var/lib/fcm",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			fmt.Printf("Already removed: %s\n", p)
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			fmt.Printf("Warning: could not remove %s: %v\n", p, err)
		} else {
			fmt.Printf("Removed: %s\n", p)
		}
	}

	fmt.Println("\nFCM has been uninstalled.")
	return nil
}
