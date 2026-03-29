package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var runFlags struct {
	image    string
	cpus     int
	memory   int
	disk     int
	sshKey   string
	cloudInit string
	template string
}

var runCmd = &cobra.Command{
	Use:   "run [name]",
	Short: "Create a VM, wait for SSH, then connect",
	Long:  "Create and start a new VM, wait for SSH readiness, then exec into an SSH session.",
	Args:  cobra.ExactArgs(1),
	RunE:  runRun,
}

func init() {
	runCmd.Flags().StringVar(&runFlags.image, "image", "", "Image name (e.g., ubuntu-22.04)")
	runCmd.Flags().IntVar(&runFlags.cpus, "cpus", 0, "Number of vCPUs (default from config)")
	runCmd.Flags().IntVar(&runFlags.memory, "memory", 0, "Memory in MB (default from config)")
	runCmd.Flags().IntVar(&runFlags.disk, "disk", 0, "Disk size in GB (default from config)")
	runCmd.Flags().StringVar(&runFlags.sshKey, "ssh-key", "", "Path to SSH public key")
	runCmd.Flags().StringVar(&runFlags.cloudInit, "cloud-init", "", "Path to cloud-init YAML file")
	runCmd.Flags().StringVar(&runFlags.template, "template", "", "Use a built-in template (see: fcm templates)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// Copy flags into createFlags so runCreate can use them
	createFlags.image = runFlags.image
	createFlags.cpus = runFlags.cpus
	createFlags.memory = runFlags.memory
	createFlags.disk = runFlags.disk
	createFlags.sshKey = runFlags.sshKey
	createFlags.cloudInit = runFlags.cloudInit
	createFlags.template = runFlags.template

	// Run the create flow (includes auto-detect SSH key and waitForSSH)
	if err := runCreate(cmd, args); err != nil {
		return err
	}

	// Load the VM to get the IP
	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	// Exec into SSH
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

	fmt.Printf("Connecting to %s...\n", v.IP)
	return syscall.Exec(sshPath, sshArgs, os.Environ())
}
