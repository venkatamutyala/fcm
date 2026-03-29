package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var cpCmd = &cobra.Command{
	Use:   "cp [src] [dst]",
	Short: "Copy files between host and VM",
	Long: `Copy files between the host and a VM using scp.

  Download: fcm cp myvm:/remote/path ./local/path
  Upload:   fcm cp ./local/path myvm:/remote/path`,
	Args: cobra.ExactArgs(2),
	RunE: runCp,
}

func init() {
	rootCmd.AddCommand(cpCmd)
}

// parseVMPath checks if a path contains "vmname:/path" and returns
// (vmName, remotePath, true) or ("", originalPath, false).
func parseVMPath(arg string) (string, string, bool) {
	// Look for the first colon; if the part before it looks like a VM name
	// (no slashes), treat it as vmname:path.
	idx := strings.Index(arg, ":")
	if idx <= 0 {
		return "", arg, false
	}
	prefix := arg[:idx]
	// If the prefix contains a path separator it is a local path, not a VM name.
	if strings.ContainsAny(prefix, `/\`) {
		return "", arg, false
	}
	return prefix, arg[idx+1:], true
}

func runCp(cmd *cobra.Command, args []string) error {
	srcVM, srcPath, srcIsRemote := parseVMPath(args[0])
	dstVM, dstPath, dstIsRemote := parseVMPath(args[1])

	if srcIsRemote && dstIsRemote {
		return fmt.Errorf("cannot copy between two VMs directly; copy to local first")
	}
	if !srcIsRemote && !dstIsRemote {
		return fmt.Errorf("one of src or dst must be a VM path (vmname:/path)")
	}

	var vmName, remotePath, localPath string
	var upload bool

	if srcIsRemote {
		vmName = srcVM
		remotePath = srcPath
		localPath = dstPath
		upload = false
	} else {
		vmName = dstVM
		remotePath = dstPath
		localPath = srcPath
		upload = true
	}

	v, err := vm.LoadVM(vmName)
	if err != nil {
		return err
	}
	if v.IP == "" {
		return fmt.Errorf("vm %q has no IP address", vmName)
	}

	scpPath, err := exec.LookPath("scp")
	if err != nil {
		return fmt.Errorf("scp not found: %w", err)
	}

	scpArgs := scpBaseArgs(v.IP, remotePath, localPath, upload)

	scpCmd := exec.Command(scpPath, scpArgs[1:]...)
	scpCmd.Stdin = os.Stdin
	scpCmd.Stdout = os.Stdout
	scpCmd.Stderr = os.Stderr
	return scpCmd.Run()
}

// scpBaseArgs builds the scp command arguments, reusing the same SSH options
// as sshBaseArgs.
func scpBaseArgs(ip, remotePath, localPath string, upload bool) []string {
	args := []string{
		"scp",
		"-r",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
	}

	if key := findSSHKey(); key != "" {
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", key)
	}

	remote := fmt.Sprintf("root@%s:%s", ip, remotePath)

	if upload {
		args = append(args, localPath, remote)
	} else {
		args = append(args, remote, localPath)
	}

	return args
}
