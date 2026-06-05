package mp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/issuepicker"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/modepicker"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/pieceswitch"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/promptinput"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var pieceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current piece status",
	Long:  `Print the current piece's status (parent, children, issue, branch) as JSON. Run from within a piece worktree, or the main repo.`,
	RunE:  runPieceStatus,
}

var pieceCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new puzzle piece",
	Long: `Create a new puzzle piece by initializing a git worktree and opening a tmux session.
The worktree will be created in a repo-scoped directory within the platform-appropriate data directory (e.g., ~/Library/Application Support/monkeypuzzle/pieces/{repo-hash}/ on macOS, ~/.local/share/monkeypuzzle/pieces/{repo-hash}/ on Linux).`,
	RunE: runPieceCreate,
}

var pieceUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update piece with latest from main branch",
	Long:  `Merges the main branch into the current piece's history. Must be run from within a piece worktree.`,
	RunE:  runPieceUpdate,
}

var pieceMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge piece back into main branch",
	Long:  `Merges the piece branch back into main. Fails if main has commits not in the piece worktree. Must be run from within a piece worktree.`,
	RunE:  runPieceMerge,
}

var pieceCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup merged pieces",
	Long:  `Finds and removes pieces whose branches have been merged. Removes worktrees, kills tmux sessions, and updates issue status to done.`,
	RunE:  runPieceCleanup,
}

var pieceAbandonCmd = &cobra.Command{
	Use:   "abandon",
	Short: "Abandon an unmerged piece",
	Long: `Remove a piece worktree, kill its tmux session, and optionally delete the branch.
Use --force to discard uncommitted changes.
Use --delete-branch to also remove the git branch.
If no --name is provided and run from within a piece, abandons the current piece.`,
	RunE: runPieceAbandon,
}

var pieceDoneCmd = &cobra.Command{
	Use:   "done",
	Short: "Cleanup current piece after merge",
	Long: `Remove the current piece worktree and tmux session after the branch has been merged.
Must be run from within a piece worktree. Verifies the piece is merged before cleanup.
Use 'mp abandon' for unmerged pieces.`,
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
	RunE: runPieceAdopt,
}

var pieceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all pieces",
	Long:  `List all pieces in a tree view showing parent/child relationships. Use --flat for a simple list.`,
	RunE:  runPieceList,
}

var flagMainBranch string
var flagPieceName string
var flagIssuePath string
var flagParent string
var flagSkipSwitch bool
var flagDryRun bool
var flagForce bool
var flagAbandonName string
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
var flagPieceListFlat bool
var flagPieceListAll bool

