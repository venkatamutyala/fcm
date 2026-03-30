package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fcm.dev/fcm-cli/internal/firecracker"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var freezeCmd = &cobra.Command{
	Use:   "freeze [name]",
	Short: "Freeze a running VM (pause + save state to disk)",
	Long:  "Pauses the VM and saves its full state (memory + CPU) to disk. Use 'unfreeze' to resume exactly where it left off.",
	Args:  cobra.ExactArgs(1),
	RunE:  runFreeze,
}

var unfreezeCmd = &cobra.Command{
	Use:   "unfreeze [name]",
	Short: "Resume a frozen VM from saved state",
	Long:  "Resumes a frozen VM exactly where it left off — running processes, network connections, everything.",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnfreeze,
}

func init() {
	rootCmd.AddCommand(freezeCmd)
	rootCmd.AddCommand(unfreezeCmd)
}

func runFreeze(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	unit := systemd.VMUnitName(name)
	if !systemd.IsActive(unit) {
		return fmt.Errorf("vm %q is not running", name)
	}

	fc := firecracker.NewClient(v.SocketPath)

	fmt.Printf("Freezing %s...\n", name)

	// Pause VM
	if err := fc.PauseVM(); err != nil {
		return fmt.Errorf("pause: %w", err)
	}

	// Save snapshot to disk
	vmDir := vm.VMDir(name)
	snapPath := filepath.Join(vmDir, "snapshot.snap")
	memPath := filepath.Join(vmDir, "snapshot.mem")

	if err := fc.CreateSnapshot(firecracker.SnapshotCreate{
		SnapshotType: "Full",
		SnapshotPath: snapPath,
		MemFilePath:  memPath,
	}); err != nil {
		_ = fc.ResumeVM() // resume on failure
		return fmt.Errorf("snapshot: %w", err)
	}

	// Stop the Firecracker process (snapshot is on disk)
	if err := systemd.Stop(unit); err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	fmt.Printf("VM %s frozen\n", name)
	return nil
}

func runUnfreeze(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	if !vm.Exists(name) {
		return fmt.Errorf("vm %q not found", name)
	}

	// Check snapshot exists
	vmDir := vm.VMDir(name)
	snapPath := filepath.Join(vmDir, "snapshot.snap")
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		return fmt.Errorf("vm %q has no snapshot (use 'fcm freeze' first)", name)
	}

	unit := systemd.VMUnitName(name)
	if systemd.IsActive(unit) {
		return fmt.Errorf("vm %q is already running", name)
	}

	// Start the VM — _configure-vm detects the snapshot and loads it
	// instead of doing a fresh boot
	fmt.Printf("Unfreezing %s...\n", name)
	if err := systemd.Start(unit); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	fmt.Printf("VM %s resumed\n", name)
	return nil
}
