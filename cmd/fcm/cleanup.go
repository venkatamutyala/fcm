package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var cleanupConfirm bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove all VMs, services, and FCM state",
	Long:  "Stops and deletes all VMs, removes systemd services, iptables rules, the bridge, and /var/lib/fcm.",
	RunE:  runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupConfirm, "confirm", false, "Confirm destructive cleanup")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	if !cleanupConfirm {
		return fmt.Errorf("this will destroy ALL VMs and FCM state — pass --confirm to proceed")
	}

	cfg, err := config.Load()
	if err != nil {
		// Use defaults if config is unreadable
		cfg = config.DefaultConfig()
	}

	// Step 1: Stop and delete all VMs
	fmt.Println("[1/6] Stopping and deleting all VMs...")
	names, _ := vm.ListVMs()
	for _, name := range names {
		unit := systemd.VMUnitName(name)
		if systemd.IsActive(unit) {
			fmt.Printf("  Stopping %s...\n", name)
			_ = systemd.Stop(unit)
		}
		fmt.Printf("  Removing systemd unit for %s...\n", name)
		_ = systemd.RemoveVMUnit(name)
		fmt.Printf("  Deleting VM data for %s...\n", name)
		_ = vm.DeleteVMState(name)
	}

	// Step 2: Stop and disable fcm-bridge.service and fcm-dhcp.service
	fmt.Println("[2/6] Stopping and disabling FCM services...")
	for _, svc := range []string{"fcm-dhcp.service", "fcm-bridge.service"} {
		if systemd.IsActive(svc) {
			fmt.Printf("  Stopping %s...\n", svc)
			_ = systemd.Stop(svc)
		}
		fmt.Printf("  Disabling %s...\n", svc)
		_ = systemd.Disable(svc)
	}

	// Step 3: Remove iptables NAT rules
	fmt.Println("[3/6] Removing iptables NAT rules...")
	network.CleanupNAT(cfg)

	// Step 4: Delete the bridge
	fmt.Println("[4/6] Deleting network bridge...")
	if network.BridgeExists(cfg.BridgeName) {
		if err := network.TeardownBridge(cfg); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
	} else {
		fmt.Println("  Bridge not present, skipping")
	}

	// Step 5: Remove /var/lib/fcm
	fmt.Println("[5/6] Removing /var/lib/fcm...")
	if err := os.RemoveAll(config.DefaultBaseDir); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Step 6: Remove systemd unit files for fcm-*
	fmt.Println("[6/6] Removing FCM systemd unit files...")
	units, _ := filepath.Glob("/etc/systemd/system/fcm-*")
	for _, u := range units {
		fmt.Printf("  Removing %s\n", u)
		_ = os.Remove(u)
	}
	if len(units) > 0 {
		_ = systemd.DaemonReload()
	}

	fmt.Println()
	fmt.Println("FCM cleanup complete. All VMs and state have been removed.")
	return nil
}
