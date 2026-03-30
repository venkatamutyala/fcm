package main

import (
	"fmt"
	"net/http"

	"fcm.dev/fcm-cli/api"
	"github.com/spf13/cobra"
)

var serveFlags struct {
	addr  string
	token string
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the FCM REST API server",
	Long:  "Runs an HTTP API server for managing VMs remotely. Requires root and a bearer token for authentication.",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveFlags.addr, "addr", ":8080", "Listen address (host:port)")
	serveCmd.Flags().StringVar(&serveFlags.token, "token", "", "Bearer token for API authentication (required)")
	_ = serveCmd.MarkFlagRequired("token")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	srv := api.NewServer(serveFlags.token)

	fmt.Printf("API server listening on %s\n", serveFlags.addr)
	return http.ListenAndServe(serveFlags.addr, srv)
}
