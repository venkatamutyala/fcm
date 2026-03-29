package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultBaseDir is the root directory for all FCM state.
	DefaultBaseDir = "/var/lib/fcm"

	// DefaultBridgeName is the name of the network bridge.
	DefaultBridgeName = "fcbr0"

	// DefaultBridgeIP is the bridge interface IP.
	DefaultBridgeIP = "192.168.100.1"

	// DefaultBridgeSubnet is the bridge subnet in CIDR notation.
	DefaultBridgeSubnet = "192.168.100.0/24"

	// DefaultBridgeMask is the subnet mask.
	DefaultBridgeMask = "255.255.255.0"

	// DefaultIPRangeStart is the first IP octet available for VMs.
	DefaultIPRangeStart = 10

	// DefaultDNS is the DNS server configured inside VMs.
	DefaultDNS = "8.8.8.8"

	// DefaultCPUs is the default number of vCPUs per VM.
	DefaultCPUs = 2

	// DefaultMemoryMB is the default memory per VM in MB.
	DefaultMemoryMB = 1024

	// DefaultDiskGB is the default disk size per VM in GB.
	DefaultDiskGB = 10
)

// Config holds the node-level FCM configuration.
type Config struct {
	BridgeName       string `json:"bridge_name"`
	BridgeIP         string `json:"bridge_ip"`
	BridgeSubnet     string `json:"bridge_subnet"`
	BridgeMask       string `json:"bridge_mask"`
	IPRangeStart     int    `json:"ip_range_start"`
	DNS              string `json:"dns"`
	DefaultKernel    string `json:"default_kernel"`
	DefaultCPUs      int    `json:"default_cpus"`
	DefaultMemoryMB  int    `json:"default_memory_mb"`
	DefaultDiskGB    int    `json:"default_disk_gb"`
	AutoUpdate       bool   `json:"auto_update"`
	FCMVersion       string `json:"fcm_version"`
	FirecrackerVersion string `json:"firecracker_version"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BridgeName:       DefaultBridgeName,
		BridgeIP:         DefaultBridgeIP,
		BridgeSubnet:     DefaultBridgeSubnet,
		BridgeMask:       DefaultBridgeMask,
		IPRangeStart:     DefaultIPRangeStart,
		DNS:              DefaultDNS,
		DefaultKernel:    filepath.Join(DefaultBaseDir, "kernels", "vmlinux-default"),
		DefaultCPUs:      DefaultCPUs,
		DefaultMemoryMB:  DefaultMemoryMB,
		DefaultDiskGB:    DefaultDiskGB,
		AutoUpdate:       false,
	}
}

// ConfigPath returns the path to the node config file.
func ConfigPath() string {
	return filepath.Join(DefaultBaseDir, "config.json")
}

// Load reads the config from disk.
func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the config to disk atomically.
func Save(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(ConfigPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp := ConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmp, ConfigPath()); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// Dirs returns all directories that FCM needs.
func Dirs() []string {
	return []string{
		DefaultBaseDir,
		filepath.Join(DefaultBaseDir, "kernels"),
		filepath.Join(DefaultBaseDir, "images"),
		filepath.Join(DefaultBaseDir, "vms"),
		filepath.Join(DefaultBaseDir, "cache"),
		filepath.Join(DefaultBaseDir, "backups"),
		filepath.Join(DefaultBaseDir, "templates"),
	}
}

// EnsureDirs creates all required directories.
func EnsureDirs() error {
	for _, dir := range Dirs() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}
