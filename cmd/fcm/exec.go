package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

	remoteArgs := args[1:]
	if len(remoteArgs) == 0 {
		return fmt.Errorf("no command specified (use: fcm exec %s -- <command>)", name)
	}

	remoteCmd := strings.Join(remoteArgs, " ")

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	sshArgs := sshBaseArgs(v.IP)
	sshArgs = append(sshArgs, remoteCmd)

	c := exec.Command(sshPath, sshArgs[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
