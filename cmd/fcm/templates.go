package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"fcm.dev/fcm-cli/internal/templates"
	"github.com/spf13/cobra"
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available VM templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		tmplList := templates.List()

		if flagJSON {
			return templatesJSON(tmplList)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tIMAGE\tDESCRIPTION\tCPUS\tMEM\tDISK\n")
		for _, t := range tmplList {
			cpus := "-"
			if t.CPUs > 0 {
				cpus = fmt.Sprintf("%d", t.CPUs)
			}
			mem := "-"
			if t.Memory > 0 {
				mem = fmt.Sprintf("%dMB", t.Memory)
			}
			disk := "-"
			if t.Disk > 0 {
				disk = fmt.Sprintf("%dGB", t.Disk)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				t.Name, t.Image, t.Description, cpus, mem, disk)
		}
		w.Flush()
		return nil
	},
}

func templatesJSON(tmplList []templates.Template) error {
	type entry struct {
		Name        string `json:"name"`
		Image       string `json:"image"`
		Description string `json:"description"`
		CPUs        int    `json:"cpus,omitempty"`
		Memory      int    `json:"memory_mb,omitempty"`
		Disk        int    `json:"disk_gb,omitempty"`
	}

	var entries []entry
	for _, t := range tmplList {
		entries = append(entries, entry{
			Name:        t.Name,
			Image:       t.Image,
			Description: t.Description,
			CPUs:        t.CPUs,
			Memory:      t.Memory,
			Disk:        t.Disk,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func init() {
	rootCmd.AddCommand(templatesCmd)
}
