package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"github.com/spf13/cobra"
)

const (
	// Firecracker release to download
	firecrackerVersion = "v1.12.0"
	firecrackerRepo    = "firecracker-microvm/firecracker"

	// Kernel — using Firecracker's CI kernel (known to work)
	kernelURL = "https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize FCM: download firecracker, kernel, set up networking",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	// Step 1: Directory structure
	fmt.Println("[1/5] Creating directory structure...")
	if err := config.EnsureDirs(); err != nil {
		return err
	}

	// Step 2: Download Firecracker
	fcPath := "/usr/local/bin/firecracker"
	if _, err := os.Stat(fcPath); os.IsNotExist(err) {
		fmt.Println("[2/5] Downloading Firecracker...")
		if err := downloadFirecracker(fcPath); err != nil {
			return fmt.Errorf("download firecracker: %w", err)
		}
	} else {
		fmt.Println("[2/5] Firecracker already installed, skipping")
	}

	// Step 3: Download kernel
	kernelPath := filepath.Join(config.DefaultBaseDir, "kernels", "vmlinux-default")
	if _, err := os.Stat(kernelPath); os.IsNotExist(err) {
		fmt.Println("[3/5] Downloading Linux kernel...")
		if err := downloadToFile(kernelURL, kernelPath); err != nil {
			return fmt.Errorf("download kernel: %w", err)
		}
		fmt.Printf("    Saved to %s\n", kernelPath)
	} else {
		fmt.Println("[3/5] Kernel already present, skipping")
	}

	// Step 4: Write config
	fmt.Println("[4/5] Writing config...")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.FirecrackerVersion = firecrackerVersion
	cfg.FCMVersion = Version
	if err := config.Save(cfg); err != nil {
		return err
	}

	// Step 5: Set up bridge and NAT
	fmt.Println("[5/6] Setting up network bridge...")
	if err := systemd.WriteBridgeUnit(); err != nil {
		return fmt.Errorf("write bridge unit: %w", err)
	}
	if err := systemd.Enable("fcm-bridge.service"); err != nil {
		return fmt.Errorf("enable bridge service: %w", err)
	}
	if err := systemd.Start("fcm-bridge.service"); err != nil {
		fmt.Println("    systemd bridge service failed, setting up directly...")
		if err := network.SetupBridge(cfg); err != nil {
			return fmt.Errorf("setup bridge: %w", err)
		}
	}

	// Step 6: Start DHCP server
	fmt.Println("[6/6] Starting DHCP server...")
	if err := systemd.WriteDHCPUnit(); err != nil {
		return fmt.Errorf("write dhcp unit: %w", err)
	}
	if err := systemd.Enable("fcm-dhcp.service"); err != nil {
		return fmt.Errorf("enable dhcp service: %w", err)
	}
	if err := systemd.Start("fcm-dhcp.service"); err != nil {
		return fmt.Errorf("start dhcp service: %w", err)
	}

	fmt.Println()
	fmt.Println("FCM initialized successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  fcm doctor              # verify everything works")
	fmt.Println("  fcm pull alpine-3.20    # download an image")
	fmt.Println("  fcm create my-vm --image alpine-3.20 --ssh-key ~/.ssh/id_ed25519.pub")
	return nil
}

func downloadFirecracker(destPath string) error {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}

	// Firecracker releases are tarballs containing the binary
	url := fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/firecracker-%s-%s.tgz",
		firecrackerRepo, firecrackerVersion, firecrackerVersion, arch,
	)

	tmpDir, err := os.MkdirTemp("", "fcm-fc-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tgzPath := filepath.Join(tmpDir, "firecracker.tgz")
	fmt.Printf("    Downloading from %s\n", url)
	if err := downloadToFile(url, tgzPath); err != nil {
		return err
	}

	// Extract the firecracker binary from the tarball
	fmt.Println("    Extracting...")
	cmd := exec.Command("tar", "xzf", tgzPath, "-C", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract tarball: %s: %w", string(out), err)
	}

	// Find the firecracker binary in extracted files
	fcBinary, err := findFirecrackerBinary(tmpDir, arch)
	if err != nil {
		return err
	}

	// Copy to destination
	if err := copyFilePath(fcBinary, destPath); err != nil {
		return err
	}
	if err := os.Chmod(destPath, 0755); err != nil {
		return err
	}

	// Verify it runs
	out, err := exec.Command(destPath, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("firecracker --version failed: %s: %w", string(out), err)
	}
	fmt.Printf("    Installed: %s", string(out))
	return nil
}

func findFirecrackerBinary(dir, arch string) (string, error) {
	// Firecracker tarballs extract to: release-<version>-<arch>/firecracker-<version>-<arch>
	pattern := filepath.Join(dir, "release-*", "firecracker-*")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		// Try flat layout
		pattern = filepath.Join(dir, "firecracker-*")
		matches, _ = filepath.Glob(pattern)
	}

	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		// Skip .debug files
		if filepath.Ext(m) == ".debug" {
			continue
		}
		return m, nil
	}

	return "", fmt.Errorf("firecracker binary not found in extracted tarball")
}

func downloadToFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("    Downloaded %d MB\n", written/1024/1024)
	return nil
}

func copyFilePath(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