func init() {
	pieceCreateCmd.Flags().StringVar(&flagPieceName, "name", "", "Optional piece name (default: auto-generated)")
	pieceCreateCmd.Flags().StringVar(&flagIssuePath, "issue", "", "Create piece from issue file (e.g., issues/foo.md)")
	pieceCreateCmd.Flags().StringVar(&flagPiecePrompt, "prompt", "", "Create piece from prompt (e.g., \"add dark mode\")")
	pieceCreateCmd.Flags().StringVarP(&flagParent, "parent", "p", "", "Parent piece name to branch from (default: main)")
	pieceCreateCmd.Flags().BoolVar(&flagSkipSwitch, "skip-switch", false, "Don't switch to the new piece after creation")
	pieceCreateCmd.Flags().BoolVar(&flagOverwriteSession, "overwrite-session", false, "Replace existing main repo tmux session")
	pieceCreateCmd.Flags().BoolVar(&flagPieceCreateSchema, "schema", false, "Output JSON schema and exit")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge (default: main)")
	pieceUpdateCmd.Flags().BoolVar(&flagPieceUpdateSchema, "schema", false, "Output JSON schema and exit")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge into (default: main)")
	pieceMergeCmd.Flags().BoolVar(&flagPieceMergeSchema, "schema", false, "Output JSON schema and exit")
	pieceMergeCmd.Flags().BoolVar(&flagForce, "force", false, "Force merge even if piece has child pieces (children are NOT re-homed)")
	pieceMergeCmd.Flags().BoolVar(&flagPieceMergeReparent, "reparent-children", false, "Merge a piece that has child pieces: re-home them onto the merge target")
	pieceMergeCmd.Flags().StringVar(&flagPieceMergeReparentStrategy, "reparent-strategy", "", "How to re-home children: 'rebase' (default, rewrites history) or 'merge' (no force-push)")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to check for merged status (default: main)")
	pieceCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be cleaned without making changes")
	pieceCleanupCmd.Flags().BoolVar(&flagForce, "force", false, "Skip confirmation prompts")
	pieceCleanupCmd.Flags().BoolVar(&flagPieceCleanupSchema, "schema", false, "Output JSON schema and exit")
	pieceAbandonCmd.Flags().StringVar(&flagAbandonName, "name", "", "Piece name to abandon (optional if in piece)")
	pieceAbandonCmd.Flags().BoolVar(&flagForce, "force", false, "Force removal even with uncommitted changes")
	pieceAbandonCmd.Flags().BoolVar(&flagDeleteBranch, "delete-branch", false, "Also delete the git branch")
	pieceAbandonCmd.Flags().BoolVar(&flagPieceAbandonSchema, "schema", false, "Output JSON schema and exit")
	pieceDoneCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch to check merge status against")
	pieceDoneCmd.Flags().BoolVar(&flagPieceDoneSchema, "schema", false, "Output JSON schema and exit")
	pieceAdoptCmd.Flags().StringVarP(&flagPieceAdoptBranch, "branch", "b", "", "Branch to adopt; local name or remote ref like origin/foo (defaults to current branch when on main)")
	pieceAdoptCmd.Flags().StringVar(&flagPieceAdoptName, "name", "", "Override piece name (defaults to branch name)")
	pieceAdoptCmd.Flags().StringVarP(&flagPieceAdoptParent, "parent", "p", "main", "Parent piece name (default: main)")
	pieceAdoptCmd.Flags().BoolVar(&flagPieceAdoptSchema, "schema", false, "Output JSON schema and exit")
	pieceStatusCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name (default: main)")
	pieceListCmd.Flags().BoolVar(&flagPieceListFlat, "flat", false, "Display pieces in a flat list instead of tree view")
	pieceListCmd.Flags().BoolVar(&flagPieceListAll, "all", false, "List pieces across all registered projects")
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
	_ = pieceCreateCmd.RegisterFlagCompletionFunc("issue", completeIssueFiles)
	_ = pieceUpdateCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	_ = pieceMergeCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	_ = pieceCleanupCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
}

