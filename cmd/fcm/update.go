package main

import (
	"fcm.dev/fcm-cli/internal/update"
	"github.com/spf13/cobra"
)

var updateVersion string

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update fcm to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		return update.SelfUpdate(Version, updateVersion)
	},
}

func init() {
	selfUpdateCmd.Flags().StringVar(&updateVersion, "version", "", "Update to a specific version")
	rootCmd.AddCommand(selfUpdateCmd)
}
