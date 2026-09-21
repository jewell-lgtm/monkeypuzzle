package main

import (
	"github.com/spf13/cobra"
)

var atomsRegistered bool

// registerAtomCommands installs the noun-oriented command surface after every
// package init function has registered the long-standing flat commands. The
// flat verbs remain available as workflow shortcuts; the noun form makes the
// underlying objects discoverable and gives scripts a regular vocabulary.
func registerAtomCommands() {
	if atomsRegistered {
		return
	}
	atomsRegistered = true

	pieceCmd := &cobra.Command{
		Use:     "piece",
		Aliases: []string{"pieces"},
		Short:   "Inspect and manage piece atoms",
		Long: `A piece binds one unit of work to a branch, worktree, and lineage record.
Optional terminal integrations may follow it. Flat workflow verbs such as 'mp create' and
'mp done' remain available; this namespace exposes the same operations by noun.`,
		Args: cobra.NoArgs,
		RunE: runPieceStatus,
	}
	pieceCmd.Flags().StringVar(&flagStatusPiece, "piece", "", "Piece to inspect (default: the piece you're in)")
	pieceCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch name")
	pieceCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceCmd.Flags().BoolVar(&flagPieceStatusJSON, "json", false, "Output JSON even on a terminal")
	_ = pieceCmd.RegisterFlagCompletionFunc("piece", completePieceNames)
	_ = pieceCmd.RegisterFlagCompletionFunc("main", completeGitBranches)
	pieceCmd.AddCommand(
		atomCommand(pieceStatusCmd, "show [piece]", []string{"status"}),
		atomCommand(pieceListCmd, "list", []string{"ls"}),
		atomCommand(pieceCreateCmd, "create", nil),
		atomCommand(pieceAdoptCmd, "adopt [branch]", nil),
		atomCommand(pieceSyncCmd, "sync", nil),
		atomCommand(pieceUpdateCmd, "update", nil),
		atomCommand(pieceMergeCmd, "merge", nil),
		atomCommand(pieceDoneCmd, "done [piece]", nil),
		atomCommand(pieceAbandonCmd, "abandon [piece]", nil),
	)

	// Stack was already a noun command. Complete its vocabulary and plural
	// aliases without changing its established default behaviour.
	stackCmd.Aliases = appendAlias(stackCmd.Aliases, "stacks")
	stackStatusCmd.Aliases = appendAlias(stackStatusCmd.Aliases, "show", "list", "ls")
	stackCmd.Args = cobra.NoArgs
	stackCmd.RunE = runStackStatus
	stackCmd.Flags().StringVar(&flagStackMain, "main", "main", "Main branch name")
	stackCmd.Flags().BoolVar(&flagStackFromRemote, "from-remote", false, "Rebuild local lineage from open PR/MR bases")
	stackCmd.Flags().BoolVar(&flagStackApplyBases, "apply-bases", false, "Edit PR/MR bases on the forge to match local lineage")
	stackCmd.Flags().BoolVar(&flagStackStatusJSON, "json", false, "Output JSON even on a terminal")
	prCmd.Aliases = appendAlias(prCmd.Aliases, "prs")
	agentCmd.Aliases = appendAlias(agentCmd.Aliases, "agents")
	agentReadCmd.Aliases = appendAlias(agentReadCmd.Aliases, "show")
	historyCmd.Aliases = appendAlias(historyCmd.Aliases, "events")
	projectAddCmd.Aliases = appendAlias(projectAddCmd.Aliases, "create", "new")
	projectRemoveCmd.Aliases = appendAlias(projectRemoveCmd.Aliases, "delete")
	projectListCmd.Aliases = appendAlias(projectListCmd.Aliases, "show")
	projectCmd.Args = cobra.NoArgs
	projectCmd.RunE = runProjectList
	projectCmd.Flags().BoolVar(&flagProjectListJSON, "json", false, "Output JSON instead of a table")
	projectCmd.Flags().BoolVar(&flagProjectListAll, "all", false, "Include hidden rows (box-side clones of placed pieces)")
	agentCmd.Args = cobra.NoArgs
	agentCmd.RunE = runAgentList
	agentCmd.Flags().BoolVar(&flagAgentListJSON, "json", false, "Output JSON instead of the table")
	agentCmd.Flags().BoolVar(&flagAgentListAll, "all", false, "Span all registered projects (implied outside a git repo)")

	rootCmd.AddCommand(pieceCmd)
}

// atomCommand makes a leaf command available below a noun without moving the
// original command away from the root. It reuses the operation and its bound
// flags but intentionally does not copy Cobra's private execution/parent state.
func atomCommand(original *cobra.Command, use string, aliases []string) *cobra.Command {
	clone := &cobra.Command{
		Use: use, Aliases: aliases, Short: original.Short, Long: original.Long,
		Example: original.Example, Args: original.Args, RunE: original.RunE,
		ValidArgsFunction: original.ValidArgsFunction,
	}
	clone.Flags().AddFlagSet(original.Flags())
	return clone
}

func appendAlias(existing []string, aliases ...string) []string {
	seen := make(map[string]bool, len(existing)+len(aliases))
	for _, alias := range existing {
		seen[alias] = true
	}
	for _, alias := range aliases {
		if !seen[alias] {
			existing = append(existing, alias)
			seen[alias] = true
		}
	}
	return existing
}
