package main

import (
	"os"

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

func init() {
	// Registered for --help only; extractRemoteTarget strips these from argv
	// before cobra ever parses, so their cobra values are never read.
	rootCmd.PersistentFlags().String("host", "", "run this command on a remote ssh host where mp is installed (env: MP_HOST)")
	rootCmd.PersistentFlags().String("dir", "", "remote directory to run in, absolute path; requires --host (env: MP_DIR)")
}

func Execute() error {
	args, target, err := extractRemoteTarget(os.Args[1:])
	if err != nil {
		rootCmd.PrintErrln("Error:", err)
		return err
	}
	if target != nil {
		os.Exit(runRemote(target, args))
	}
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
