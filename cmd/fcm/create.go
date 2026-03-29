package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fcm.dev/fcm-cli/internal/cloudinit"
	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/images"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var createFlags struct {
	image     string
	cpus      int
	memory    int
	disk      int
	sshKey    string
	cloudInit string
}

var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create and start a new VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVar(&createFlags.image, "image", "", "Image name (e.g., ubuntu-22.04)")
	createCmd.Flags().IntVar(&createFlags.cpus, "cpus", 0, "Number of vCPUs (default from config)")
	createCmd.Flags().IntVar(&createFlags.memory, "memory", 0, "Memory in MB (default from config)")
	createCmd.Flags().IntVar(&createFlags.disk, "disk", 0, "Disk size in GB (default from config)")
	createCmd.Flags().StringVar(&createFlags.sshKey, "ssh-key", "", "Path to SSH public key")
	createCmd.Flags().StringVar(&createFlags.cloudInit, "cloud-init", "", "Path to cloud-init YAML file")
	_ = createCmd.MarkFlagRequired("image")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	if err := vm.ValidateName(name); err != nil {
		return err
	}

	if vm.Exists(name) {
		return fmt.Errorf("vm %q already exists", name)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check that fcm init has been run
	if !network.BridgeExists(cfg.BridgeName) {
		return fmt.Errorf("network bridge %s not found — run 'fcm init' first", cfg.BridgeName)
	}

	// Apply defaults
	cpus := createFlags.cpus
	if cpus == 0 {
		cpus = cfg.DefaultCPUs
	}
	memory := createFlags.memory
	if memory == 0 {
		memory = cfg.DefaultMemoryMB
	}
	disk := createFlags.disk
	if disk == 0 {
		disk = cfg.DefaultDiskGB
	}

	// Validate
	if cpus < 1 || cpus > 32 {
		return fmt.Errorf("--cpus must be between 1 and 32")
	}
	if memory < 128 {
		return fmt.Errorf("--memory must be at least 128 MB")
	}
	if disk < 1 {
		return fmt.Errorf("--disk must be at least 1 GB")
	}

	// Read SSH key if provided
	var sshPubKey string
	if createFlags.sshKey != "" {
		data, err := os.ReadFile(createFlags.sshKey)
		if err != nil {
			return fmt.Errorf("read ssh key: %w", err)
		}
		sshPubKey = string(data)
	}

	// Auto-pull image if not cached
	if !images.Exists(createFlags.image) {
		fmt.Printf("Image %q not found locally, pulling...\n", createFlags.image)
		if err := images.Pull(createFlags.image); err != nil {
			return fmt.Errorf("pull image: %w", err)
		}
	}

	// The rest must happen under a lock to prevent IP races
	var v *vm.VM
	err = vm.WithLock(func() error {
		// Allocate IP
		ip, err := network.AllocateIP(cfg)
		if err != nil {
			return err
		}

		mac := network.MACFromIP(ip)
		tapDevice := network.TAPName(name)
		vmDir := vm.VMDir(name)

		v = &vm.VM{
			Name:       name,
			Image:      createFlags.image,
			Kernel:     cfg.DefaultKernel,
			CPUs:       cpus,
			MemoryMB:   memory,
			DiskGB:     disk,
			IP:         ip,
			Gateway:    cfg.BridgeIP,
			MAC:        mac,
			TAPDevice:  tapDevice,
			SocketPath: filepath.Join(vmDir, "fc.socket"),
			RootfsPath: filepath.Join(vmDir, "rootfs.ext4"),
			CIDataPath: filepath.Join(vmDir, "cidata.iso"),
			SerialLog:  filepath.Join(vmDir, "console.log"),
			CreatedAt:  time.Now(),
			BootArgs:   network.BootArgs(),
		}

		// Create VM directory
		if err := os.MkdirAll(vmDir, 0700); err != nil {
			return fmt.Errorf("create vm dir: %w", err)
		}

		// Save state early to reserve the IP
		if err := vm.SaveVM(v); err != nil {
			os.RemoveAll(vmDir) // rollback
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// From here, rollback on failure by removing the VM directory
	rollback := func() {
		_ = os.RemoveAll(vm.VMDir(name))
		_ = systemd.RemoveVMUnit(name)
	}

	// Copy and resize image
	fmt.Printf("Preparing rootfs (%d GB)...\n", disk)
	if err := images.CopyForVM(createFlags.image, v.RootfsPath, disk); err != nil {
		rollback()
		return fmt.Errorf("prepare rootfs: %w", err)
	}

	// Generate cloud-init CIDATA disk — handles SSH keys, networking, hostname
	fmt.Println("Generating cloud-init...")
	netCfg := &cloudinit.NetworkConfig{
		IP:      v.IP,
		Gateway: v.Gateway,
		Mask:    cfg.BridgeMask,
		DNS:     cfg.DNS,
	}
	if err := cloudinit.GenerateCloudInitDisk(v.CIDataPath, name, sshPubKey, createFlags.cloudInit, netCfg); err != nil {
		rollback()
		return fmt.Errorf("generate cloud-init: %w", err)
	}

	// Create serial log file
	_ = os.WriteFile(v.SerialLog, nil, 0600)

	// Generate systemd unit
	if err := systemd.WriteVMUnit(v); err != nil {
		rollback()
		return fmt.Errorf("write systemd unit: %w", err)
	}

	// Enable and start
	unit := systemd.VMUnitName(name)
	if err := systemd.Enable(unit); err != nil {
		rollback()
		return fmt.Errorf("enable unit: %w", err)
	}

	fmt.Printf("Starting %s...\n", name)
	if err := systemd.Start(unit); err != nil {
		rollback()
		return fmt.Errorf("start vm: %w", err)
	}

	fmt.Printf("\nVM %s created and started:\n", name)
	fmt.Printf("  IP:     %s\n", v.IP)
	fmt.Printf("  CPUs:   %d\n", v.CPUs)
	fmt.Printf("  Memory: %d MB\n", v.MemoryMB)
	fmt.Printf("  Disk:   %d GB\n", v.DiskGB)
	fmt.Printf("  Image:  %s\n", v.Image)
	fmt.Printf("\nAccess:\n")
	fmt.Printf("  SSH:     ssh root@%s\n", v.IP)
	fmt.Printf("  Console: fcm console %s\n", name)
	return nil
}
