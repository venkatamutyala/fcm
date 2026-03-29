package main

import (
	"fmt"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [name]",
	Short: "Start a stopped VM",
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
		if systemd.IsActive(unit) {
			fmt.Printf("VM %s is already running\n", name)
			return nil
		}

		if err := systemd.Start(unit); err != nil {
			return fmt.Errorf("start vm: %w", err)
		}

		fmt.Printf("VM %s started\n", name)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart [name]",
	Short: "Restart a VM",
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
		if systemd.IsActive(unit) {
			if err := systemd.Stop(unit); err != nil {
				return fmt.Errorf("stop vm: %w", err)
			}
		}

		if err := systemd.Start(unit); err != nil {
			return fmt.Errorf("start vm: %w", err)
		}

		fmt.Printf("VM %s restarted\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(restartCmd)
}
