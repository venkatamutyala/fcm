package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"fcm.dev/fcm-cli/internal/images"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var resizeFlags struct {
	cpus   int
	memory int
	disk   int
}

var resizeCmd = &cobra.Command{
	Use:   "resize [name]",
	Short: "Resize a VM's CPUs, memory, or disk",
	Args:  cobra.ExactArgs(1),
	RunE:  runResize,
}

func init() {
	resizeCmd.Flags().IntVar(&resizeFlags.cpus, "cpus", 0, "New number of vCPUs")
	resizeCmd.Flags().IntVar(&resizeFlags.memory, "memory", 0, "New memory in MB")
	resizeCmd.Flags().IntVar(&resizeFlags.disk, "disk", 0, "New disk size in GB")
	rootCmd.AddCommand(resizeCmd)
}

func runResize(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	if resizeFlags.cpus == 0 && resizeFlags.memory == 0 && resizeFlags.disk == 0 {
		return fmt.Errorf("specify at least one of --cpus, --memory, or --disk")
	}

	// Validate new values
	if resizeFlags.cpus != 0 {
		if resizeFlags.cpus < 1 || resizeFlags.cpus > 32 {
			return fmt.Errorf("--cpus must be between 1 and 32")
		}
	}
	if resizeFlags.memory != 0 {
		if resizeFlags.memory < 128 {
			return fmt.Errorf("--memory must be at least 128 MB")
		}
	}
	if resizeFlags.disk != 0 {
		if resizeFlags.disk < v.DiskGB {
			return fmt.Errorf("--disk (%d GB) cannot be smaller than current size (%d GB)", resizeFlags.disk, v.DiskGB)
		}
		if resizeFlags.disk == v.DiskGB {
			fmt.Printf("Disk is already %d GB\n", v.DiskGB)
			resizeFlags.disk = 0 // nothing to do
		}
	}

	needsRestart := resizeFlags.cpus != 0 || resizeFlags.memory != 0
	unit := systemd.VMUnitName(name)
	wasRunning := systemd.IsActive(unit)

	// Handle disk resize (can work while stopped)
	if resizeFlags.disk != 0 {
		if wasRunning {
			fmt.Printf("Stopping %s for disk resize...\n", name)
			if err := systemd.Stop(unit); err != nil {
				return fmt.Errorf("stop vm: %w", err)
			}
		}

		fmt.Printf("Resizing disk from %d GB to %d GB...\n", v.DiskGB, resizeFlags.disk)
		sizeBytes := fmt.Sprintf("%dG", resizeFlags.disk)
		if err := exec.Command("truncate", "-s", sizeBytes, v.RootfsPath).Run(); err != nil {
			return fmt.Errorf("truncate rootfs: %w", err)
		}

		fsType := images.DetectFS(v.RootfsPath)
		switch fsType {
		case "ext4", "ext2", "ext3":
			if out, err := exec.Command("e2fsck", "-fy", v.RootfsPath).CombinedOutput(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() > 1 {
					return fmt.Errorf("e2fsck: %s: %w", string(out), err)
				}
			}
			if out, err := exec.Command("resize2fs", v.RootfsPath).CombinedOutput(); err != nil {
				return fmt.Errorf("resize2fs: %s: %w", string(out), err)
			}
		case "xfs":
			tmpMount, _ := os.MkdirTemp("", "fcm-xfs-resize-")
			defer os.RemoveAll(tmpMount)
			if out, err := exec.Command("mount", "-o", "loop", v.RootfsPath, tmpMount).CombinedOutput(); err != nil {
				return fmt.Errorf("mount xfs for resize: %s: %w", string(out), err)
			}
			_ = exec.Command("xfs_growfs", tmpMount).Run()
			_ = exec.Command("umount", tmpMount).Run()
		default:
			fmt.Printf("  Warning: unknown filesystem %q, skipping resize\n", fsType)
		}

		v.DiskGB = resizeFlags.disk
		fmt.Printf("  Disk resized to %d GB\n", v.DiskGB)
	}

	// Handle CPU/memory changes
	if needsRestart {
		if wasRunning && resizeFlags.disk == 0 {
			// Only stop if we didn't already stop for disk resize
			fmt.Printf("Stopping %s for CPU/memory resize...\n", name)
			if err := systemd.Stop(unit); err != nil {
				return fmt.Errorf("stop vm: %w", err)
			}
		}

		if resizeFlags.cpus != 0 {
			fmt.Printf("  CPUs: %d -> %d\n", v.CPUs, resizeFlags.cpus)
			v.CPUs = resizeFlags.cpus
		}
		if resizeFlags.memory != 0 {
			fmt.Printf("  Memory: %d MB -> %d MB\n", v.MemoryMB, resizeFlags.memory)
			v.MemoryMB = resizeFlags.memory
		}

		// Regenerate systemd unit with new config
		fmt.Println("Regenerating systemd unit...")
		if err := systemd.WriteVMUnit(v); err != nil {
			return fmt.Errorf("write systemd unit: %w", err)
		}
	}

	// Delete stale snapshot files so a stale snapshot isn't loaded on next unfreeze
	os.Remove(filepath.Join(vm.VMDir(name), "snapshot.snap"))
	os.Remove(filepath.Join(vm.VMDir(name), "snapshot.mem"))

	// Save updated VM state
	if err := vm.SaveVM(v); err != nil {
		return fmt.Errorf("save vm state: %w", err)
	}

	// Restart if it was running
	if wasRunning {
		fmt.Printf("Starting %s...\n", name)
		if err := systemd.Start(unit); err != nil {
			return fmt.Errorf("start vm: %w", err)
		}
	}

	fmt.Printf("VM %s resized successfully\n", name)
	return nil
}
