package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	agentcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/promptinput"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var pieceStatusCmd = &cobra.Command{
	Use:   "status [piece]",
	Short: "Show a piece's status",
	Long: `Print a piece's status (parent, children, branch) as JSON. Defaults to the
piece you're standing in (or the main repo); name a piece positionally or with
--piece to inspect one from anywhere in the repo.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPieceStatus,
}

var pieceCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new puzzle piece",
	Long: `Create a new puzzle piece by initializing a git worktree and opening a tmux session.
The worktree will be created in a repo-scoped directory within the platform-appropriate data directory (e.g., ~/Library/Application Support/monkeypuzzle/pieces/{repo-hash}/ on macOS, ~/.local/share/monkeypuzzle/pieces/{repo-hash}/ on Linux).`,
	Args: cobra.NoArgs,
	RunE: runPieceCreate,
}

var pieceUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update piece with latest from main branch",
	Long:  `Merges the main branch into the current piece's history. Must be run from within a piece worktree.`,
	Args:  cobra.NoArgs,
	RunE:  runPieceUpdate,
}

var pieceMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge piece back into main branch",
	Long:  `Merges the piece branch back into main. Fails if main has commits not in the piece worktree. Must be run from within a piece worktree.`,
	Args:  cobra.NoArgs,
	RunE:  runPieceMerge,
}

var pieceCleanupCmd = &cobra.Command{
	Use:     "cleanup",
	Aliases: []string{"repair"},
	Short:   "Cleanup merged pieces and prune deleted projects",
	Long: `Repairs local state. Finds and removes pieces whose branches have been
merged (removing worktrees and killing tmux sessions), and prunes registry
entries for projects whose repository directory no longer exists on disk.

Dry-run by default: it reports what would be removed and changes nothing. Pass
--apply to actually clean up. In an interactive terminal you are shown the
preview and asked to confirm; non-interactive callers (flags/stdin JSON) preview
unless --apply (or "apply": true) is given.`,
	Args: cobra.NoArgs,
	RunE: runPieceCleanup,
}

var pieceAbandonCmd = &cobra.Command{
	Use:   "abandon [piece]",
	Short: "Abandon an unmerged piece",
	Long: `Remove a piece worktree, kill its tmux session, and optionally delete the branch.
Use --force to discard uncommitted changes.
Use --delete-branch to also remove the git branch.
Defaults to the piece you're standing in; name a piece positionally or with
--piece to abandon one from anywhere in the repo.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPieceAbandon,
}

var pieceDoneCmd = &cobra.Command{
	Use:   "done [piece]",
	Short: "Cleanup a piece after merge",
	Long: `Remove a piece worktree and tmux session after the branch has been merged.
Defaults to the piece you're standing in; name a piece positionally or with
--piece to finish one from anywhere in the repo. Verifies the piece is merged
before cleanup. Use 'mp abandon' for unmerged pieces.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPieceDone,
}

