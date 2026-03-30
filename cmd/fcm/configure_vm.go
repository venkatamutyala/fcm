package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fcm.dev/fcm-cli/internal/firecracker"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var configureVMCmd = &cobra.Command{
	Use:    "_configure-vm [name]",
	Short:  "Configure and boot a VM via Firecracker API (internal)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		v, err := vm.LoadVM(name)
		if err != nil {
			return err
		}

		// Wait for Firecracker socket to be ready
		if err := firecracker.WaitForSocket(v.SocketPath, 5*time.Second); err != nil {
			return err
		}

		fc := firecracker.NewClient(v.SocketPath)

		// Check if this is an unfreeze (snapshot exists)
		vmDir := vm.VMDir(name)
		snapPath := filepath.Join(vmDir, "snapshot.snap")
		memPath := filepath.Join(vmDir, "snapshot.mem")
		if _, err := os.Stat(snapPath); err == nil {
			fmt.Printf("Restoring %s from snapshot...\n", name)
			if err := fc.LoadSnapshot(firecracker.SnapshotLoad{
				SnapshotPath: snapPath,
				MemBackend: firecracker.MemBackend{
					BackendPath: memPath,
					BackendType: "File",
				},
			}); err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			if err := fc.ResumeVM(); err != nil {
				return fmt.Errorf("resume vm: %w", err)
			}
			fmt.Printf("VM %s resumed from snapshot\n", name)
			return nil
		}

		// Normal boot: configure in the required order
		fmt.Printf("Configuring %s...\n", name)

		if err := fc.PutMachineConfig(firecracker.MachineConfig{
			VCPUCount:  v.CPUs,
			MemSizeMiB: v.MemoryMB,
		}); err != nil {
			return fmt.Errorf("set machine config: %w", err)
		}

		if err := fc.PutBootSource(firecracker.BootSource{
			KernelImagePath: v.Kernel,
			BootArgs:        v.BootArgs,
		}); err != nil {
			return fmt.Errorf("set boot source: %w", err)
		}

		// Build drive rate limiter if configured
		var driveRL *firecracker.RateLimiter
		if v.DiskBandwidth != "" || v.DiskIOPS > 0 {
			driveRL = &firecracker.RateLimiter{}
			if v.DiskBandwidth != "" {
				bw := parseBandwidth(v.DiskBandwidth)
				if bw > 0 {
					driveRL.Bandwidth = &firecracker.TokenBucket{
						Size:       bw,
						RefillTime: 1000, // 1 second
					}
				}
			}
			if v.DiskIOPS > 0 {
				driveRL.Ops = &firecracker.TokenBucket{
					Size:       int64(v.DiskIOPS),
					RefillTime: 1000,
				}
			}
		}

		rootfsDrive := firecracker.Drive{
			DriveID:      "rootfs",
			PathOnHost:   v.RootfsPath,
			IsRootDevice: true,
			IsReadOnly:   false,
			RateLimiter:  driveRL,
		}
		if err := fc.PutDrive("rootfs", rootfsDrive); err != nil {
			return fmt.Errorf("set rootfs drive: %w", err)
		}

		if v.CIDataPath != "" {
			if err := fc.PutDrive("cidata", firecracker.Drive{
				DriveID:      "cidata",
				PathOnHost:   v.CIDataPath,
				IsRootDevice: false,
				IsReadOnly:   true,
			}); err != nil {
				return fmt.Errorf("set cidata drive: %w", err)
			}
		}

		// Build network rate limiter if configured
		var netRL *firecracker.RateLimiter
		if v.NetBandwidth != "" {
			bw := parseBandwidth(v.NetBandwidth)
			if bw > 0 {
				netRL = &firecracker.RateLimiter{
					Bandwidth: &firecracker.TokenBucket{
						Size:       bw,
						RefillTime: 1000,
					},
				}
			}
		}

		if err := fc.PutNetworkInterface("eth0", firecracker.NetworkInterface{
			IfaceID:     "eth0",
			GuestMAC:    v.MAC,
			HostDevName: v.TAPDevice,
			RateLimiter: netRL,
		}); err != nil {
			return fmt.Errorf("set network interface: %w", err)
		}

		if err := fc.StartInstance(); err != nil {
			return fmt.Errorf("start instance: %w", err)
		}

		fmt.Printf("VM %s booting\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configureVMCmd)
}

// parseBandwidth converts a bandwidth string like "100mbps" or "50mbps"
// into bytes per second. Returns 0 if the string cannot be parsed.
func parseBandwidth(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))

	if strings.HasSuffix(s, "gbps") {
		s = strings.TrimSuffix(s, "gbps")
		val := parseInt64(s)
		return val * 1000000000 // e.g., "1gbps" = 1,000,000,000 bytes/sec
	}
	if strings.HasSuffix(s, "mbps") {
		s = strings.TrimSuffix(s, "mbps")
		val := parseInt64(s)
		return val * 1000000 // e.g., "100mbps" = 100,000,000 bytes/sec
	}
	if strings.HasSuffix(s, "kbps") {
		s = strings.TrimSuffix(s, "kbps")
		val := parseInt64(s)
		return val * 1000 // e.g., "100kbps" = 100,000 bytes/sec
	}

	// Assume raw bytes/sec
	return parseInt64(s)
}

func parseInt64(s string) int64 {
	var v int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int64(c-'0')
		}
	}
	return v
}