// newPieceHandler creates a piece handler with multiplexer based on user config.
func newPieceHandler(deps core.Deps) *piececmd.Handler {
	userCfg, err := config.LoadUserConfig()
	if err != nil {
		// Fall back to noop on config error
		return newPieceHandler(deps)
	}

	mux, err := adapters.NewMultiplexer(userCfg.Multiplexer, deps.Exec)
	if err != nil {
		// Fall back to noop on unknown provider
		return newPieceHandler(deps)
	}

	return piececmd.NewHandlerWithMultiplexer(deps, mux)
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

func completeIssueFiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Allow file completion, filtered to issues/ directory by default
	return nil, cobra.ShellCompDirectiveDefault
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

func runPieceStatus(cmd *cobra.Command, args []string) error {
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

	// Get main branch (default to "main")
	mainBranch := "main"
	if flagMainBranch != "" {
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
			fmt.Fprintf(os.Stderr, "\n⚠ Cannot merge: has unmerged children\n")
		}

		// Display the prompt the piece was created from, if any.
		meta, metaErr := piececmd.ReadPieceMetadata(status.WorktreePath, deps.FS)
		if metaErr == nil && meta != nil && meta.Prompt != "" {
			fmt.Fprintf(os.Stderr, "\nPrompt: %s\n", meta.Prompt)
		}

		// Display issue information if available
		marker, err := handler.ReadCurrentIssueMarker(status.WorktreePath)
		if err == nil && marker != nil && !marker.Issue.IsEmpty() {
			fmt.Fprintf(os.Stderr, "\nIssue: %s\n", marker.Issue.DisplayName())
			if marker.Status != "" {
				fmt.Fprintf(os.Stderr, "Status: %s\n", marker.Status)
			}
			if marker.Dirty {
				fmt.Fprintf(os.Stderr, "⚠ Unsynced changes (run 'mp issue sync')\n")
			}
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

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func runPieceCreate(cmd *cobra.Command, args []string) error {
	// --schema mode: output JSON schema and exit
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

	// Setup deps with issue sync
	issueSync := core.NewIssueSyncSignal()
	deps := core.NewDepsWithSync(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
		issueSync,
	)

	// Register issue sync subscriber
	issueSync.Sub(newIssueSyncSubscriber(wd, deps, os.Stderr))

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

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal info: %w", err)
	}
	fmt.Println(string(jsonData))

	// Switch to the new piece (unless skip_switch is set)
	if !input.SkipSwitch {
		_, err := handler.SwitchPiece(ctx, info.Name)
		if err != nil {
			// Non-fatal: log warning but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to switch to piece: %v\n", err)
		}
	}

	return nil
}

func getPieceCreateInput(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	var input piececmd.NewPieceInput
	var err error

	// Mode 1: Flags provided
	if flagIssuePath != "" || flagPieceName != "" || flagPiecePrompt != "" {
		input = piececmd.NewPieceInput{
			Name:   flagPieceName,
			Prompt: flagPiecePrompt,
			Parent: flagParent,
		}
		// Look up issue via provider if --issue flag specified
		if flagIssuePath != "" {
			tuiDeps := core.NewDeps(deps.FS, deps.Output, deps.Exec, deps.HTTP, nil)
			issueHandler := issue.NewHandler(tuiDeps, workDir)
			issues, err := issueHandler.SearchIssues(issue.SearchInput{
				Query: flagIssuePath,
				Limit: 1,
			})
			if err != nil {
				return piececmd.NewPieceInput{}, fmt.Errorf("failed to find issue %q: %w", flagIssuePath, err)
			}
			if len(issues) == 0 {
				return piececmd.NewPieceInput{}, fmt.Errorf("no issue found matching %q", flagIssuePath)
			}
			input.Issue = issues[0].ToIssueRef()
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

	// Apply defaults and validate inside input layer
	input = piececmd.WithNewPieceDefaults(input)
	if err := piececmd.ValidateNewPieceInput(input); err != nil {
		return piececmd.NewPieceInput{}, err
	}

	return input, nil
}

func runPieceCreateTUI(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	tuiDeps := core.NewDeps(deps.FS, deps.Output, deps.Exec, deps.HTTP, nil)
	issueHandler := issue.NewHandler(tuiDeps, workDir)
	issues, _ := issueHandler.ListIssues([]string{piececmd.StatusTodo})

	// If no issues, skip straight to prompt input
	if len(issues) == 0 {
		return runPromptInputTUI()
	}

	// Show mode picker: "From issue" or "From prompt"
	p := tea.NewProgram(modepicker.New())
	m, err := p.Run()
	if err != nil {
		return piececmd.NewPieceInput{}, fmt.Errorf("TUI error: %w", err)
	}

	picker := m.(modepicker.Model)
	if picker.Cancelled {
		return piececmd.NewPieceInput{}, fmt.Errorf("cancelled")
	}

	if picker.Chosen == modepicker.ModePrompt {
		return runPromptInputTUI()
	}

	// Issue picker flow
	searchFn := func(query string) tea.Cmd {
		return func() tea.Msg {
			results, err := issueHandler.SearchIssues(issue.SearchInput{
				Query:  query,
				Status: []string{piececmd.StatusTodo},
				Limit:  100,
			})
			return issuepicker.IssuesLoadedMsg{
				Query:  query,
				Issues: results,
				Err:    err,
			}
		}
	}

	ip := tea.NewProgram(issuepicker.NewWithSearch(issues, searchFn))
	im, err := ip.Run()
	if err != nil {
		return piececmd.NewPieceInput{}, fmt.Errorf("TUI error: %w", err)
	}

	finalModel := im.(issuepicker.Model)
	if finalModel.Cancelled {
		return piececmd.NewPieceInput{}, fmt.Errorf("cancelled")
	}

	selectedIssue, ok := finalModel.SelectedIssue()
	if !ok {
		return piececmd.NewPieceInput{}, fmt.Errorf("no issue selected")
	}

	return piececmd.NewPieceInput{
		Issue: selectedIssue.ToIssueRef(),
	}, nil
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
	input, err := getUpdateInput()
	if err != nil {
		return err
	}

	result, err := handler.UpdatePiece(ctx, wd, input.MainBranch)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getUpdateInput() (piececmd.UpdateInput, error) {
	var input piececmd.UpdateInput

	if flagMainBranch != "" {
		input = piececmd.UpdateInput{MainBranch: flagMainBranch}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.UpdateInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseUpdateJSON(data)
		if err != nil {
			return piececmd.UpdateInput{}, err
		}
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
	input, err := getMergeInput()
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

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getMergeInput() (piececmd.MergeInput, error) {
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

	// Flags override JSON input
	if flagMainBranch != "" {
		input.MainBranch = flagMainBranch
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

	// Setup deps with issue sync
	issueSync := core.NewIssueSyncSignal()
	deps := core.NewDepsWithSync(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
		issueSync,
	)

	// Register issue sync subscriber
	issueSync.Sub(newIssueSyncSubscriber(wd, deps, os.Stderr))

	handler := newPieceHandler(deps)

	// Get input
	input, err := getCleanupInput()
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

	opts := piececmd.CleanupOptions{
		DryRun:     input.DryRun,
		Force:      input.Force,
		MainBranch: input.MainBranch,
	}

	results, err := handler.CleanupMergedPieces(ctx, repoRoot, opts)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getCleanupInput() (piececmd.CleanupInput, error) {
	var input piececmd.CleanupInput

	// Flags take priority
	if flagMainBranch != "" || flagDryRun || flagForce {
		input = piececmd.CleanupInput{
			MainBranch: flagMainBranch,
			DryRun:     flagDryRun,
			Force:      flagForce,
		}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.CleanupInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseCleanupJSON(data)
		if err != nil {
			return piececmd.CleanupInput{}, err
		}
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

	// Get validated input
	input, err := getAbandonInput(ctx, handler)
	if err != nil {
		if err.Error() == "cancelled" || err.Error() == "no pieces" {
			return nil
		}
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

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
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
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Setup deps with issue sync
	issueSync := core.NewIssueSyncSignal()
	deps := core.NewDepsWithSync(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
		issueSync,
	)

	// Register issue sync subscriber
	issueSync.Sub(newIssueSyncSubscriber(wd, deps, os.Stderr))

	handler := newPieceHandler(deps)

	// Get input
	input, err := getDoneInput()
	if err != nil {
		return err
	}

	result, err := handler.DonePiece(ctx, wd, input)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getDoneInput() (piececmd.DoneInput, error) {
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

	// Flags override stdin
	if flagMainBranch != "" {
		input.MainBranch = flagMainBranch
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

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

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

func getAbandonInput(ctx context.Context, handler *piececmd.Handler) (piececmd.AbandonInput, error) {
	var input piececmd.AbandonInput
	var err error

	if flagAbandonName != "" {
		input = piececmd.AbandonInput{Name: flagAbandonName}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.AbandonInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseAbandonJSON(data)
		if err != nil {
			return piececmd.AbandonInput{}, err
		}
	} else if cli.IsTerminal() {
		input, err = runAbandonTUI(ctx, handler)
		if err != nil {
			return piececmd.AbandonInput{}, err
		}
	}

	// Flags override stdin/TUI options
	if flagForce {
		input.Force = true
	}
	if flagDeleteBranch {
		input.DeleteBranch = true
	}

	// If name is still empty, try to detect from current piece
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
		}
	}

	input = piececmd.WithAbandonDefaults(input)
	if err := piececmd.ValidateAbandonInput(input); err != nil {
		return piececmd.AbandonInput{}, err
	}

	return input, nil
}

func runAbandonTUI(ctx context.Context, handler *piececmd.Handler) (piececmd.AbandonInput, error) {
	// Get repo root from current directory (use GetMainRepoRoot for worktree support)
	repoRoot := ""
	wd, err := os.Getwd()
	if err == nil {
		git := adapters.NewGit(adapters.NewOSExec())
		detectedRoot, err := git.GetMainRepoRoot(ctx, wd)
		if err != nil {
			detectedRoot, err = git.RepoRoot(ctx, wd)
		}
		if err == nil {
			repoRoot = detectedRoot
		}
	}

	pieces, err := handler.ListPieces(ctx, repoRoot)
	if err != nil {
		return piececmd.AbandonInput{}, err
	}

	if len(pieces) == 0 {
		fmt.Fprintln(os.Stderr, "No pieces to abandon.")
		return piececmd.AbandonInput{}, fmt.Errorf("no pieces")
	}

	p := tea.NewProgram(pieceswitch.New(pieces))
	m, err := p.Run()
	if err != nil {
		return piececmd.AbandonInput{}, fmt.Errorf("TUI error: %w", err)
	}

	finalModel := m.(pieceswitch.Model)
	if finalModel.Cancelled {
		return piececmd.AbandonInput{}, fmt.Errorf("cancelled")
	}

	return piececmd.AbandonInput{Name: pieces[finalModel.Selected].Name}, nil
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

	// Output JSON to stdout
	if flagPieceListFlat {
		// Flat list: just output pieces as JSON array
		jsonData, err := json.MarshalIndent(pieces, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(jsonData))
	} else {
		// Tree view: build tree and render
		tree := piececmd.BuildPieceTree(pieces)
		renderTree(tree)
	}

	return nil
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

	if flagPieceListFlat || !cli.IsStdoutTerminal() {
		jsonData, err := json.MarshalIndent(map[string]any{"projects": results}, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	}

	if len(results) == 0 {
		fmt.Println("No registered projects. Run `mp init` in a repo, or `mp project add <path>`.")
		return nil
	}
	for i, entry := range results {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("# %s (%s)\n", entry.Name, entry.Path)
		if entry.Error != "" {
			fmt.Printf("  error: %s\n", entry.Error)
			continue
		}
		renderTree(piececmd.BuildPieceTree(entry.Pieces))
	}
	return nil
}

// renderTree renders a piece tree to stdout in human-readable format
func renderTree(root *piececmd.TreeNode) {
	if root == nil {
		return
	}

	// Print "main" as root
	fmt.Println("main")

	// Render children recursively
	renderTreeNodes(root.Children, "")
}

// renderTreeNodes recursively renders tree nodes with proper indentation
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
			fmt.Printf("%s%s(orphaned)\n", prefix, connector)
			renderTreeNodes(node.Children, childPrefix)
			continue
		}

		// Handle regular piece node
		if node.Piece != nil {
			timeStr := formatTimeAgo(node.Piece.ModTime)
			fmt.Printf("%s%s%s (%s)\n", prefix, connector, node.Piece.Name, timeStr)
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

// newIssueSyncSubscriber creates a subscriber that syncs status changes to providers.
func newIssueSyncSubscriber(workDir string, deps core.Deps, output io.Writer) func(event core.IssueSyncEvent) {
	return func(event core.IssueSyncEvent) {
		if event.IssueID == "" {
			return
		}

		// Sync to provider
		handler := issue.NewHandler(deps, workDir)
		err := handler.SyncStatus(event.IssueID, event.NewStatus)
		if err != nil {
			_, _ = fmt.Fprintf(output, "⚠ Failed to sync: %v\n", err)
			return
		}

		// Clear dirty flag
		if event.WorktreePath != "" {
			_ = piececmd.ClearIssueDirtyFlag(event.WorktreePath, deps.FS)
		}

		_, _ = fmt.Fprintf(output, "✓ Synced %s → %s\n", event.IssueID, event.NewStatus)
	}
}
