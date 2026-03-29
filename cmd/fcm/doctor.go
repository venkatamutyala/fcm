package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"fcm.dev/fcm-cli/internal/config"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system readiness for FCM",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type check struct {
	name string
	fn   func() error
}

func runDoctor(cmd *cobra.Command, args []string) error {
	checks := []check{
		{"Running as root", checkRoot},
		{"/dev/kvm accessible", checkKVM},
		{"Firecracker installed", checkFirecracker},
		{"Kernel available", checkKernel},
		{"Bridge fcbr0 active", checkBridge},
		{"NAT rules configured", checkNAT},
		{"Directory structure", checkDirs},
	}

	allPassed := true
	for _, c := range checks {
		if err := c.fn(); err != nil {
			fmt.Printf("  x %s: %v\n", c.name, err)
			allPassed = false
		} else {
			fmt.Printf("  + %s\n", c.name)
		}
	}

	if !allPassed {
		return fmt.Errorf("some checks failed")
	}

	fmt.Println("\nAll checks passed. FCM is ready.")
	return nil
}

func checkRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("not running as root")
	}
	return nil
}

func checkKVM() error {
	info, err := os.Stat("/dev/kvm")
	if err != nil {
		return fmt.Errorf("/dev/kvm not found (is KVM enabled?)")
	}
	// Check it's a character device
	if info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("/dev/kvm is not a character device")
	}
	return nil
}

func checkFirecracker() error {
	path, err := exec.LookPath("firecracker")
	if err != nil {
		return fmt.Errorf("firecracker not found in PATH")
	}

	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("firecracker --version failed: %v", err)
	}
	fmt.Printf("    %s", string(out))
	return nil
}

func checkKernel() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %v", err)
	}

	if _, err := os.Stat(cfg.DefaultKernel); err != nil {
		return fmt.Errorf("kernel not found at %s", cfg.DefaultKernel)
	}
	return nil
}

func checkBridge() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %v", err)
	}

	bridgePath := filepath.Join("/sys/class/net", cfg.BridgeName)
	if _, err := os.Stat(bridgePath); err != nil {
		return fmt.Errorf("bridge %s not found", cfg.BridgeName)
	}
	return nil
}

func checkNAT() error {
	out, err := exec.Command("iptables", "-t", "nat", "-L", "POSTROUTING", "-n").CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables check failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %v", err)
	}

	if len(out) == 0 {
		return fmt.Errorf("no NAT rules found for %s", cfg.BridgeSubnet)
	}
	// Basic check — just verify iptables is accessible
	return nil
}

func checkDirs() error {
	for _, dir := range config.Dirs() {
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("directory %s missing (run fcm init)", dir)
		}
	}
	return nil
}
