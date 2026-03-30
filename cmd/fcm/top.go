package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Live dashboard of all VMs",
	Long:  "Shows CPU, memory, and network usage for all running VMs. Refreshes every 2 seconds. Press Ctrl+C to stop.",
	RunE:  runTop,
}

func init() {
	rootCmd.AddCommand(topCmd)
}

func runTop(cmd *cobra.Command, args []string) error {
	for {
		// Clear screen
		fmt.Print("\033[H\033[2J")

		vms, err := vm.LoadAllVMs()
		if err != nil {
			return err
		}

		fmt.Printf("fcm top — %s\n\n", time.Now().Format("15:04:05"))

		if len(vms) == 0 {
			fmt.Println("No VMs found.")
			time.Sleep(2 * time.Second)
			continue
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tSTATUS\tCPU%%\tMEM USED\tMEM TOTAL\tNET RX\tNET TX\n")

		for _, v := range vms {
			status := systemd.VMStatus(v.Name)
			if status != "running" {
				fmt.Fprintf(w, "%s\t%s\t-\t-\t-\t-\t-\n", v.Name, status)
				continue
			}

			cpu, memUsed, memTotal, rx, tx := getVMStats(v.IP)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				v.Name, status, cpu, memUsed, memTotal, rx, tx)
		}
		w.Flush()

		fmt.Println("\nPress Ctrl+C to stop")
		time.Sleep(2 * time.Second)
	}
}

func getVMStats(ip string) (cpu, memUsed, memTotal, rx, tx string) {
	key := findSSHKey()
	sshArgs := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=2",
		"-o", "BatchMode=yes",
		"-o", "LogLevel=ERROR",
	}
	if key != "" {
		sshArgs = append(sshArgs, "-i", key, "-o", "IdentitiesOnly=yes")
	}
	sshArgs = append(sshArgs, fmt.Sprintf("root@%s", ip))

	// Get stats in one SSH call
	cmd := exec.Command(sshArgs[0], append(sshArgs[1:],
		"cat /proc/loadavg; free -m | grep Mem; cat /proc/net/dev | grep eth0")...)
	out, err := cmd.Output()
	if err != nil {
		return "?", "?", "?", "?", "?"
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	// Parse loadavg (line 0)
	if len(lines) > 0 {
		fields := strings.Fields(lines[0])
		if len(fields) > 0 {
			cpu = fields[0]
		}
	}

	// Parse memory (line 1: Mem: total used free ...)
	if len(lines) > 1 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 3 {
			memTotal = fields[1] + "MB"
			memUsed = fields[2] + "MB"
		}
	}

	// Parse network (line 2: eth0: rx_bytes ...)
	if len(lines) > 2 {
		fields := strings.Fields(lines[2])
		if len(fields) >= 10 {
			rx = formatBytes(fields[1])
			tx = formatBytes(fields[9])
		}
	}

	return
}

func formatBytes(s string) string {
	var v int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int64(c-'0')
		}
	}
	switch {
	case v >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(v)/float64(1<<30))
	case v >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(v)/float64(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(v)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", v)
	}
}
