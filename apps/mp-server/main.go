// Command mp-server is the global piece registry, with private developer accounts.
// It runs in two modes:
//
//	mp-server serve    # registry HTTP API, dashboard and MCP
//	mp-server worker   # optional PR monitoring, PR_SYNC_ENABLED=true
//
// The registry needs Postgres and authentication. Temporal is optional.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{Use: "mp-server", Short: "Global piece registry for Monkeypuzzle"}
	root.AddCommand(
		&cobra.Command{
			Use:   "serve",
			Short: "Run the piece registry (HTTP API, dashboard and MCP)",
			RunE:  func(*cobra.Command, []string) error { return runServe() },
		},
		&cobra.Command{
			Use:   "worker",
			Short: "Run the optional PR-monitoring worker",
			RunE:  func(*cobra.Command, []string) error { return runWorker() },
		},
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
