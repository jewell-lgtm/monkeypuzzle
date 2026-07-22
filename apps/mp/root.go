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
	// Registered for --help only; extractRemoteSpec strips these from argv
	// before cobra ever parses, so their cobra values are never read. They
	// only count between `mp` and the verb: `mp --host wire list`.
	rootCmd.PersistentFlags().String("host", "", "before the verb: run the command on a remote ssh host where mp is installed (env: MP_HOST)")
	rootCmd.PersistentFlags().String("dir", "", "before the verb: remote directory to run in (absolute, or relative to the ssh login home); requires a remote target (env: MP_DIR)")
	rootCmd.PersistentFlags().String("project", "", "before the verb: run the command against a registered project — proxied over ssh if it has a host, from its path if local")
}

func Execute() error {
	args, spec, err := extractRemoteSpec(os.Args[1:])
	if err != nil {
		rootCmd.PrintErrln("Error:", err)
		return err
	}
	target, chdir, err := resolveTarget(spec)
	if err != nil {
		rootCmd.PrintErrln("Error:", err)
		return err
	}
	if target != nil {
		os.Exit(runRemote(target, args))
	}
	if chdir != "" {
		if err := os.Chdir(chdir); err != nil {
			rootCmd.PrintErrln("Error:", err)
			return err
		}
	}
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
