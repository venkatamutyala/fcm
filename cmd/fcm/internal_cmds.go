package main

import (
	"fmt"
	"os"

	"os/exec"

	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

// These commands are called by systemd, not by users directly.

var setupBridgeCmd = &cobra.Command{
	Use:    "_setup-bridge",
	Short:  "Set up the FCM network bridge (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return network.SetupBridge(cfg)
	},
}

var teardownBridgeCmd = &cobra.Command{
	Use:    "_teardown-bridge",
	Short:  "Tear down the FCM network bridge (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return network.TeardownBridge(cfg)
	},
}

var setupVMCmd = &cobra.Command{
	Use:    "_setup-vm [name]",
	Short:  "Pre-start setup for a VM (internal)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}

		name := args[0]
		v, err := vm.LoadVM(name)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Verify required files exist
		for _, path := range []string{v.RootfsPath, v.Kernel} {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("required file missing: %s", path)
			}
		}

		// Run e2fsck on rootfs to fix any journal issues from unclean shutdown
		// This prevents systemd from seeing stale state on the next boot
		exec.Command("e2fsck", "-fy", v.RootfsPath).Run()

		// Create TAP device and attach to bridge
		if err := network.CreateTAP(v.TAPDevice, cfg.BridgeName); err != nil {
			return fmt.Errorf("create tap: %w", err)
		}

		// Clean up stale socket
		os.Remove(v.SocketPath)

		return nil
	},
}

var cleanupVMCmd = &cobra.Command{
	Use:    "_cleanup-vm [name]",
	Short:  "Post-stop cleanup for a VM (internal)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		v, err := vm.LoadVM(name)
		if err != nil {
			// VM state may be gone, still try cleanup
			fmt.Fprintf(os.Stderr, "warning: could not load vm state: %v\n", err)
			return nil // always exit 0
		}

		// Remove TAP device (ignore errors)
		_ = network.DeleteTAP(v.TAPDevice)

		// Remove stale socket
		os.Remove(v.SocketPath)

		return nil // always exit 0
	},
}

var dhcpCmd = &cobra.Command{
	Use:    "_dhcp",
	Short:  "Run the embedded DHCP server (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := network.StartDHCP(cfg); err != nil {
			return err
		}
		// Block forever — systemd manages the lifecycle
		select {}
	},
}

func init() {
	rootCmd.AddCommand(setupBridgeCmd)
	rootCmd.AddCommand(teardownBridgeCmd)
	rootCmd.AddCommand(setupVMCmd)
	rootCmd.AddCommand(cleanupVMCmd)
	rootCmd.AddCommand(dhcpCmd)
}
