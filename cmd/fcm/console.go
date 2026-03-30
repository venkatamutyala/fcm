package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var consoleLines int

var consoleCmd = &cobra.Command{
	Use:   "console [name]",
	Short: "Attach to a VM's serial console log",
	Long:  "Follow the VM's serial console output (console.log). Press Ctrl+C to stop.",
	Args:  cobra.ExactArgs(1),
	RunE:  runConsole,
}

func init() {
	consoleCmd.Flags().IntVarP(&consoleLines, "lines", "n", 50, "Number of lines to show initially")
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	if _, err := os.Stat(v.SerialLog); os.IsNotExist(err) {
		return fmt.Errorf("console log not found: %s", v.SerialLog)
	}

	tailPath, err := exec.LookPath("tail")
	if err != nil {
		return fmt.Errorf("tail not found: %w", err)
	}

	tailArgs := []string{"tail", fmt.Sprintf("-n%d", consoleLines), "-f", v.SerialLog}
	return syscall.Exec(tailPath, tailArgs, os.Environ())
}
