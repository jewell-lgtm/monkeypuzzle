package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "mp",
	Short:             "Monkeypuzzle - development workflow CLI",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return ensureUserConfig(cmd) },
}

func Execute() error {
	return rootCmd.Execute()
}
