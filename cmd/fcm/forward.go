package main

import (
	"fmt"
	"os/exec"
	"strings"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var forwardFlags struct {
	list   bool
	remove string
}

var forwardCmd = &cobra.Command{
	Use:   "forward [name] [hostPort:vmPort]",
	Short: "Manage port forwarding for a VM",
	Long: `Add, list, or remove port forwards for a VM.

Examples:
  fcm forward myvm 8080:80        # Forward host:8080 to vm:80
  fcm forward myvm --list         # List active forwards
  fcm forward myvm --remove 8080  # Remove forward on host port 8080`,
	Args: cobra.MinimumNArgs(1),
	RunE: runForward,
}

func init() {
	forwardCmd.Flags().BoolVar(&forwardFlags.list, "list", false, "List active port forwards")
	forwardCmd.Flags().StringVar(&forwardFlags.remove, "remove", "", "Remove forward for the given host port")
	rootCmd.AddCommand(forwardCmd)
}

func runForward(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	if forwardFlags.list {
		return listForwards(v)
	}

	if forwardFlags.remove != "" {
		return removeForward(v, forwardFlags.remove)
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: fcm forward <vm> <hostPort:vmPort>")
	}

	return addForward(v, args[1])
}

func listForwards(v *vm.VM) error {
	if len(v.Forwards) == 0 {
		fmt.Printf("No port forwards for %s\n", v.Name)
		return nil
	}
	fmt.Printf("Port forwards for %s:\n", v.Name)
	for hostPort, vmPort := range v.Forwards {
		fmt.Printf("  host:%s -> vm:%s\n", hostPort, vmPort)
	}
	return nil
}

func addForward(v *vm.VM, mapping string) error {
	parts := strings.SplitN(mapping, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format %q, expected hostPort:vmPort", mapping)
	}
	hostPort := parts[0]
	vmPort := parts[1]

	// Add iptables DNAT rules
	if err := addForwardRules(hostPort, v.IP, vmPort); err != nil {
		return fmt.Errorf("add iptables rules: %w", err)
	}

	// Store in vm.json
	if v.Forwards == nil {
		v.Forwards = make(map[string]string)
	}
	v.Forwards[hostPort] = vmPort
	if err := vm.SaveVM(v); err != nil {
		return fmt.Errorf("save vm state: %w", err)
	}

	fmt.Printf("Forward added: host:%s -> %s:%s\n", hostPort, v.IP, vmPort)
	return nil
}

func removeForward(v *vm.VM, hostPort string) error {
	vmPort, ok := v.Forwards[hostPort]
	if !ok {
		return fmt.Errorf("no forward found for host port %s", hostPort)
	}

	// Remove iptables DNAT rules
	removeForwardRulesForPort(hostPort, v.IP, vmPort)

	delete(v.Forwards, hostPort)
	if err := vm.SaveVM(v); err != nil {
		return fmt.Errorf("save vm state: %w", err)
	}

	fmt.Printf("Forward removed: host:%s\n", hostPort)
	return nil
}

func addForwardRules(hostPort, vmIP, vmPort string) error {
	// PREROUTING rule for external access
	if err := iptablesRun("-t", "nat", "-A", "PREROUTING",
		"-p", "tcp", "--dport", hostPort,
		"-j", "DNAT", "--to", vmIP+":"+vmPort); err != nil {
		return err
	}

	// OUTPUT rule for localhost access
	if err := iptablesRun("-t", "nat", "-A", "OUTPUT",
		"-p", "tcp", "--dport", hostPort,
		"-j", "DNAT", "--to", vmIP+":"+vmPort); err != nil {
		return err
	}

	return nil
}

func removeForwardRulesForPort(hostPort, vmIP, vmPort string) {
	_ = iptablesRun("-t", "nat", "-D", "PREROUTING",
		"-p", "tcp", "--dport", hostPort,
		"-j", "DNAT", "--to", vmIP+":"+vmPort)

	_ = iptablesRun("-t", "nat", "-D", "OUTPUT",
		"-p", "tcp", "--dport", hostPort,
		"-j", "DNAT", "--to", vmIP+":"+vmPort)
}

// CleanupAllForwards removes all iptables forward rules for a VM.
func cleanupAllForwards(v *vm.VM) {
	for hostPort, vmPort := range v.Forwards {
		removeForwardRulesForPort(hostPort, v.IP, vmPort)
	}
}

func iptablesRun(args ...string) error {
	cmd := exec.Command("iptables", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
