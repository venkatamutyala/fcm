package main

import (
	"fmt"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [name]",
	Short: "Stop a running VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}

		name := args[0]
		if !vm.Exists(name) {
			return fmt.Errorf("vm %q not found", name)
		}

		unit := systemd.VMUnitName(name)
		if !systemd.IsActive(unit) {
			fmt.Printf("VM %s is already stopped\n", name)
			return nil
		}

		if err := systemd.Stop(unit); err != nil {
			return fmt.Errorf("stop vm: %w", err)
		}

		fmt.Printf("VM %s stopped\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
