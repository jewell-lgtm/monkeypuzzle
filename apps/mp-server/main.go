// Command mp-server is the Monkey Puzzle web + MCP server. It runs in two modes:
//
//	mp-server serve    # HTTP: HTML UI (humans) + MCP endpoint (agents)
//	mp-server worker   # Temporal worker syncing GitHub -> Postgres
//
// Both read configuration from the environment (see config.go). docker-compose
// brings up Postgres + a Temporal dev server + both modes.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{Use: "mp-server", Short: "Monkey Puzzle web + MCP server"}
	root.AddCommand(
		&cobra.Command{
			Use:   "serve",
			Short: "Run the HTTP server (HTML UI + MCP endpoint)",
			RunE:  func(*cobra.Command, []string) error { return runServe() },
		},
		&cobra.Command{
			Use:   "worker",
			Short: "Run the Temporal sync worker",
			RunE:  func(*cobra.Command, []string) error { return runWorker() },
		},
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
