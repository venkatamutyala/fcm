package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [name] -- [command...]",
	Short: "Execute a command in a VM via SSH",
	Long:  "Runs a command inside the VM over SSH. Use -- to separate the VM name from the command.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	if v.IP == "" {
		return fmt.Errorf("vm %q has no IP address", name)
	}

	// Everything after the first arg is the remote command.
	// Cobra strips the "--" but passes the rest in args.
	remoteArgs := args[1:]
	if len(remoteArgs) == 0 {
		return fmt.Errorf("no command specified (use: fcm exec %s -- <command>)", name)
	}

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
		strings.Join(remoteArgs, " "),
	}

	// Replace process with ssh so stdin/stdout/stderr pass through
	// and the remote exit code is returned.
	err = syscall.Exec(sshPath, sshArgs, os.Environ())
	if err != nil {
		return fmt.Errorf("exec ssh: %w", err)
	}
	return nil
}
