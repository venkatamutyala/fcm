package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh [name]",
	Short: "SSH into a VM via its bridge IP",
	Args:  cobra.ExactArgs(1),
	RunE:  runSSH,
}

func init() {
	rootCmd.AddCommand(sshCmd)
}

func runSSH(cmd *cobra.Command, args []string) error {
	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	if v.IP == "" {
		return fmt.Errorf("vm %q has no IP address", name)
	}

	// Exec into ssh, replacing the current process
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	sshArgs := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		fmt.Sprintf("root@%s", v.IP),
	}

	// Replace process with ssh
	return syscall.Exec(sshPath, sshArgs, os.Environ())
}
