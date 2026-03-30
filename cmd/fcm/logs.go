package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var logsFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs [name]",
	Short: "View VM systemd journal logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !vm.Exists(name) {
		return fmt.Errorf("vm %q not found", name)
	}

	unit := systemd.VMUnitName(name)

	journalctl, err := exec.LookPath("journalctl")
	if err != nil {
		return fmt.Errorf("journalctl not found: %w", err)
	}

	journalArgs := []string{"journalctl", "-u", unit, "--no-pager"}
	if logsFollow {
		journalArgs = append(journalArgs, "-f")
	}

	return syscall.Exec(journalctl, journalArgs, os.Environ())
}
