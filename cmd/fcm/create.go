package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fcm.dev/fcm-cli/internal/cloudinit"
	"fcm.dev/fcm-cli/internal/config"
	fcmerrors "fcm.dev/fcm-cli/internal/errors"
	"fcm.dev/fcm-cli/internal/images"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/templates"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var createFlags struct {
	image        string
	cpus         int
	memory       int
	disk         int
	sshKey       string
	cloudInit    string
	ip           string
	template     string
	forward      []string
	isolated     bool
	netBandwidth string
	diskIOPS     int
	diskBandwidth string
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
	createCmd.Flags().StringVar(&createFlags.ip, "ip", "", "Static IP address for the VM (must be within configured subnet)")
	createCmd.Flags().StringVar(&createFlags.template, "template", "", "Use a built-in template (see: fcm templates)")
	createCmd.Flags().StringSliceVar(&createFlags.forward, "forward", nil, "Port forward in hostPort:vmPort format (can be repeated)")
	createCmd.Flags().BoolVar(&createFlags.isolated, "isolated", false, "Isolate VM from other VMs on the bridge")
	createCmd.Flags().StringVar(&createFlags.netBandwidth, "net-bandwidth", "", "Network bandwidth limit (e.g., 100mbps)")
	createCmd.Flags().IntVar(&createFlags.diskIOPS, "disk-iops", 0, "Disk IOPS limit")
	createCmd.Flags().StringVar(&createFlags.diskBandwidth, "disk-bandwidth", "", "Disk bandwidth limit (e.g., 50mbps)")
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
		return fcmerrors.WithBridgeHint(fmt.Errorf("network bridge %s not found", cfg.BridgeName))
	}

	// Resolve template
	var tmpl *templates.Template
	if createFlags.template != "" {
		tmpl = templates.Get(createFlags.template)
		if tmpl == nil {
			return fmt.Errorf("unknown template %q (see available templates with: fcm templates)", createFlags.template)
		}
		// Template provides image if --image not explicitly set
		if createFlags.image == "" {
			createFlags.image = tmpl.Image
		}
	}

	// Default to ubuntu-24.04 if no image or template is set
	if createFlags.image == "" {
		createFlags.image = "ubuntu-24.04"
		fmt.Println("Using default image: ubuntu-24.04")
	}

	// Apply defaults: template values override config defaults, explicit flags override everything
	cpus := createFlags.cpus
	if cpus == 0 {
		if tmpl != nil && tmpl.CPUs > 0 {
			cpus = tmpl.CPUs
		} else {
			cpus = cfg.DefaultCPUs
		}
	}
	memory := createFlags.memory
	if memory == 0 {
		if tmpl != nil && tmpl.Memory > 0 {
			memory = tmpl.Memory
		} else {
			memory = cfg.DefaultMemoryMB
		}
	}
	disk := createFlags.disk
	if disk == 0 {
		if tmpl != nil && tmpl.Disk > 0 {
			disk = tmpl.Disk
		} else {
			disk = cfg.DefaultDiskGB
		}
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

	// Read SSH key — auto-detect if not provided
	sshKeyPath := createFlags.sshKey
	if sshKeyPath == "" {
		sshKeyPath = autoDetectSSHKey()
	}
	var sshPubKey string
	if sshKeyPath != "" {
		data, err := os.ReadFile(sshKeyPath)
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
		// Allocate or validate IP
		var ip string
		if createFlags.ip != "" {
			if err := network.ValidateIP(createFlags.ip, cfg); err != nil {
				return fmt.Errorf("invalid --ip: %w", err)
			}
			ip = createFlags.ip
		} else {
			var err error
			ip, err = network.AllocateIP(cfg)
			if err != nil {
				return err
			}
		}

		mac := network.MACFromIP(ip)
		tapDevice := network.TAPName(name)
		vmDir := vm.VMDir(name)

		v = &vm.VM{
			Name:          name,
			Image:         createFlags.image,
			Kernel:        cfg.DefaultKernel,
			CPUs:          cpus,
			MemoryMB:      memory,
			DiskGB:        disk,
			IP:            ip,
			Gateway:       cfg.BridgeIP,
			MAC:           mac,
			TAPDevice:     tapDevice,
			SocketPath:    filepath.Join(vmDir, "fc.socket"),
			RootfsPath:    filepath.Join(vmDir, "rootfs.ext4"),
			CIDataPath:    filepath.Join(vmDir, "cidata.iso"),
			SerialLog:     filepath.Join(vmDir, "console.log"),
			CreatedAt:     time.Now(),
			BootArgs:      network.BootArgs(ip, cfg.BridgeIP, cfg.BridgeMask),
			Isolated:      createFlags.isolated,
			NetBandwidth:  createFlags.netBandwidth,
			DiskIOPS:      createFlags.diskIOPS,
			DiskBandwidth: createFlags.diskBandwidth,
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
	cloudInitFile := createFlags.cloudInit

	// Support cloud-init from URL
	if strings.HasPrefix(cloudInitFile, "http://") || strings.HasPrefix(cloudInitFile, "https://") {
		fmt.Printf("Downloading cloud-init from %s...\n", cloudInitFile)
		tmpFile := filepath.Join(vm.VMDir(name), "cloud-init-custom.yaml")
		resp, err := http.Get(cloudInitFile)
		if err != nil {
			rollback()
			return fmt.Errorf("download cloud-init: %w", err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			rollback()
			return fmt.Errorf("read cloud-init: %w", err)
		}
		if err := os.WriteFile(tmpFile, data, 0600); err != nil {
			rollback()
			return fmt.Errorf("save cloud-init: %w", err)
		}
		cloudInitFile = tmpFile
	}

	// If using a template with cloud-init and no explicit --cloud-init, generate merged user-data
	if cloudInitFile == "" && tmpl != nil && tmpl.CloudInit != "" {
		baseUserData := cloudinit.DefaultUserData(name, sshPubKey)
		merged := templates.MergeCloudInit(baseUserData, tmpl.CloudInit)

		tmpFile, err := os.CreateTemp("", "fcm-template-ci-*.yaml")
		if err != nil {
			rollback()
			return fmt.Errorf("create temp cloud-init: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(merged); err != nil {
			tmpFile.Close()
			rollback()
			return fmt.Errorf("write temp cloud-init: %w", err)
		}
		tmpFile.Close()
		cloudInitFile = tmpFile.Name()
	}

	netCfg := &cloudinit.NetworkConfig{
		IP:      v.IP,
		Gateway: v.Gateway,
		Mask:    cfg.BridgeMask,
		DNS:     cfg.DNS,
	}
	if err := cloudinit.GenerateCloudInitDisk(v.CIDataPath, name, sshPubKey, cloudInitFile, netCfg); err != nil {
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
	bootStart := time.Now()
	if err := systemd.Start(unit); err != nil {
		rollback()
		return fmt.Errorf("start vm: %w", err)
	}

	// Wait for SSH readiness
	waitForSSH(v.IP, bootStart)

	// Apply port forwards if requested
	if len(createFlags.forward) > 0 {
		// Reload v to get the saved state
		v, err = vm.LoadVM(name)
		if err != nil {
			return fmt.Errorf("reload vm state for forwards: %w", err)
		}
		if v.Forwards == nil {
			v.Forwards = make(map[string]string)
		}
		for _, fwd := range createFlags.forward {
			if err := addForward(v, fwd); err != nil {
				fmt.Printf("Warning: failed to add forward %s: %v\n", fwd, err)
			}
		}
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

// autoDetectSSHKey checks common SSH public key paths and returns the first
// one found. Returns empty string if none found.
func autoDetectSSHKey() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519.pub"),
		filepath.Join(home, ".ssh", "id_rsa.pub"),
		filepath.Join(home, ".ssh", "id_ecdsa.pub"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("Using SSH key: %s\n", p)
			return p
		}
	}
	fmt.Println("Warning: No SSH key found. VM accessible via console only (password: fcm)")
	return ""
}

// waitForSSH polls TCP port 22 on the VM's IP until it becomes reachable or
// the timeout (120s) expires.
func waitForSSH(ip string, bootStart time.Time) {
	timeout := 120 * time.Second
	poll := 2 * time.Second
	deadline := time.Now().Add(timeout)
	spinChars := []rune{'|', '/', '-', '\\'}
	i := 0

	for time.Now().Before(deadline) {
		elapsed := time.Since(bootStart).Truncate(time.Second)
		fmt.Printf("\r%c Waiting for VM to boot... (%s)", spinChars[i%len(spinChars)], elapsed)
		i++

		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "22"), poll)
		if err == nil {
			conn.Close()
			elapsed = time.Since(bootStart)
			fmt.Printf("\rVM ready! (booted in %.1fs)          \n", elapsed.Seconds())
			return
		}
		time.Sleep(poll)
	}

	fmt.Printf("\rVM started but SSH not yet reachable          \n")
}
