package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BridgeName != DefaultBridgeName {
		t.Errorf("BridgeName = %q, want %q", cfg.BridgeName, DefaultBridgeName)
	}
	if cfg.DefaultCPUs != DefaultCPUs {
		t.Errorf("DefaultCPUs = %d, want %d", cfg.DefaultCPUs, DefaultCPUs)
	}
	if cfg.DefaultMemoryMB != DefaultMemoryMB {
		t.Errorf("DefaultMemoryMB = %d, want %d", cfg.DefaultMemoryMB, DefaultMemoryMB)
	}
	if cfg.IPRangeStart != DefaultIPRangeStart {
		t.Errorf("IPRangeStart = %d, want %d", cfg.IPRangeStart, DefaultIPRangeStart)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FCMVersion = "0.1.0"
	cfg.FirecrackerVersion = "1.12.1"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.FCMVersion != "0.1.0" {
		t.Errorf("FCMVersion = %q, want %q", loaded.FCMVersion, "0.1.0")
	}
	if loaded.BridgeSubnet != DefaultBridgeSubnet {
		t.Errorf("BridgeSubnet = %q, want %q", loaded.BridgeSubnet, DefaultBridgeSubnet)
	}
}

func TestEnsureDirs(t *testing.T) {
	// Create a temp directory to verify Dirs() returns expected paths
	dirs := Dirs()
	if len(dirs) == 0 {
		t.Fatal("Dirs() returned empty list")
	}

	// Verify all paths are under DefaultBaseDir
	for _, dir := range dirs {
		rel, err := filepath.Rel(DefaultBaseDir, dir)
		if err != nil {
			t.Errorf("dir %q is not relative to %q: %v", dir, DefaultBaseDir, err)
		}
		if len(rel) >= 2 && rel[:2] == ".." {
			t.Errorf("dir %q is outside base dir %q", dir, DefaultBaseDir)
		}
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Use a temp dir to test save/load without needing /var/lib/fcm
	tmpDir := t.TempDir()
	tmpConfig := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.FCMVersion = "test-version"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(tmpConfig, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	readData, err := os.ReadFile(tmpConfig)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.FCMVersion != "test-version" {
		t.Errorf("FCMVersion = %q, want %q", loaded.FCMVersion, "test-version")
	}
}
