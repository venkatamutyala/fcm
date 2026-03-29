package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		vms, err := vm.LoadAllVMs()
		if err != nil {
			return err
		}

		if len(vms) == 0 {
			fmt.Println("No VMs found. Create one with: fcm create <name> --image <image>")
			return nil
		}

		if flagJSON {
			return listJSON(vms)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tSTATUS\tIP\tCPUS\tMEMORY\tIMAGE\n")
		for _, v := range vms {
			status := systemd.VMStatus(v.Name)
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d MB\t%s\n",
				v.Name, status, v.IP, v.CPUs, v.MemoryMB, v.Image)
		}
		w.Flush()
		return nil
	},
}

func listJSON(vms []*vm.VM) error {
	type vmEntry struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		IP       string `json:"ip"`
		CPUs     int    `json:"cpus"`
		MemoryMB int    `json:"memory_mb"`
		Image    string `json:"image"`
	}

	var entries []vmEntry
	for _, v := range vms {
		entries = append(entries, vmEntry{
			Name:     v.Name,
			Status:   systemd.VMStatus(v.Name),
			IP:       v.IP,
			CPUs:     v.CPUs,
			MemoryMB: v.MemoryMB,
			Image:    v.Image,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func init() {
	rootCmd.AddCommand(listCmd)
}
