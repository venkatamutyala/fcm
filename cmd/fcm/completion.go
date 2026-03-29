package main

import (
	"fcm.dev/fcm-cli/internal/images"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
)

// completeVMNames returns a ValidArgsFunction that completes VM names.
func completeVMNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, err := vm.ListVMs()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeImageNames returns a ValidArgsFunction that completes available image names.
func completeImageNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return images.AvailableImages(), cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Register VM name completion on commands that take a VM name
	sshCmd.ValidArgsFunction = completeVMNames
	stopCmd.ValidArgsFunction = completeVMNames
	startCmd.ValidArgsFunction = completeVMNames
	restartCmd.ValidArgsFunction = completeVMNames
	deleteCmd.ValidArgsFunction = completeVMNames
	inspectCmd.ValidArgsFunction = completeVMNames
	logsCmd.ValidArgsFunction = completeVMNames
	backupCmd.ValidArgsFunction = completeVMNames
	restoreCmd.ValidArgsFunction = completeVMNames
	consoleCmd.ValidArgsFunction = completeVMNames
	execCmd.ValidArgsFunction = completeVMNames
	resizeCmd.ValidArgsFunction = completeVMNames

	// Register image name completion on --image flag for create
	createCmd.RegisterFlagCompletionFunc("image", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return images.AvailableImages(), cobra.ShellCompDirectiveNoFileComp
	})

	// Register image name completion for pull command
	pullCmd.ValidArgsFunction = completeImageNames
}
