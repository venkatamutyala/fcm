package cloudinit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GenerateCloudInitDisk creates a NoCloud cloud-init vfat disk image.
// Uses vfat instead of ISO 9660 because Firecracker kernels typically
// don't include the iso9660 module.
// See: https://docs.cloud-init.io/en/latest/reference/datasources/nocloud.html
func GenerateCloudInitDisk(outputPath, hostname, sshPubKey, cloudInitFile string, netCfg *NetworkConfig) error {
	tmpDir, err := os.MkdirTemp("", "fcm-cidata-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write meta-data
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", hostname, hostname)
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return fmt.Errorf("write meta-data: %w", err)
	}

	// Write user-data
	var userData string
	if cloudInitFile != "" {
		data, err := os.ReadFile(cloudInitFile)
		if err != nil {
			return fmt.Errorf("read cloud-init file: %w", err)
		}
		userData = string(data)
		if !strings.HasPrefix(strings.TrimSpace(userData), "#cloud-config") {
			userData = "#cloud-config\n" + userData
		}
	} else {
		userData = defaultUserData(hostname, sshPubKey)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
		return fmt.Errorf("write user-data: %w", err)
	}

	// Write network-config
	if netCfg != nil {
		networkData := generateNetworkConfig(netCfg)
		if err := os.WriteFile(filepath.Join(tmpDir, "network-config"), []byte(networkData), 0644); err != nil {
			return fmt.Errorf("write network-config: %w", err)
		}
	}

	// Create a vfat disk image with label CIDATA
	// 1. Create a small disk image (8MB is plenty)
	if err := exec.Command("truncate", "-s", "8M", outputPath).Run(); err != nil {
		return fmt.Errorf("create disk image: %w", err)
	}

	// 2. Format as vfat with label CIDATA
	if out, err := exec.Command("mkfs.vfat", "-n", "CIDATA", outputPath).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.vfat: %s: %w", string(out), err)
	}

	// 3. Copy files into the vfat image using mcopy (from mtools)
	for _, name := range []string{"meta-data", "user-data", "network-config"} {
		src := filepath.Join(tmpDir, name)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if out, err := exec.Command("mcopy", "-i", outputPath, src, "::"+name).CombinedOutput(); err != nil {
			return fmt.Errorf("mcopy %s: %s: %w", name, string(out), err)
		}
	}

	return nil
}

// NetworkConfig holds guest network settings for cloud-init.
type NetworkConfig struct {
	IP      string
	Gateway string
	Mask    string
	DNS     string
}

func generateNetworkConfig(cfg *NetworkConfig) string {
	cidr := maskToCIDRBits(cfg.Mask)
	nc := "network:\n"
	nc += "  version: 2\n"
	nc += "  ethernets:\n"
	nc += "    eth0:\n"
	nc += fmt.Sprintf("      addresses: [\"%s/%s\"]\n", cfg.IP, cidr)
	nc += fmt.Sprintf("      gateway4: %s\n", cfg.Gateway)
	nc += "      nameservers:\n"
	nc += fmt.Sprintf("        addresses: [%s]\n", cfg.DNS)
	nc += "      match:\n"
	nc += "        name: \"eth*\"\n"
	return nc
}

func maskToCIDRBits(mask string) string {
	switch mask {
	case "255.255.255.0":
		return "24"
	case "255.255.0.0":
		return "16"
	case "255.0.0.0":
		return "8"
	default:
		return "24"
	}
}

// DefaultUserData generates the base cloud-init user-data with hostname,
// SSH key, and root password configuration.
func DefaultUserData(hostname, sshPubKey string) string {
	return defaultUserData(hostname, sshPubKey)
}

func defaultUserData(hostname, sshPubKey string) string {
	cfg := "#cloud-config\n"
	cfg += fmt.Sprintf("hostname: %s\n", hostname)
	cfg += "manage_etc_hosts: true\n"

	// Configure root user explicitly with SSH key
	// Using the users directive ensures the key goes to root,
	// not the distro's default user (ubuntu, cloud-user, etc.)
	cfg += "users:\n"
	cfg += "  - name: root\n"
	cfg += "    lock_passwd: false\n"
	if sshPubKey != "" {
		sshPubKey = strings.TrimSpace(sshPubKey)
		cfg += "    ssh_authorized_keys:\n"
		cfg += fmt.Sprintf("      - %s\n", sshPubKey)
	}

	// Also inject at top level for distros where users directive
	// doesn't apply to root (belt and suspenders)
	if sshPubKey != "" {
		cfg += "ssh_authorized_keys:\n"
		cfg += fmt.Sprintf("  - %s\n", sshPubKey)
	}

	// Root password for console access
	cfg += "chpasswd:\n"
	cfg += "  list: |\n"
	cfg += "    root:fcm\n"
	cfg += "  expire: false\n"

	// SSH and root config
	cfg += "ssh_pwauth: true\n"
	cfg += "disable_root: false\n"

	return cfg
}
