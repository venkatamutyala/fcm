package main

import (
	"encoding/json"
	"fmt"
	"os"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [name]",
	Short: "Show detailed VM information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		v, err := vm.LoadVM(name)
		if err != nil {
			return err
		}

		type inspectOutput struct {
			*vm.VM
			Status string `json:"status"`
		}

		out := inspectOutput{
			VM:     v,
			Status: systemd.VMStatus(name),
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fmt.Errorf("encode output: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}
