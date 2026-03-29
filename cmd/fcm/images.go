package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"fcm.dev/fcm-cli/internal/images"
	"github.com/spf13/cobra"
)

var imagesAvailable bool

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List available images",
	RunE: func(cmd *cobra.Command, args []string) error {
		if imagesAvailable {
			return listAvailableImages()
		}

		imgs, err := images.List()
		if err != nil {
			return err
		}

		if len(imgs) == 0 {
			fmt.Println("No images found. Pull one with: fcm pull ubuntu-22.04")
			fmt.Println("See all pullable images with: fcm images --available")
			return nil
		}

		if flagJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(imgs)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tSIZE\n")
		for _, img := range imgs {
			fmt.Fprintf(w, "%s\t%d MB\n", img.Name, img.Size/1024/1024)
		}
		w.Flush()
		return nil
	},
}

func listAvailableImages() error {
	families := images.ImageFamilies()

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(families)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tFAMILY\tFORMAT\n")
	for _, fam := range families {
		for _, img := range fam.Images {
			fmt.Fprintf(w, "%s\t%s\t%s\n", img.Name, fam.Family, img.Format)
		}
	}
	w.Flush()
	return nil
}

var pullCmd = &cobra.Command{
	Use:   "pull [image]",
	Short: "Download a VM image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		return images.Pull(args[0])
	},
}

var importCmd = &cobra.Command{
	Use:   "import [name] [path]",
	Short: "Import a local ext4 file as an image",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		if err := images.Import(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Image %s imported\n", args[0])
		return nil
	},
}

var rmiCmd = &cobra.Command{
	Use:   "rmi [name]",
	Short: "Remove a cached image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireRoot(); err != nil {
			return err
		}
		if err := images.Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("Image %s removed\n", args[0])
		return nil
	},
}

func init() {
	imagesCmd.Flags().BoolVar(&imagesAvailable, "available", false, "List all pullable images")
	rootCmd.AddCommand(imagesCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(rmiCmd)
}
