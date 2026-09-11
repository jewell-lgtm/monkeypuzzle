package main

import (
	"os"
	"sync"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mp",
	Short: "Monkeypuzzle - development workflow CLI",
	Long: `Monkeypuzzle gives every change its own piece: a branch, worktree, and
optional multiplexer session.

The default flow is deliberately small:
  mp create -> mp sync -> mp pr create -> mp pr ready -> mp merge -> mp done

Use mp abandon for work you have decided not to finish. Use mp settle only to
remove a published piece from your private registry; it never changes Git or
the piece's workflow state. The flow and its safety defaults are opinionated;
the forge provider, multiplexer, opener, and policy gates stay configurable.`,
	// Domain errors returned by RunE are not usage errors, so don't dump the full
	// usage text after them -- just print a clean "Error: ...". SilenceErrors is
	// left off so cobra still prints the error itself.
	SilenceUsage:      true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return ensureUserConfig(cmd) },
}

var rootHelpOnce sync.Once

// organizeRootHelp makes the top-level command list teach the workflow instead
// of presenting one alphabetical wall. This runs after package init, when every
// command has registered itself.
func organizeRootHelp() {
	rootHelpOnce.Do(func() {
		rootCmd.AddGroup(
			&cobra.Group{ID: "workflow", Title: "Piece workflow:"},
			&cobra.Group{ID: "navigate", Title: "Find and move:"},
			&cobra.Group{ID: "collaborate", Title: "Collaboration:"},
			&cobra.Group{ID: "setup", Title: "Setup and maintenance:"},
		)
		groups := map[string]string{
			"create": "workflow", "adopt": "workflow", "sync": "workflow",
			"pr": "workflow", "merge": "workflow", "done": "workflow",
			"abandon": "workflow", "stack": "workflow", "update": "workflow",
			"switch": "navigate", "open": "navigate", "go": "navigate",
			"list": "navigate", "status": "navigate", "inbox": "navigate",
			"history": "navigate",
			"agent":   "collaborate", "wait": "collaborate", "tracking": "collaborate",
			"settle": "collaborate",
			"init":   "setup", "reinit": "setup", "config": "setup",
			"doctor": "setup", "integration": "setup", "project": "setup",
			"remote": "setup", "shell-init": "setup", "cleanup": "setup",
			"flatten": "setup", "move": "setup",
			"completion": "setup", "claude": "setup",
		}
		for _, cmd := range rootCmd.Commands() {
			if group := groups[cmd.Name()]; group != "" {
				cmd.GroupID = group
			}
		}
	})
}

func init() {
	// Registered for --help only; extractRemoteSpec strips these from argv
	// before cobra ever parses, so their cobra values are never read. They
	// only count between `mp` and the verb: `mp --host wire list`.
	rootCmd.PersistentFlags().String("host", "", "before the verb: run the command on a remote ssh host where mp is installed (env: MP_HOST)")
	rootCmd.PersistentFlags().String("dir", "", "before the verb: remote directory to run in (absolute, or relative to the ssh login home); requires a remote target (env: MP_DIR)")
	rootCmd.PersistentFlags().String("project", "", "before the verb: run the command against a registered project — proxied over ssh if it has a host, from its path if local")
}

// invocationArgs is argv after extractRemoteSpec took the leading
// --host/--dir/--project (and resolveTarget acted on them): what a verb that
// re-forwards the invocation to a box (proxyPlaced) must send, never
// os.Args, or the box would re-apply a --project against its own registry.
var invocationArgs []string

func Execute() error {
	organizeRootHelp()
	args, spec, err := extractRemoteSpec(os.Args[1:])
	if err != nil {
		rootCmd.PrintErrln("Error:", err)
		return err
	}
	invocationArgs = args
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
