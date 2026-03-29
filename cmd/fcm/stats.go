package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats [name]",
	Short: "Show resource usage statistics for a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

// VMStats holds parsed VM statistics for JSON output.
type VMStats struct {
	Name    string      `json:"name"`
	IP      string      `json:"ip"`
	Uptime  UptimeInfo  `json:"uptime"`
	Memory  MemoryInfo  `json:"memory"`
	Disk    DiskInfo    `json:"disk"`
	Network NetworkInfo `json:"network"`
}

// UptimeInfo holds parsed uptime information.
type UptimeInfo struct {
	Raw      string  `json:"raw"`
	Uptime   string  `json:"uptime"`
	LoadAvg  string  `json:"load_avg"`
	Load1    float64 `json:"load_1"`
	Load5    float64 `json:"load_5"`
	Load15   float64 `json:"load_15"`
}

// MemoryInfo holds parsed memory information from free -m.
type MemoryInfo struct {
	Raw       string `json:"raw"`
	TotalMB   int    `json:"total_mb"`
	UsedMB    int    `json:"used_mb"`
	FreeMB    int    `json:"free_mb"`
	AvailMB   int    `json:"available_mb"`
}

// DiskInfo holds parsed disk information from df -h /.
type DiskInfo struct {
	Raw        string `json:"raw"`
	Size       string `json:"size"`
	Used       string `json:"used"`
	Available  string `json:"available"`
	UsePercent string `json:"use_percent"`
}

// NetworkInfo holds parsed network statistics from ip -s link show eth0.
type NetworkInfo struct {
	Raw      string `json:"raw"`
	RXBytes  int64  `json:"rx_bytes"`
	TXBytes  int64  `json:"tx_bytes"`
	RXPackets int64 `json:"rx_packets"`
	TXPackets int64 `json:"tx_packets"`
}

func runStats(cmd *cobra.Command, args []string) error {
	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}
	if v.IP == "" {
		return fmt.Errorf("vm %q has no IP address", name)
	}

	// Build the SSH command to gather stats
	remoteCmd := "uptime && echo '---SEPARATOR---' && free -m && echo '---SEPARATOR---' && df -h / && echo '---SEPARATOR---' && ip -s link show eth0"

	sshArgs := sshBaseArgs(v.IP)
	sshArgs = append(sshArgs, remoteCmd)

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	out, err := exec.Command(sshPath, sshArgs[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get stats from %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}

	output := string(out)
	sections := strings.Split(output, "---SEPARATOR---")

	if !flagJSON {
		// Pretty output
		fmt.Printf("=== Stats for %s (%s) ===\n\n", name, v.IP)
		if len(sections) > 0 {
			fmt.Printf("--- Uptime ---\n%s\n", strings.TrimSpace(sections[0]))
		}
		if len(sections) > 1 {
			fmt.Printf("--- Memory ---\n%s\n", strings.TrimSpace(sections[1]))
		}
		if len(sections) > 2 {
			fmt.Printf("--- Disk ---\n%s\n", strings.TrimSpace(sections[2]))
		}
		if len(sections) > 3 {
			fmt.Printf("--- Network ---\n%s\n", strings.TrimSpace(sections[3]))
		}
		return nil
	}

	// JSON output: parse sections
	stats := VMStats{
		Name: name,
		IP:   v.IP,
	}

	if len(sections) > 0 {
		stats.Uptime = parseUptime(strings.TrimSpace(sections[0]))
	}
	if len(sections) > 1 {
		stats.Memory = parseMemory(strings.TrimSpace(sections[1]))
	}
	if len(sections) > 2 {
		stats.Disk = parseDisk(strings.TrimSpace(sections[2]))
	}
	if len(sections) > 3 {
		stats.Network = parseNetwork(strings.TrimSpace(sections[3]))
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func parseUptime(raw string) UptimeInfo {
	info := UptimeInfo{Raw: raw}

	// Extract uptime portion (between "up" and "user" or "load")
	if idx := strings.Index(raw, "up "); idx >= 0 {
		rest := raw[idx+3:]
		// Find "load average:" to extract load
		if laIdx := strings.Index(rest, "load average:"); laIdx >= 0 {
			info.Uptime = strings.TrimSpace(strings.TrimRight(rest[:laIdx], ", \t"))
			// Remove parts after first comma that contains "user"
			if uIdx := strings.Index(info.Uptime, " user"); uIdx >= 0 {
				// Walk back to the comma before "N user"
				commaIdx := strings.LastIndex(info.Uptime[:uIdx], ",")
				if commaIdx >= 0 {
					info.Uptime = strings.TrimSpace(info.Uptime[:commaIdx])
				}
			}

			loadStr := strings.TrimSpace(rest[laIdx+len("load average:"):])
			info.LoadAvg = loadStr
			fmt.Sscanf(loadStr, "%f, %f, %f", &info.Load1, &info.Load5, &info.Load15)
		}
	}

	return info
}

func parseMemory(raw string) MemoryInfo {
	info := MemoryInfo{Raw: raw}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Mem:") {
			fields := strings.Fields(line)
			if len(fields) >= 7 {
				fmt.Sscanf(fields[1], "%d", &info.TotalMB)
				fmt.Sscanf(fields[2], "%d", &info.UsedMB)
				fmt.Sscanf(fields[3], "%d", &info.FreeMB)
				if len(fields) >= 7 {
					fmt.Sscanf(fields[6], "%d", &info.AvailMB)
				}
			}
		}
	}
	return info
}

func parseDisk(raw string) DiskInfo {
	info := DiskInfo{Raw: raw}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		if strings.Contains(line, "/") && !strings.HasPrefix(line, "Filesystem") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				info.Size = fields[1]
				info.Used = fields[2]
				info.Available = fields[3]
				info.UsePercent = fields[4]
			}
			break
		}
	}
	return info
}

func parseNetwork(raw string) NetworkInfo {
	info := NetworkInfo{Raw: raw}
	lines := strings.Split(raw, "\n")
	// ip -s link output: after "RX:" line, next line has bytes/packets
	// after "TX:" line, next line has bytes/packets
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "RX:") && i+1 < len(lines) {
			fields := strings.Fields(strings.TrimSpace(lines[i+1]))
			if len(fields) >= 2 {
				fmt.Sscanf(fields[0], "%d", &info.RXBytes)
				fmt.Sscanf(fields[1], "%d", &info.RXPackets)
			}
		}
		if strings.HasPrefix(trimmed, "TX:") && i+1 < len(lines) {
			fields := strings.Fields(strings.TrimSpace(lines[i+1]))
			if len(fields) >= 2 {
				fmt.Sscanf(fields[0], "%d", &info.TXBytes)
				fmt.Sscanf(fields[1], "%d", &info.TXPackets)
			}
		}
	}
	return info
}
