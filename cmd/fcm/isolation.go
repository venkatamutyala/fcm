package main

import (
	"fmt"
)

// applyIsolationRules adds iptables rules that prevent a VM from
// communicating with other VMs on the bridge while still allowing
// traffic to the gateway (bridge IP) and the internet via NAT.
func applyIsolationRules(vmIP, bridgeIP string) error {
	// Allow VM -> bridge IP (gateway)
	if err := iptablesRun("-A", "FORWARD",
		"-s", vmIP, "-d", bridgeIP,
		"-j", "ACCEPT"); err != nil {
		return fmt.Errorf("allow gateway rule: %w", err)
	}

	// Block VM -> any other VM on the bridge subnet (192.168.100.0/24 except .1)
	if err := iptablesRun("-A", "FORWARD",
		"-s", vmIP, "-d", "192.168.100.0/24",
		"-j", "DROP"); err != nil {
		return fmt.Errorf("block inter-vm rule: %w", err)
	}

	return nil
}

// removeIsolationRules removes the iptables isolation rules for a VM.
func removeIsolationRules(vmIP string) {
	// Remove in reverse order; ignore errors since rules may not exist.
	_ = iptablesRun("-D", "FORWARD",
		"-s", vmIP, "-d", "192.168.100.0/24",
		"-j", "DROP")

	_ = iptablesRun("-D", "FORWARD",
		"-s", vmIP, "-d", "192.168.100.1",
		"-j", "ACCEPT")
}
