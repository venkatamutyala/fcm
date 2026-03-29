package network

import (
	"fmt"
	"net"
	"strings"

	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/vm"
)

// AllocateIP finds the next available IP in the bridge subnet.
// Must be called within vm.WithLock() to prevent races.
func AllocateIP(cfg *config.Config) (string, error) {
	used, err := usedIPs()
	if err != nil {
		return "", fmt.Errorf("scan used ips: %w", err)
	}

	baseIP, _, err := net.ParseCIDR(cfg.BridgeSubnet)
	if err != nil {
		return "", fmt.Errorf("parse subnet: %w", err)
	}

	base := baseIP.To4()
	if base == nil {
		return "", fmt.Errorf("subnet %s is not IPv4", cfg.BridgeSubnet)
	}

	// Scan from ip_range_start to 254
	for i := cfg.IPRangeStart; i <= 254; i++ {
		candidate := fmt.Sprintf("%d.%d.%d.%d", base[0], base[1], base[2], i)
		if !used[candidate] {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no available IPs in subnet %s (all %d-%d used)",
		cfg.BridgeSubnet, cfg.IPRangeStart, 254)
}

// MACFromIP generates a deterministic MAC address from an IP.
// Format: AA:FC:00:00:XX:YY where XX:YY derive from the last two octets as hex.
func MACFromIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "AA:FC:00:00:00:01"
	}
	var oct3, oct4 int
	_, _ = fmt.Sscanf(parts[2], "%d", &oct3)
	_, _ = fmt.Sscanf(parts[3], "%d", &oct4)
	return fmt.Sprintf("AA:FC:00:00:%02X:%02X", oct3, oct4)
}

// BootArgs generates the kernel boot arguments.
// Networking is handled by DHCP (embedded in fcm) + cloud-init, not kernel args.
// net.ifnames=0 ensures the guest sees "eth0" regardless of distro.
func BootArgs() string {
	return "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0"
}

func usedIPs() (map[string]bool, error) {
	vms, err := vm.LoadAllVMs()
	if err != nil {
		return nil, err
	}

	used := make(map[string]bool)
	for _, v := range vms {
		if v.IP != "" {
			used[v.IP] = true
		}
	}
	return used, nil
}