var pieceAdoptCmd = &cobra.Command{
	Use:   "adopt [branch]",
	Short: "Convert existing branch to piece",
	Long: `Convert a git branch into a piece worktree.
Accepts a local branch name, or a remote ref like "origin/foo" — remote refs are
fetched and a new local branch is created tracking the remote. When run from the
main repo with no branch specified, the current branch is used. From inside a
piece worktree, --branch is required.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPieceAdopt,
}

var pieceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all pieces",
	Long:  `List all pieces in a tree view showing parent/child relationships. Use --flat for a simple list.`,
	Args:  cobra.NoArgs,
	RunE:  runPieceList,
}

var flagMainBranch string
var flagMainBranchLegacy string
var flagPieceCleanupYes bool
var flagPieceName string
var flagParent string
var flagSkipSwitch bool
var flagDryRun bool
var flagForce bool
var flagPieceCleanupApply bool
var flagPieceCleanupJSON bool
var flagAbandonName string
var flagAbandonPiece string
var flagDonePiece string
var flagStatusPiece string
var flagDeleteBranch bool
var flagOverwriteSession bool
var flagPieceCreateSchema bool
var flagPieceUpdateSchema bool
var flagPieceMergeSchema bool
var flagPieceMergeReparent bool
var flagPieceMergeReparentStrategy string
var flagPieceCleanupSchema bool
var flagPieceAbandonSchema bool
var flagPieceDoneSchema bool
var flagPieceAdoptBranch string
var flagPieceAdoptName string
var flagPieceAdoptParent string
var flagPieceAdoptSchema bool
var flagPiecePrompt string
var flagPieceAgent string
var flagPieceListFlat bool
var flagPieceListAll bool
var flagPieceCreateJSON bool
var flagPieceDoneJSON bool
var flagPieceAdoptJSON bool
var flagPieceStatusJSON bool
var flagPieceUpdateJSON bool
var flagPieceMergeJSON bool
var flagPieceAbandonJSON bool
var flagPieceListJSON bool

func init() {
	pieceCreateCmd.Flags().StringVar(&flagPieceName, "name", "", "Optional piece name (default: auto-generated)")
	pieceCreateCmd.Flags().StringVar(&flagPiecePrompt, "prompt", "", "Create piece from prompt (e.g., \"add dark mode\")")
	pieceCreateCmd.Flags().StringVarP(&flagParent, "parent", "p", "", "Parent piece name to branch from (default: main)")
	pieceCreateCmd.Flags().BoolVar(&flagSkipSwitch, "skip-switch", false, "Don't switch to the new piece after creation")
	pieceCreateCmd.Flags().BoolVar(&flagOverwriteSession, "overwrite-session", false, "Replace existing main repo tmux session")
	pieceCreateCmd.Flags().BoolVar(&flagPieceCreateSchema, "schema", false, "Print an example input document and exit")
	pieceCreateCmd.Flags().BoolVar(&flagPieceCreateJSON, "json", false, "Output JSON even on a terminal")
	pieceCreateCmd.Flags().StringVar(&flagPieceAgent, "agent", "", "Launch an agent in the new piece: claude or codex (typed into the session, or run headless with --prompt)")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch name to merge (default: main)")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceUpdateCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceUpdateCmd.Flags().BoolVar(&flagPieceUpdateSchema, "schema", false, "Print an example input document and exit")
	pieceUpdateCmd.Flags().BoolVar(&flagPieceUpdateJSON, "json", false, "Output JSON even on a terminal")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch name to merge into (default: main)")
	pieceMergeCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceMergeCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceMergeCmd.Flags().BoolVar(&flagPieceMergeSchema, "schema", false, "Print an example input document and exit")
	pieceMergeCmd.Flags().BoolVar(&flagForce, "force", false, "Force merge even if piece has child pieces (children are NOT re-homed)")
	pieceMergeCmd.Flags().BoolVar(&flagPieceMergeReparent, "reparent-children", false, "Merge a piece that has child pieces: re-home them onto the merge target")
	pieceMergeCmd.Flags().StringVar(&flagPieceMergeReparentStrategy, "reparent-strategy", "", "How to re-home children: 'rebase' (default, rewrites history) or 'merge' (no force-push)")
	pieceMergeCmd.Flags().BoolVar(&flagPieceMergeJSON, "json", false, "Output JSON even on a terminal")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch name to check for merged status (default: main)")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceCleanupCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceCleanupCmd.Flags().BoolVar(&flagPieceCleanupApply, "apply", false, "Apply the cleanup (default is a dry-run preview)")
	pieceCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview what would be cleaned without changing anything")
	pieceCleanupCmd.Flags().BoolVarP(&flagPieceCleanupYes, "yes", "y", false, "Skip the interactive confirmation prompt (implies --apply)")
	pieceCleanupCmd.Flags().BoolVar(&flagPieceCleanupSchema, "schema", false, "Print an example input document and exit")
	pieceCleanupCmd.Flags().BoolVar(&flagPieceCleanupJSON, "json", false, "Output JSON even on a terminal")
	pieceAbandonCmd.Flags().StringVar(&flagAbandonPiece, "piece", "", "Piece to abandon (default: the piece you're in)")
	pieceAbandonCmd.Flags().StringVar(&flagAbandonName, "name", "", "Piece name to abandon (deprecated: use --piece)")
	_ = pieceAbandonCmd.Flags().MarkDeprecated("name", "use --piece")
	pieceAbandonCmd.Flags().BoolVar(&flagForce, "force", false, "Force removal even with uncommitted changes")
	pieceAbandonCmd.Flags().BoolVar(&flagDeleteBranch, "delete-branch", false, "Also delete the git branch")
	pieceAbandonCmd.Flags().BoolVar(&flagPieceAbandonSchema, "schema", false, "Print an example input document and exit")
	pieceAbandonCmd.Flags().BoolVar(&flagPieceAbandonJSON, "json", false, "Output JSON even on a terminal")
	pieceDoneCmd.Flags().StringVar(&flagDonePiece, "piece", "", "Piece to finish (default: the piece you're in)")
	pieceDoneCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch to check merge status against")
	pieceDoneCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceDoneCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceDoneCmd.Flags().BoolVar(&flagPieceDoneSchema, "schema", false, "Print an example input document and exit")
	pieceDoneCmd.Flags().BoolVar(&flagPieceDoneJSON, "json", false, "Output JSON even on a terminal")
	pieceAdoptCmd.Flags().StringVarP(&flagPieceAdoptBranch, "branch", "b", "", "Branch to adopt; local name or remote ref like origin/foo (defaults to current branch when on main)")
	pieceAdoptCmd.Flags().StringVar(&flagPieceAdoptName, "name", "", "Override piece name (defaults to branch name)")
	pieceAdoptCmd.Flags().StringVarP(&flagPieceAdoptParent, "parent", "p", "main", "Parent piece name (default: main)")
	pieceAdoptCmd.Flags().BoolVar(&flagPieceAdoptSchema, "schema", false, "Print an example input document and exit")
	pieceAdoptCmd.Flags().BoolVar(&flagPieceAdoptJSON, "json", false, "Output JSON even on a terminal")
	pieceStatusCmd.Flags().StringVar(&flagStatusPiece, "piece", "", "Piece to inspect (default: the piece you're in)")
	pieceStatusCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Main branch name (default: main)")
	pieceStatusCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceStatusCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceStatusCmd.Flags().BoolVar(&flagPieceStatusJSON, "json", false, "Output JSON even on a terminal")
	pieceListCmd.Flags().BoolVar(&flagPieceListFlat, "flat", false, "Display pieces in a flat list instead of tree view")
	pieceListCmd.Flags().BoolVar(&flagPieceListAll, "all", false, "List pieces across all registered projects")
	pieceListCmd.Flags().BoolVar(&flagPieceListJSON, "json", false, "Output JSON even on a terminal")
	rootCmd.AddCommand(pieceStatusCmd)
	rootCmd.AddCommand(pieceCreateCmd)
	rootCmd.AddCommand(pieceUpdateCmd)
	rootCmd.AddCommand(pieceMergeCmd)
	rootCmd.AddCommand(pieceCleanupCmd)
	rootCmd.AddCommand(pieceAbandonCmd)
	rootCmd.AddCommand(pieceDoneCmd)
	rootCmd.AddCommand(pieceAdoptCmd)
	rootCmd.AddCommand(pieceListCmd)

	// Register completion functions (errors ignored - completion is optional)
	_ = pieceAbandonCmd.RegisterFlagCompletionFunc("name", completePieceNames)
	_ = pieceUpdateCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	_ = pieceMergeCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	_ = pieceCleanupCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
}

// newPieceHandler creates a piece handler, choosing the multiplexer from the
// live interactivity context.
func newPieceHandler(deps core.Deps) *piececmd.Handler {
	return piececmd.NewHandlerWithMultiplexer(deps, chooseMultiplexer(deps.Exec))
}

// chooseMultiplexer picks the multiplexer for the configured provider, or the
// no-op multiplexer when mp should not manage sessions. Real session management
// (create / switch-client / attach) happens only when ALL hold:
//
//   - stdin is a TTY (cli.IsTerminal). Agents and scripts drive mp through its
//     stateless API (flags / stdin JSON, output captured), which is never a TTY.
//     MP_TMUX_PLUGIN=1 substitutes for the TTY check: the companion tmux plugin
//     (apps/tmux) drives mp through the stateless API (no controlling TTY) but
//     still wants mp to perform the switch-client/session-create. Only the
//     plugin sets it, per invocation -- an agent never does. The opt-in is
//     tmux-specific and never enables other providers.
//   - the configured multiplexer reports InSession() -- e.g. $TMUX set for tmux
//     -- so switching can move the user's existing client. The env var alone is
//     NOT trusted: it is inherited by child processes, so an agent spawned from
//     inside the user's tmux still has it set. The TTY check is what excludes
//     those callers -- without it, an agent's switch-client would hijack the
//     human's terminal and session-create would litter detached sessions.
//
// Outside this context, commands surface the worktree path (JSON or stdout) to
// cd into. Config problems also degrade to no-op with a warning so piece
// commands keep working with a broken user config.
func chooseMultiplexer(exec core.Exec) core.Multiplexer {
	pluginDriven := os.Getenv("MP_TMUX_PLUGIN") == "1"
	if !cli.IsTerminal() && !pluginDriven {
		return adapters.NewNoopMultiplexer()
	}

	userCfg, err := config.LoadUserConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring user config: %v\n", err)
		return adapters.NewNoopMultiplexer()
	}

	mux, err := adapters.NewMultiplexer(userCfg.Multiplexer, exec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; session management disabled\n", err)
		return adapters.NewNoopMultiplexer()
	}

	if !mux.InSession() {
		return adapters.NewNoopMultiplexer()
	}

	// The plugin opt-in stood in for the TTY check; it is only valid for tmux.
	if !cli.IsTerminal() && mux.Name() != "tmux" {
		return adapters.NewNoopMultiplexer()
	}

	return mux
}

func completePieceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(io.Discard),
		adapters.NewOSExec(),
		http.DefaultClient,
		core.NewLoadingSignal(),
	)
	handler := newPieceHandler(deps)

	// Get repo root from current directory
	repoRoot := ""
	wd, err := os.Getwd()
	if err == nil {
		git := adapters.NewGit(deps.Exec)
		detectedRoot, err := git.RepoRoot(cmd.Context(), wd)
		if err == nil {
			repoRoot = detectedRoot
		}
	}

	pieces, err := handler.ListPieces(cmd.Context(), repoRoot)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, p := range pieces {
		if strings.HasPrefix(p.Name, toComplete) {
			names = append(names, p.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeGitBranches(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	exec := adapters.NewOSExec()
	out, err := exec.Run(cmd.Context(), "git", "branch", "--format=%(refname:short)")
	if err != nil {
		return []string{"main", "master"}, cobra.ShellCompDirectiveNoFileComp
	}
	branches := strings.Split(strings.TrimSpace(string(out)), "\n")
	var filtered []string
	for _, b := range branches {
		if strings.HasPrefix(b, toComplete) {
			filtered = append(filtered, b)
		}
	}
	return filtered, cobra.ShellCompDirectiveNoFileComp
}

// emitResult honors the output contract: exactly one JSON document on stdout
// when the caller is a pipe/agent (or forces it with --json); a human at a
// terminal gets only the stderr messages the handlers already stream.
func emitResult(v any, jsonFlag bool) error {
	if jsonFlag || !cli.IsStdoutTerminal() || !cli.IsTerminal() {
		return cli.PrintJSON(v)
	}
	return nil
}

// mainBranchFromFlags returns the trunk name from --main (canonical) or the
// deprecated --main-branch alias, reporting whether either was set explicitly.
func mainBranchFromFlags(cmd *cobra.Command) (string, bool) {
	if cmd.Flags().Changed("main") {
		return flagMainBranch, true
	}
	if cmd.Flags().Changed("main-branch") {
		return flagMainBranchLegacy, true
	}
	return "", false
}

// pieceSelector merges the positional [piece] arg with a --piece flag value:
// either alone wins, both set to different names is an error, neither is "".
func pieceSelector(args []string, flagVal string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = strings.TrimSpace(args[0])
	}
	flagVal = strings.TrimSpace(flagVal)
	if positional != "" && flagVal != "" && positional != flagVal {
		return "", fmt.Errorf("conflicting piece selectors: %q (positional) vs %q (--piece)", positional, flagVal)
	}
	if positional != "" {
		return positional, nil
	}
	return flagVal, nil
}

// resolvePieceWorkDir returns the directory a piece-scoped command should run
// against: the named piece's worktree when a selector is given, else the
// current directory (the historical run-from-inside behavior).
func resolvePieceWorkDir(ctx context.Context, selector string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	if selector == "" {
		return wd, nil
	}
	git := adapters.NewGit(adapters.NewOSExec())
	root, err := git.GetMainRepoRoot(ctx, wd)
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	piecesDir, err := projectdir.PiecesDir(root)
	if err != nil {
		return "", err
	}
	piecePath := filepath.Join(piecesDir, selector)
	if fi, err := os.Stat(piecePath); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("piece %q not found in %s", selector, root)
	}
	return piecePath, nil
}

func runPieceStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	selector, err := pieceSelector(args, flagStatusPiece)
	if err != nil {
		return err
	}
	wd, err := resolvePieceWorkDir(ctx, selector)
	if err != nil {
		return err
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	// Get main branch (default to "main")
	mainBranch := "main"
	if v, ok := mainBranchFromFlags(cmd); ok && v != "" {
		mainBranch = v
	} else if flagMainBranch != "" {
		mainBranch = flagMainBranch
	}

	status, err := handler.GetPieceHierarchyStatus(ctx, wd, mainBranch)
	if err != nil {
		return err
	}

	// Output to stderr for human-readable text
	if status.InPiece {
		fmt.Fprintf(os.Stderr, "Current piece: %s\n\n", status.PieceName)

		// Display parent
		if status.Parent != "" && status.Parent != "main" {
			fmt.Fprintf(os.Stderr, "Parent: %s\n", status.Parent)
		} else {
			fmt.Fprintf(os.Stderr, "Parent: main\n")
		}

		// Display children
		if len(status.Children) == 0 {
			fmt.Fprintf(os.Stderr, "Children: none\n")
		} else {
			fmt.Fprintf(os.Stderr, "Children:\n")
			for _, child := range status.Children {
				fmt.Fprintf(os.Stderr, "  - %s\n", child)
			}
		}

		// Display stack depth
		if status.StackDepth > 0 {
			fmt.Fprintf(os.Stderr, "Stack depth: %d\n", status.StackDepth)
		}

		// Display merge readiness
		if !status.CanMerge && len(status.Children) > 0 {
			fmt.Fprintf(os.Stderr, "\n%s Cannot merge: has unmerged children\n", cli.GlyphWarn)
		}

		// Display the prompt the piece was created from, if any.
		meta, metaErr := piececmd.ReadPieceMetadata(status.WorktreePath, deps.FS)
		if metaErr == nil && meta != nil && meta.Prompt != "" {
			fmt.Fprintf(os.Stderr, "\nPrompt: %s\n", meta.Prompt)
		}

		// Display branch information
		git := adapters.NewGit(deps.Exec)
		branchName, err := git.CurrentBranch(ctx, status.WorktreePath)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Branch: %s\n", branchName)
		}
	} else {
		fmt.Fprintf(os.Stderr, "In main repository\n")
		if status.RepoRoot != "" {
			fmt.Fprintf(os.Stderr, "Repo root: %s\n", status.RepoRoot)
		}
	}

	// JSON is for pipes and agents (or --json); the human block above already
	// told the story on stderr.
	return emitResult(status, flagPieceStatusJSON)
}

func runPieceCreate(cmd *cobra.Command, args []string) error {
	// --schema mode: print an example input document and exit
	if flagPieceCreateSchema {
		schema, err := piececmd.NewPieceSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)

	handler := newPieceHandler(deps)

	// Get validated input from flags/stdin/TUI
	input, err := getPieceCreateInput(deps, wd)
	if err != nil {
		return err
	}

	opts := piececmd.CreatePieceOptions{
		OverwriteSession: input.OverwriteSession,
	}

	// Unified handler routes based on input
	info, err := handler.CreatePieceWithInput(ctx, input, opts)
	if err != nil {
		return err
	}

	// On a terminal the handler's success line tells the story; the JSON
	// payload is for pipes and agents (or --json).
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagPieceCreateJSON {
		jsonData, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal info: %w", err)
		}
		fmt.Println(string(jsonData))
	}

	// Switch to the new piece (unless skip_switch is set)
	if !input.SkipSwitch {
		_, err := handler.SwitchPiece(ctx, info.Name)
		if err != nil {
			// Non-fatal: log warning but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to switch to piece: %v\n", err)
		}
	}

	if input.Agent != "" {
		if err := launchAgentInPiece(ctx, deps, info, input); err != nil {
			// Non-fatal: the piece exists and is usable without the agent.
			fmt.Fprintf(os.Stderr, "Warning: failed to launch agent: %v\n", err)
		}
	} else {
		cli.Hint("mp pr create --draft")
	}

	return nil
}

// launchAgentInPiece starts the requested agent in a fresh piece. With a live
// multiplexer session the launch line is typed into it (interactive TUI);
// otherwise the agent runs headless in the worktree — which needs a prompt —
// detached, logging under the repo's monkeypuzzle logs dir. The agent's own
// integration hooks (mp integration install) take it from there: status
// reports need no help from this path.
func launchAgentInPiece(ctx context.Context, deps core.Deps, info piececmd.PieceInfo, input piececmd.NewPieceInput) error {
	spec, err := agentcmd.BuildLaunch(input.Agent, input.Prompt)
	if err != nil {
		return err
	}

	mux := chooseMultiplexer(deps.Exec)
	if pane, ok := mux.(core.PaneOps); ok && mux.Exists(ctx, info.SessionName) {
		if err := pane.SendText(ctx, info.SessionName, spec.Line); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Launched %s in session %s\n", spec.Kind, info.SessionName)
		return nil
	}

	if len(spec.Argv) == 0 {
		return fmt.Errorf("headless %s launch needs --prompt (no multiplexer session to type into)", spec.Kind)
	}
	repoRoot, err := projectdir.MainRepoRoot(info.WorktreePath)
	if err != nil {
		repoRoot = info.WorktreePath
	}
	logPath := filepath.Join(projectdir.LogsDir(repoRoot), "agent-"+spec.Kind+"-"+info.Name+".log")
	// Strip tmux identity from the headless agent's env: inherited TMUX_PANE
	// would be the *user's* pane, and the agent's report hooks would record it
	// — after which `mp agent send` / the plugin's focus would target the
	// user's own shell instead of the agent.
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "TMUX=") || strings.HasPrefix(e, "TMUX_PANE=") {
			continue
		}
		env = append(env, e)
	}
	if err := deps.Exec.StartDetached(info.WorktreePath, env, logPath, spec.Argv[0], spec.Argv[1:]...); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Launched %s headless in %s; output: %s\n", spec.Kind, info.WorktreePath, logPath)
	return nil
}

func getPieceCreateInput(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	var input piececmd.NewPieceInput
	var err error

	// Mode 1: Flags provided
	if flagPieceName != "" || flagPiecePrompt != "" {
		input = piececmd.NewPieceInput{
			Name:   flagPieceName,
			Prompt: flagPiecePrompt,
			Parent: flagParent,
		}
	} else if cli.HasStdinData() {
		// Mode 2: Stdin JSON
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.NewPieceInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseNewPieceJSON(data)
		if err != nil {
			return piececmd.NewPieceInput{}, err
		}
	} else if cli.IsTerminal() {
		// Mode 3: Interactive TUI
		input, err = runPieceCreateTUI(deps, workDir)
		if err != nil {
			return piececmd.NewPieceInput{}, err
		}
	} else {
		return piececmd.NewPieceInput{}, fmt.Errorf("no input provided; use --schema to see expected format")
	}

	// Flags override stdin/TUI options (flags take priority)
	if flagParent != "" {
		input.Parent = flagParent
	}
	if flagSkipSwitch {
		input.SkipSwitch = true
	}
	if flagOverwriteSession {
		input.OverwriteSession = true
	}
	if flagPieceAgent != "" {
		input.Agent = flagPieceAgent
	}

	// Apply defaults and validate inside input layer
	input = piececmd.WithNewPieceDefaults(input)
	if err := piececmd.ValidateNewPieceInput(input); err != nil {
		return piececmd.NewPieceInput{}, err
	}
	// Fail on an unknown agent kind before the piece is created.
	if input.Agent != "" {
		if _, err := agentcmd.BuildLaunch(input.Agent, input.Prompt); err != nil {
			return piececmd.NewPieceInput{}, err
		}
	}

	return input, nil
}

// runPieceCreateTUI prompts for a free-form description.
func runPieceCreateTUI(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	return runPromptInputTUI()
}

func runPromptInputTUI() (piececmd.NewPieceInput, error) {
	p := tea.NewProgram(promptinput.New())
	m, err := p.Run()
	if err != nil {
		return piececmd.NewPieceInput{}, fmt.Errorf("TUI error: %w", err)
	}

	model := m.(promptinput.Model)
	if model.Cancelled {
		return piececmd.NewPieceInput{}, fmt.Errorf("cancelled")
	}

	return piececmd.NewPieceInput{
		Prompt: model.Value,
	}, nil
}

func runPieceUpdate(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceUpdateSchema {
		schema, err := piececmd.UpdateSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	// Get input
	input, err := getUpdateInput(cmd)
	if err != nil {
		return err
	}

	result, err := handler.UpdatePiece(ctx, wd, input.MainBranch)
	if err != nil {
		return err
	}

	return emitResult(result, flagPieceUpdateJSON)
}

func getUpdateInput(cmd *cobra.Command) (piececmd.UpdateInput, error) {
	var input piececmd.UpdateInput

	// Read stdin JSON first, then let an explicit --main-branch flag override it.
	// The flag defaults to "main", so gating on Changed() avoids silently
	// clobbering a main_branch supplied via stdin (e.g. on a master-trunk repo).
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.UpdateInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseUpdateJSON(data)
		if err != nil {
			return piececmd.UpdateInput{}, err
		}
	}

	if v, ok := mainBranchFromFlags(cmd); ok {
		input.Main = v
		input.MainBranch = v
	}

	return piececmd.WithUpdateDefaults(input), nil
}

func runPieceMerge(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceMergeSchema {
		schema, err := piececmd.MergeSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	// Get input
	input, err := getMergeInput(cmd)
	if err != nil {
		return err
	}

	// If this piece has child pieces and the caller hasn't already chosen how to
	// handle them, ask (interactively) how to re-home them.
	if !input.Force && !input.ReparentChildren {
		if hs, hsErr := handler.GetPieceHierarchyStatus(ctx, wd, input.MainBranch); hsErr == nil && len(hs.Children) > 0 {
			if cli.IsTerminal() {
				choice, ok, cerr := chooser.Run(
					fmt.Sprintf("Piece %q has child pieces", hs.PieceName),
					[]string{"Children: " + strings.Join(hs.Children, ", "), "Merging it re-homes them onto the merge target."},
					[]chooser.Option{
						{Label: "Rebase children onto the target", Desc: "cleanest for stacks; rewrites their branch history (force-push if PRs are open)", Value: piececmd.ReparentRebase},
						{Label: "Merge the target into children", Desc: "no history rewrite; their branches keep the parent's commits alongside", Value: piececmd.ReparentMerge},
						{Label: "Cancel", Desc: "don't merge", Value: ""},
					},
				)
				if cerr != nil {
					return cerr
				}
				if !ok || choice == "" {
					fmt.Fprintln(os.Stderr, "Cancelled.")
					return nil
				}
				input.ReparentChildren = true
				input.ReparentStrategy = choice
			}
			// Non-interactive: leave it; MergePiece returns an error explaining --reparent-children.
		}
	}

	result, err := handler.MergePiece(ctx, wd, input)
	if err != nil {
		return err
	}

	cli.Hint("mp done")
	return emitResult(result, flagPieceMergeJSON)
}

func getMergeInput(cmd *cobra.Command) (piececmd.MergeInput, error) {
	var input piececmd.MergeInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.MergeInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseMergeJSON(data)
		if err != nil {
			return piececmd.MergeInput{}, err
		}
	}

	// Flags override JSON input. The trunk flag defaults to "main", so only
	// apply it when the user explicitly set it (otherwise a stdin value is lost).
	if v, ok := mainBranchFromFlags(cmd); ok {
		input.Main = v
		input.MainBranch = v
	}
	if flagForce {
		input.Force = true
	}
	if flagPieceMergeReparent {
		input.ReparentChildren = true
	}
	if flagPieceMergeReparentStrategy != "" {
		input.ReparentChildren = true
		input.ReparentStrategy = flagPieceMergeReparentStrategy
	}

	input = piececmd.WithMergeDefaults(input)
	if err := piececmd.ValidateMergeInput(input); err != nil {
		return piececmd.MergeInput{}, err
	}
	return input, nil
}

func runPieceCleanup(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceCleanupSchema {
		schema, err := piececmd.CleanupSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)

	handler := newPieceHandler(deps)

	// Get input
	input, err := getCleanupInput(cmd)
	if err != nil {
		return err
	}

	// Get repo root (either from piece or main repo)
	status, err := handler.Status(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to get piece status: %w", err)
	}

	repoRoot := status.RepoRoot
	if repoRoot == "" {
		return fmt.Errorf("not in a git repository")
	}

	// Cleanup is dry-run by default. Always preview first, then decide whether to
	// apply: --apply (or --force) opts in, --dry-run stays a preview, an
	// interactive terminal is asked to confirm, and any other (non-interactive)
	// caller previews.
	output, err := cleanupPass(ctx, handler, repoRoot, input.MainBranch, true)
	if err != nil {
		return err
	}

	anythingToDo := len(output.CleanedPieces) > 0 || len(output.RemovedProjects) > 0
	apply, err := resolveApply(input.Apply || flagPieceCleanupYes, input.DryRun, anythingToDo, func() (bool, error) {
		return confirmApply("Clean up merged pieces?", cleanupSummary(output))
	})
	if err != nil {
		return err
	}
	if apply {
		output, err = cleanupPass(ctx, handler, repoRoot, input.MainBranch, false)
		if err != nil {
			return err
		}
	}

	// Human summary always goes to stderr (like every other command's), so
	// stdout stays JSON-only.
	fmt.Fprintln(os.Stderr, cleanupHumanSummary(output, apply))
	if !apply {
		cli.Hint("mp cleanup --apply")
	}
	return emitResult(output, flagPieceCleanupJSON)
}

// cleanupHumanSummary is the one-line terminal wrap-up for `mp cleanup`.
func cleanupHumanSummary(output cleanupOutput, applied bool) string {
	if len(output.CleanedPieces) == 0 && len(output.RemovedProjects) == 0 {
		return "Nothing to clean."
	}
	names := make([]string, 0, len(output.CleanedPieces))
	for _, c := range output.CleanedPieces {
		names = append(names, c.PieceName)
	}
	verb := "Would clean"
	if applied {
		verb = "Cleaned"
	}
	s := fmt.Sprintf("%s %d piece(s): %s", verb, len(names), strings.Join(names, ", "))
	if n := len(output.RemovedProjects); n > 0 {
		s += fmt.Sprintf("; pruned %d stale project(s)", n)
	}
	return s
}

// cleanupPass runs one cleanup pass (merged-piece removal + stale-project prune)
// and returns the combined output. When dryRun is true nothing is mutated; it
// only reports what would be removed.
func cleanupPass(ctx context.Context, handler *piececmd.Handler, repoRoot, mainBranch string, dryRun bool) (cleanupOutput, error) {
	results, err := handler.CleanupMergedPieces(ctx, repoRoot, piececmd.CleanupOptions{
		DryRun:     dryRun,
		MainBranch: mainBranch,
	})
	if err != nil {
		return cleanupOutput{}, err
	}

	removedProjects, err := projectcmd.PruneStale(dryRun)
	if err != nil {
		return cleanupOutput{}, fmt.Errorf("failed to prune deleted projects: %w", err)
	}
	for _, p := range removedProjects {
		verb := "Pruned deleted project"
		if dryRun {
			verb = "[dry-run] Would prune deleted project"
		}
		fmt.Fprintf(os.Stderr, "%s: %s (%s)\n", verb, p.Name, p.Path)
	}

	output := cleanupOutput{CleanedPieces: results, RemovedProjects: removedProjects}
	if output.CleanedPieces == nil {
		output.CleanedPieces = []piececmd.CleanupResult{}
	}
	if output.RemovedProjects == nil {
		output.RemovedProjects = []registry.Project{}
	}
	return output, nil
}

// cleanupSummary renders a one-line summary of a cleanup preview for the
// interactive confirmation prompt.
func cleanupSummary(out cleanupOutput) string {
	pieces := make([]string, len(out.CleanedPieces))
	for i, p := range out.CleanedPieces {
		pieces[i] = p.PieceName
	}
	parts := []string{}
	if len(pieces) > 0 {
		parts = append(parts, fmt.Sprintf("remove %d merged piece(s): %s", len(pieces), strings.Join(pieces, ", ")))
	}
	if len(out.RemovedProjects) > 0 {
		parts = append(parts, fmt.Sprintf("prune %d deleted project(s)", len(out.RemovedProjects)))
	}
	return "Would " + strings.Join(parts, "; ")
}

// cleanupOutput is the stdout JSON shape for `mp cleanup` / `mp repair`.
type cleanupOutput struct {
	CleanedPieces   []piececmd.CleanupResult `json:"cleaned_pieces"`
	RemovedProjects []registry.Project       `json:"removed_projects"`
}

func getCleanupInput(cmd *cobra.Command) (piececmd.CleanupInput, error) {
	var input piececmd.CleanupInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.CleanupInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseCleanupJSON(data)
		if err != nil {
			return piececmd.CleanupInput{}, err
		}
	}

	// Flags override stdin. Gate the trunk flag on Changed() so a stdin value
	// is preserved when the flag was left at its "main" default.
	if v, ok := mainBranchFromFlags(cmd); ok {
		input.Main = v
		input.MainBranch = v
	}
	if flagDryRun {
		input.DryRun = true
	}
	if flagPieceCleanupApply {
		input.Apply = true
	}

	return piececmd.WithCleanupDefaults(input), nil
}

func runPieceAbandon(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceAbandonSchema {
		schema, err := piececmd.AbandonSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	// Get validated input. --piece is canonical; --name is the deprecated alias.
	selector, err := pieceSelector(args, flagAbandonPiece)
	if err != nil {
		return err
	}
	if selector == "" {
		selector = flagAbandonName
	}
	input, err := getAbandonInput(ctx, handler, selector)
	if err != nil {
		return err
	}

	opts := piececmd.AbandonOptions{
		Force:        input.Force,
		DeleteBranch: input.DeleteBranch,
	}

	result, err := handler.AbandonPiece(ctx, input.Name, opts)
	if err != nil {
		return err
	}

	return emitResult(result, flagPieceAbandonJSON)
}

func runPieceDone(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceDoneSchema {
		schema, err := piececmd.DoneSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	selector, err := pieceSelector(args, flagDonePiece)
	if err != nil {
		return err
	}
	wd, err := resolvePieceWorkDir(ctx, selector)
	if err != nil {
		return err
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)

	handler := newPieceHandler(deps)

	// Get input
	input, err := getDoneInput(cmd)
	if err != nil {
		return err
	}

	result, err := handler.DonePiece(ctx, wd, input)
	if err != nil {
		return err
	}

	cli.Hint("mp create, or mp go to pick up other work")
	return emitResult(result, flagPieceDoneJSON)
}

func getDoneInput(cmd *cobra.Command) (piececmd.DoneInput, error) {
	var input piececmd.DoneInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.DoneInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseDoneJSON(data)
		if err != nil {
			return piececmd.DoneInput{}, err
		}
	}

	// Flags override stdin. The trunk flag defaults to "main", so only apply it
	// when explicitly set to avoid clobbering a stdin value.
	if v, ok := mainBranchFromFlags(cmd); ok {
		input.Main = v
		input.MainBranch = v
	}

	return piececmd.WithDoneDefaults(input), nil
}

func runPieceAdopt(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPieceAdoptSchema {
		schema, err := piececmd.AdoptPieceSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	// Positional arg takes precedence over --branch flag
	if len(args) > 0 && flagPieceAdoptBranch == "" {
		flagPieceAdoptBranch = args[0]
	}

	ctx := cmd.Context()
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	// Get input
	input, err := getAdoptInput()
	if err != nil {
		return err
	}

	info, err := handler.AdoptPiece(ctx, input)
	if err != nil {
		return err
	}

	// On a terminal the handler's success line tells the story; the JSON
	// payload is for pipes and agents (or --json).
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagPieceAdoptJSON {
		jsonData, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(jsonData))
	}

	// Switch to the adopted piece. In an interactive session this creates and
	// attaches the piece's tmux session; for agents/automation the multiplexer is
	// the no-op (see chooseMultiplexer), so this does nothing and the caller reads
	// the worktree path from the JSON above.
	if _, err := handler.SwitchPiece(ctx, info.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to switch to piece: %v\n", err)
	}

	return nil
}

func getAdoptInput() (piececmd.AdoptPieceInput, error) {
	var input piececmd.AdoptPieceInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.AdoptPieceInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseAdoptPieceJSON(data)
		if err != nil {
			return piececmd.AdoptPieceInput{}, err
		}
	}

	// Flags override stdin
	if flagPieceAdoptBranch != "" {
		input.Branch = flagPieceAdoptBranch
	}
	if flagPieceAdoptName != "" {
		input.Name = flagPieceAdoptName
	}
	if flagPieceAdoptParent != "" {
		input.Parent = flagPieceAdoptParent
	}

	return piececmd.WithAdoptPieceDefaults(input), nil
}

func getAbandonInput(ctx context.Context, handler *piececmd.Handler, selector string) (piececmd.AbandonInput, error) {
	var input piececmd.AbandonInput

	if selector != "" {
		input = piececmd.AbandonInput{Name: selector}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.AbandonInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseAbandonJSON(data)
		if err != nil {
			return piececmd.AbandonInput{}, err
		}
	}

	// Flags override stdin options
	if flagForce {
		input.Force = true
	}
	if flagDeleteBranch {
		input.DeleteBranch = true
	}

	// Abandon only ever targets the current piece. When no name was supplied
	// (the common interactive case: `mp abandon` run from inside a piece), detect
	// it from the working directory rather than prompting which piece to abandon.
	if input.Name == "" {
		wd, err := os.Getwd()
		if err != nil {
			return piececmd.AbandonInput{}, fmt.Errorf("failed to get working directory: %w", err)
		}
		status, err := handler.Status(ctx, wd)
		if err != nil {
			return piececmd.AbandonInput{}, fmt.Errorf("failed to get piece status: %w", err)
		}
		if status.InPiece {
			input.Name = status.PieceName
		} else {
			return piececmd.AbandonInput{}, fmt.Errorf("not inside a piece; run from within the piece to abandon, or pass a piece name")
		}
	}

	input = piececmd.WithAbandonDefaults(input)
	if err := piececmd.ValidateAbandonInput(input); err != nil {
		return piececmd.AbandonInput{}, err
	}

	return input, nil
}

func runPieceList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	if flagPieceListAll {
		return runPieceListAll(ctx, handler)
	}

	pieces, err := handler.ListPieces(ctx, "")
	if err != nil {
		return err
	}

	if flagPieceListFlat {
		// --flat has no separate human rendering — it *is* the machine format,
		// requested explicitly — so it always prints JSON regardless of TTY.
		// (Gating this on TTY like emitResult would leave a terminal caller
		// looking at nothing: a real regression a review once caught here.)
		return cli.PrintJSON(pieces)
	}

	// Tree view is the human message: always stderr, like every other
	// command's summary line, so `mp list | jq` sees only JSON on stdout
	// (gated the same way as everywhere else) and never a tree it can't parse.
	renderTree(piececmd.BuildPieceTree(pieces))
	return emitResult(pieces, flagPieceListJSON)
}

// projectPieces is the per-project entry in `mp list --all` JSON output.
type projectPieces struct {
	Name   string                   `json:"name"`
	Path   string                   `json:"path"`
	Pieces []piececmd.PieceListItem `json:"pieces"`
	Error  string                   `json:"error,omitempty"`
}

func runPieceListAll(ctx context.Context, handler *piececmd.Handler) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	results := make([]projectPieces, 0, len(reg.Projects))
	for _, p := range reg.Projects {
		entry := projectPieces{Name: p.Name, Path: p.Path}
		pieces, err := handler.ListPieces(ctx, p.Path)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Pieces = pieces
		}
		results = append(results, entry)
	}

	if flagPieceListFlat {
		// See the matching comment in runPieceList: --flat is the machine
		// format on request, always unconditional JSON, never TTY-gated.
		return cli.PrintJSON(map[string]any{"projects": results})
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No registered projects. Run `mp init` in a repo, or `mp project add <path>`.")
	}
	for i, entry := range results {
		if i > 0 {
			fmt.Fprintln(os.Stderr)
		}
		fmt.Fprintf(os.Stderr, "# %s (%s)\n", entry.Name, entry.Path)
		if entry.Error != "" {
			fmt.Fprintf(os.Stderr, "  error: %s\n", entry.Error)
			continue
		}
		renderTree(piececmd.BuildPieceTree(entry.Pieces))
	}
	return emitResult(map[string]any{"projects": results}, flagPieceListJSON)
}

// renderTree renders a piece tree in human-readable form. It writes to
// stderr, like every other command's human summary — stdout stays JSON-only
// (see emitResult) so `mp list | jq` never has to parse a tree.
func renderTree(root *piececmd.TreeNode) {
	if root == nil {
		return
	}

	// Print "main" as root
	fmt.Fprintln(os.Stderr, "main")

	// Render children recursively
	renderTreeNodes(root.Children, "")
}

// renderTreeNodes recursively renders tree nodes with proper indentation.
func renderTreeNodes(nodes []*piececmd.TreeNode, prefix string) {
	for i, node := range nodes {
		isLastChild := i == len(nodes)-1

		// Determine the connector symbols
		var connector, childPrefix string
		if isLastChild {
			connector = "└── "
			childPrefix = prefix + "    "
		} else {
			connector = "├── "
			childPrefix = prefix + "│   "
		}

		// Handle orphan node
		if node.IsOrphan {
			fmt.Fprintf(os.Stderr, "%s%s(orphaned)\n", prefix, connector)
			renderTreeNodes(node.Children, childPrefix)
			continue
		}

		// Handle regular piece node
		if node.Piece != nil {
			timeStr := formatTimeAgo(node.Piece.ModTime)
			fmt.Fprintf(os.Stderr, "%s%s%s (%s)\n", prefix, connector, node.Piece.Name, timeStr)
			renderTreeNodes(node.Children, childPrefix)
		}
	}
}

// formatTimeAgo formats a time as "X ago" (e.g., "1h ago", "30m ago")
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
