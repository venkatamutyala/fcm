package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var backupOutput string
var backupPrune int

var backupCmd = &cobra.Command{
	Use:   "backup [name]",
	Short: "Create a disk backup of a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runBackup,
}

var backupsCmd = &cobra.Command{
	Use:   "backups [name]",
	Short: "List backups for a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runBackups,
}

var restoreCmd = &cobra.Command{
	Use:   "restore [name] [backup-file]",
	Short: "Restore a VM from a backup",
	Args:  cobra.ExactArgs(2),
	RunE:  runRestore,
}

var backupRmCmd = &cobra.Command{
	Use:   "backup-rm [backup-name]",
	Short: "Delete a backup file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		if err := vm.DeleteBackup(args[0]); err != nil {
			return err
		}
		fmt.Printf("Backup %s deleted\n", args[0])
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupOutput, "output", "", "Custom output path")
	backupCmd.Flags().IntVar(&backupPrune, "prune", 0, "Delete backups older than N days")
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(backupsCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(backupRmCmd)
}

func runBackup(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	unit := systemd.VMUnitName(name)
	wasRunning := systemd.IsActive(unit)

	// Stop VM for consistent backup
	if wasRunning {
		fmt.Printf("Stopping %s for backup...\n", name)
		if err := systemd.Stop(unit); err != nil {
			return fmt.Errorf("stop vm: %w", err)
		}
	}

	fmt.Printf("Backing up %s...\n", name)
	outputPath, err := vm.BackupVM(v, backupOutput)
	if err != nil {
		// Restart even on failure
		if wasRunning {
			_ = systemd.Start(unit)
		}
		return err
	}

	// Restart if it was running
	if wasRunning {
		fmt.Printf("Restarting %s...\n", name)
		if err := systemd.Start(unit); err != nil {
			return fmt.Errorf("restart vm after backup: %w", err)
		}
	}

	fmt.Printf("Backup saved to %s\n", outputPath)

	// Prune old backups if requested
	if backupPrune > 0 {
		pruned, err := vm.PruneBackups(name, backupPrune)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: prune failed: %v\n", err)
		} else if pruned > 0 {
			fmt.Printf("Pruned %d old backup(s)\n", pruned)
		}
	}

	return nil
}

func runBackups(cmd *cobra.Command, args []string) error {
	name := args[0]
	backups, err := vm.ListBackups(name)
	if err != nil {
		return err
	}

	if len(backups) == 0 {
		fmt.Printf("No backups found for %s\n", name)
		return nil
	}

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(backups)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tTYPE\tSIZE\tCREATED\n")
	for _, b := range backups {
		fmt.Fprintf(w, "%s\t%s\t%d MB\t%s\n",
			b.Name, b.Type, b.Size/1024/1024, b.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	w.Flush()
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	backupFile := args[1]

	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	unit := systemd.VMUnitName(name)
	if systemd.IsActive(unit) {
		fmt.Printf("Stopping %s for restore...\n", name)
		if err := systemd.Stop(unit); err != nil {
			return fmt.Errorf("stop vm: %w", err)
		}
	}

	// Resolve backup path — check if it's a full path or just a name
	backupPath := backupFile
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		backupPath = vm.BackupPath(backupFile)
	}

	fmt.Printf("Restoring %s from %s...\n", name, backupPath)
	if err := vm.RestoreVM(v, backupPath); err != nil {
		return err
	}

	fmt.Printf("Starting %s...\n", name)
	if err := systemd.Start(unit); err != nil {
		return fmt.Errorf("start vm after restore: %w", err)
	}

	fmt.Printf("VM %s restored and running\n", name)
	return nil
}
