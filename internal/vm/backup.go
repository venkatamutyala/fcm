package vm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fcm.dev/fcm-cli/internal/config"
)

const backupsDir = "/var/lib/fcm/backups"

// Backup represents a disk backup.
type Backup struct {
	Name      string
	Type      string // "disk" or "snapshot"
	Path      string
	Size      int64
	CreatedAt time.Time
}

// BackupVM performs a disk backup: copies rootfs and vm.json.
func BackupVM(v *VM, outputPath string) (string, error) {
	if err := os.MkdirAll(backupsDir, 0700); err != nil {
		return "", fmt.Errorf("create backups dir: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")

	if outputPath == "" {
		outputPath = filepath.Join(backupsDir, fmt.Sprintf("%s-%s.ext4", v.Name, timestamp))
	}

	// Copy rootfs
	if err := copyFile(v.RootfsPath, outputPath); err != nil {
		return "", fmt.Errorf("copy rootfs: %w", err)
	}

	// Copy vm.json alongside
	jsonPath := strings.TrimSuffix(outputPath, ".ext4") + ".json"
	srcJSON := VMStatePath(v.Name)
	if err := copyFile(srcJSON, jsonPath); err != nil {
		// Non-fatal, rootfs backup is what matters
		fmt.Fprintf(os.Stderr, "warning: could not backup vm.json: %v\n", err)
	}

	return outputPath, nil
}

// RestoreVM replaces a VM's rootfs with a backup file.
func RestoreVM(v *VM, backupPath string) error {
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	if err := copyFile(backupPath, v.RootfsPath); err != nil {
		return fmt.Errorf("restore rootfs: %w", err)
	}

	return nil
}

// ListBackups returns all backups for a VM.
func ListBackups(vmName string) ([]Backup, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}

	prefix := vmName + "-"
	var backups []Backup

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".ext4") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		backups = append(backups, Backup{
			Name:      e.Name(),
			Type:      "disk",
			Path:      filepath.Join(backupsDir, e.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// DeleteBackup removes a backup file and its associated JSON.
func DeleteBackup(name string) error {
	path := filepath.Join(backupsDir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("backup %q not found", name)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove backup: %w", err)
	}

	// Also remove associated JSON if it exists
	jsonPath := strings.TrimSuffix(path, ".ext4") + ".json"
	os.Remove(jsonPath)

	return nil
}

// PruneBackups removes backups older than the given number of days.
func PruneBackups(vmName string, keepDays int) (int, error) {
	backups, err := ListBackups(vmName)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -keepDays)
	pruned := 0

	for _, b := range backups {
		if b.CreatedAt.Before(cutoff) {
			if err := DeleteBackup(b.Name); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not prune %s: %v\n", b.Name, err)
				continue
			}
			pruned++
		}
	}

	return pruned, nil
}

// BackupPath returns the full path to a backup file.
func BackupPath(name string) string {
	return filepath.Join(config.DefaultBaseDir, "backups", name)
}

func copyFile(src, dst string) error {
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

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
