package vm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fcm.dev/fcm-cli/internal/config"
	"golang.org/x/sys/unix"
)

const lockFile = "/var/lib/fcm/.lock"

// VM represents the persisted state of a virtual machine.
type VM struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Kernel       string            `json:"kernel"`
	CPUs         int               `json:"cpus"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	IP           string            `json:"ip"`
	Gateway      string            `json:"gateway"`
	MAC          string            `json:"mac"`
	TAPDevice    string            `json:"tap_device"`
	SocketPath   string            `json:"socket_path"`
	RootfsPath   string            `json:"rootfs_path"`
	CIDataPath   string            `json:"cidata_path"`
	SerialLog    string            `json:"serial_log"`
	Tags         map[string]string `json:"tags,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	BootArgs     string            `json:"boot_args"`
	Forwards     map[string]string `json:"forwards,omitempty"`
	Isolated     bool              `json:"isolated,omitempty"`
	NetBandwidth string            `json:"net_bandwidth,omitempty"`
	DiskIOPS     int               `json:"disk_iops,omitempty"`
	DiskBandwidth string           `json:"disk_bandwidth,omitempty"`
}

// VMDir returns the directory for a given VM.
func VMDir(name string) string {
	return filepath.Join(config.DefaultBaseDir, "vms", name)
}

// VMStatePath returns the path to a VM's state file.
func VMStatePath(name string) string {
	return filepath.Join(VMDir(name), "vm.json")
}

// LoadVM reads a VM's state from disk.
func LoadVM(name string) (*VM, error) {
	data, err := os.ReadFile(VMStatePath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("vm %q not found", name)
		}
		return nil, fmt.Errorf("read vm state: %w", err)
	}

	var v VM
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse vm state for %q: %w", name, err)
	}
	return &v, nil
}

// SaveVM writes a VM's state to disk atomically.
func SaveVM(v *VM) error {
	dir := VMDir(v.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create vm dir: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal vm state: %w", err)
	}

	tmp := VMStatePath(v.Name) + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write vm state: %w", err)
	}

	if err := os.Rename(tmp, VMStatePath(v.Name)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename vm state: %w", err)
	}
	return nil
}

// ListVMs returns all VM names by scanning the vms directory.
func ListVMs() ([]string, error) {
	vmsDir := filepath.Join(config.DefaultBaseDir, "vms")
	entries, err := os.ReadDir(vmsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list vms: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only include directories that have a vm.json
		if _, err := os.Stat(VMStatePath(e.Name())); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// LoadAllVMs reads state for all VMs.
func LoadAllVMs() ([]*VM, error) {
	names, err := ListVMs()
	if err != nil {
		return nil, err
	}

	var vms []*VM
	for _, name := range names {
		v, err := LoadVM(name)
		if err != nil {
			continue // skip broken state files
		}
		vms = append(vms, v)
	}
	return vms, nil
}

// Exists returns true if a VM with the given name exists.
func Exists(name string) bool {
	_, err := os.Stat(VMStatePath(name))
	return err == nil
}

// DeleteVMState removes a VM's entire directory.
func DeleteVMState(name string) error {
	dir := VMDir(name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove vm dir %q: %w", name, err)
	}
	return nil
}

// WithLock acquires an exclusive file lock and runs fn.
// This prevents concurrent state mutations (e.g., IP allocation races).
func WithLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockFile), 0700); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}

	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer f.Close()

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }()

	return fn()
}

// ValidateName checks that a VM name is valid (DNS-label rules).
func ValidateName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("vm name cannot be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("vm name %q exceeds 63 characters", name)
	}

	for i, c := range name {
		if c >= 'a' && c <= 'z' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i > 0 && i < len(name)-1 {
			continue
		}
		return fmt.Errorf("vm name %q contains invalid character %q (allowed: lowercase alphanumeric and hyphens, cannot start or end with hyphen)", name, string(c))
	}

	if name[0] >= '0' && name[0] <= '9' {
		return fmt.Errorf("vm name %q must start with a letter", name)
	}
	return nil
}
