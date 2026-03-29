package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"fcm.dev/fcm-cli/internal/config"
)

// SetupBridge creates the FCM bridge, assigns its IP, and configures NAT.
// This is called by the `_setup-bridge` internal command.
func SetupBridge(cfg *config.Config) error {
	// Enable IP forwarding
	if err := sysctl("net.ipv4.ip_forward", "1"); err != nil {
		return fmt.Errorf("enable ip forwarding: %w", err)
	}

	// Create bridge if it doesn't exist
	if !linkExists(cfg.BridgeName) {
		if err := run("ip", "link", "add", "name", cfg.BridgeName, "type", "bridge"); err != nil {
			return fmt.Errorf("create bridge: %w", err)
		}
	}

	// Assign IP if not already set
	if !hasAddr(cfg.BridgeName, cfg.BridgeIP) {
		cidr := cfg.BridgeIP + "/" + maskToCIDR(cfg.BridgeMask)
		if err := run("ip", "addr", "add", cidr, "dev", cfg.BridgeName); err != nil {
			// Ignore "already exists" errors
			if !strings.Contains(err.Error(), "RTNETLINK answers: File exists") {
				return fmt.Errorf("assign bridge ip: %w", err)
			}
		}
	}

	// Bring up the bridge
	if err := run("ip", "link", "set", cfg.BridgeName, "up"); err != nil {
		return fmt.Errorf("bring up bridge: %w", err)
	}

	// Setup NAT (idempotent)
	if err := ensureNAT(cfg); err != nil {
		return fmt.Errorf("setup nat: %w", err)
	}

	return nil
}

// TeardownBridge removes NAT rules and deletes the bridge.
func TeardownBridge(cfg *config.Config) error {

	// Remove NAT rule (ignore errors if it doesn't exist)
	removeNAT(cfg)

	// Remove forwarding rules
	removeForwardRules(cfg)

	// Delete bridge
	if linkExists(cfg.BridgeName) {
		if err := run("ip", "link", "set", cfg.BridgeName, "down"); err != nil {
			return fmt.Errorf("bring down bridge: %w", err)
		}
		if err := run("ip", "link", "del", cfg.BridgeName); err != nil {
			return fmt.Errorf("delete bridge: %w", err)
		}
	}

	return nil
}

// BridgeExists returns true if the bridge interface is present.
func BridgeExists(name string) bool {
	return linkExists(name)
}

func ensureNAT(cfg *config.Config) error {
	// Check if NAT rule already exists
	err := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING",
		"-s", cfg.BridgeSubnet, "!", "-o", cfg.BridgeName,
		"-j", "MASQUERADE").Run()
	if err == nil {
		return nil // rule already exists
	}

	// Add NAT rule
	if err := run("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", cfg.BridgeSubnet, "!", "-o", cfg.BridgeName,
		"-j", "MASQUERADE"); err != nil {
		return err
	}

	// Add FORWARD rules for bridge traffic
	forwardRules := [][]string{
		{"-A", "FORWARD", "-i", cfg.BridgeName, "-o", cfg.BridgeName, "-j", "ACCEPT"},
		{"-A", "FORWARD", "-i", cfg.BridgeName, "!", "-o", cfg.BridgeName, "-j", "ACCEPT"},
		// Allow ALL traffic from host to VMs (not just RELATED,ESTABLISHED)
		// This enables SSH, HTTP, etc. from host to guest
		{"-A", "FORWARD", "!", "-i", cfg.BridgeName, "-o", cfg.BridgeName, "-j", "ACCEPT"},
	}

	for _, rule := range forwardRules {
		// Check first, add if missing
		checkArgs := append([]string{"-C"}, rule[1:]...)
		if exec.Command("iptables", checkArgs...).Run() != nil {
			if err := run("iptables", rule...); err != nil {
				return fmt.Errorf("add forward rule: %w", err)
			}
		}
	}

	return nil
}

func removeNAT(cfg *config.Config) {
	_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", cfg.BridgeSubnet, "!", "-o", cfg.BridgeName,
		"-j", "MASQUERADE").Run()
}

func removeForwardRules(cfg *config.Config) {
	rules := [][]string{
		{"-D", "FORWARD", "-i", cfg.BridgeName, "-o", cfg.BridgeName, "-j", "ACCEPT"},
		{"-D", "FORWARD", "-i", cfg.BridgeName, "!", "-o", cfg.BridgeName, "-j", "ACCEPT"},
		{"-D", "FORWARD", "!", "-i", cfg.BridgeName, "-o", cfg.BridgeName, "-j", "ACCEPT"},
	}
	for _, rule := range rules {
		_ = exec.Command("iptables", rule...).Run()
	}
}

func linkExists(name string) bool {
	return exec.Command("ip", "link", "show", name).Run() == nil
}

func hasAddr(dev, ip string) bool {
	out, err := exec.Command("ip", "addr", "show", dev).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), ip)
}

func sysctl(key, value string) error {
	return run("sysctl", "-w", key+"="+value)
}

func maskToCIDR(mask string) string {
	ip := net.ParseIP(mask)
	if ip == nil {
		return "24" // default
	}
	ones, _ := net.IPMask(ip.To4()).Size()
	return fmt.Sprintf("%d", ones)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s: %w", name, strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
