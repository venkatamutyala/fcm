package main

import "os"

// findSSHKey looks for common SSH private key files and returns the path if found.
func findSSHKey() string {
	home := os.Getenv("HOME")
	candidates := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/id_rsa",
		home + "/.ssh/id_ecdsa",
		"/root/.ssh/id_ed25519",
		"/root/.ssh/id_rsa",
		"/root/.ssh/id_ecdsa",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// sshBaseArgs returns the common SSH arguments for connecting to a VM.
func sshBaseArgs(ip string) []string {
	args := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
	}

	if key := findSSHKey(); key != "" {
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", key)
	}

	args = append(args, "root@"+ip)
	return args
}
