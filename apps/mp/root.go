package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mp",
	Short: "Monkeypuzzle - development workflow CLI",
	// Domain errors returned by RunE are not usage errors, so don't dump the full
	// usage text after them -- just print a clean "Error: ...". SilenceErrors is
	// left off so cobra still prints the error itself.
	SilenceUsage:      true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return ensureUserConfig(cmd) },
}

func Execute() error {
	return rootCmd.Execute()
}
