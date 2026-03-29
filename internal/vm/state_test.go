package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"dev-box", false},
		{"web1", false},
		{"a", false},
		{"my-long-vm-name", false},
		{"", true},             // empty
		{"-bad", true},         // starts with hyphen
		{"bad-", true},         // ends with hyphen
		{"1bad", true},         // starts with digit
		{"BAD", true},          // uppercase
		{"bad name", true},     // space
		{"bad.name", true},     // dot
		{"bad_name", true},     // underscore
		{"bad/name", true},     // slash
		{string(make([]byte, 64)), true}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestVMStateRoundTrip(t *testing.T) {
	v := &VM{
		Name:      "test-vm",
		Image:     "ubuntu-22.04",
		Kernel:    "/var/lib/fcm/kernels/vmlinux-default",
		CPUs:      2,
		MemoryMB:  1024,
		DiskGB:    10,
		IP:        "192.168.100.10",
		Gateway:   "192.168.100.1",
		MAC:       "AA:FC:00:00:00:01",
		TAPDevice: "fcm0",
		CreatedAt: time.Now().Truncate(time.Second),
		Tags:      map[string]string{"env": "dev"},
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var loaded VM
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.Name != v.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, v.Name)
	}
	if loaded.IP != v.IP {
		t.Errorf("IP = %q, want %q", loaded.IP, v.IP)
	}
	if loaded.Tags["env"] != "dev" {
		t.Errorf("Tags[env] = %q, want %q", loaded.Tags["env"], "dev")
	}
}

func TestSaveAndLoadVM(t *testing.T) {
	tmpDir := t.TempDir()
	vmDir := filepath.Join(tmpDir, "test-vm")
	if err := os.MkdirAll(vmDir, 0700); err != nil {
		t.Fatal(err)
	}

	v := &VM{
		Name:     "test-vm",
		Image:    "ubuntu-22.04",
		CPUs:     4,
		MemoryMB: 2048,
		IP:       "192.168.100.11",
	}

	// Write directly to temp dir (bypasses VMDir path)
	statePath := filepath.Join(vmDir, "vm.json")
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	readData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var loaded VM
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.CPUs != 4 {
		t.Errorf("CPUs = %d, want 4", loaded.CPUs)
	}
	if loaded.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", loaded.MemoryMB)
	}
}
