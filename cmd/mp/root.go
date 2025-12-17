package mp

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mp",
	Short: "Monkeypuzzle - development workflow CLI",
}

func Execute() error {
	return rootCmd.Execute()
}
