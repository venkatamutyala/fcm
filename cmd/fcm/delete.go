package main

import (
	"fmt"

	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var deleteForce bool

var deleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a VM and all its data",
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
			if !deleteForce {
				return fmt.Errorf("vm %q is running (use --force to stop and delete)", name)
			}
			fmt.Printf("Stopping %s...\n", name)
			if err := systemd.Stop(unit); err != nil {
				return fmt.Errorf("stop vm: %w", err)
			}
		}

		// Load VM state for cleanup
		v, err := vm.LoadVM(name)
		if err == nil {
			// Clean up TAP device
			_ = network.DeleteTAP(v.TAPDevice)
		}

		// Remove systemd unit
		if err := systemd.RemoveVMUnit(name); err != nil {
			fmt.Printf("Warning: could not remove systemd unit: %v\n", err)
		}

		// Remove VM directory (state, rootfs, cidata, logs)
		if err := vm.DeleteVMState(name); err != nil {
			return fmt.Errorf("delete vm data: %w", err)
		}

		fmt.Printf("VM %s deleted\n", name)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force delete a running VM")
	rootCmd.AddCommand(deleteCmd)
}
