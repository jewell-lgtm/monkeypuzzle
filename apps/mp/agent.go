package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	agentcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Track agent processes running in pieces",
	Long: `Agents (Claude Code, codex, ...) running in piece sessions are detected with
nothing installed into the agent: mp recognizes agent processes in the
session's panes and reads their state (blocked/working/idle) off the screen.
Each piece aggregates its agents by severity (blocked > working > done >
idle) and surfaces the result in 'mp go --json' for pickers and status lines.

'mp integration install claude' is an optional upgrade: the agent's own hooks
then report exact state — adding the "done" status, stable session ids, and
the agent-blocked/agent-done lifecycle hooks — instead of screen inference.`,
}

var agentReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report an agent's status (called by integration hooks)",
	Long: `Upsert the calling agent's status on the piece the working directory belongs
to. Outside a piece worktree this is a silent no-op (exit 0), so integration
hooks are safe to install globally.

--claude-hook reads a Claude Code hook payload from stdin and derives the
agent id and status from it; otherwise status comes from --status/stdin JSON.
Status "gone" removes the agent's record.`,
	Args: cobra.NoArgs,
	RunE: runAgentReport,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List live agents across the project's pieces",
	Long: `List live agents in the current project's pieces (blocked first). --all spans
every registered project instead; outside a git repo, --all is implied — so
status lines and cross-project pickers work from anywhere.`,
	Args: cobra.NoArgs,
	RunE: runAgentList,
}

var agentSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Compact one-line agent summary (for status lines)",
	Args:  cobra.NoArgs,
	RunE:  runAgentSummary,
}

var agentReadCmd = &cobra.Command{
	Use:   "read <agent-id|piece>",
	Short: "Capture an agent's pane contents",
	Long: `Print the current visible contents of the pane an agent runs in — check on a
worker without switching focus to it. Accepts an agent id or a piece name (the
piece's most attention-worthy agent wins). Requires a multiplexer with pane
support (tmux).`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentRead,
}

var agentSendCmd = &cobra.Command{
	Use:   "send <agent-id|piece> <text>...",
	Short: "Type text into an agent's pane",
	Long: `Send text (plus Enter) to the pane an agent runs in, as if typed — answer a
blocked agent or hand it a follow-up prompt without switching focus. Accepts
an agent id or piece name. Requires a multiplexer with pane support (tmux).

The text lands in the agent's input verbatim: you are prompting the agent.`,
	Args: cobra.MinimumNArgs(2),
	RunE: runAgentSend,
}

var agentFocusCmd = &cobra.Command{
	Use:   "focus [agent-id|piece]",
	Short: "Switch the client to an agent's pane",
	Long: `Move your tmux client to an agent's pane — the CLI form of the tmux plugin's
agent picker and blocked-jump chords, so either can be one mp invocation.

Accepts an agent id or piece name (mutually exclusive with --blocked); with
--blocked, picks the most urgent blocked agent instead and does nothing
(exit 0, warning on stderr) if none are blocked. If the agent's session is no
longer live, falls back to a plain piece switch — mp switch's own contract:
attach, never adopt or create. --json only applies to a direct pane focus;
the switch fallback always follows mp switch's fixed output (an attach
message on stderr, or the bare worktree path on stdout for
cd $(mp agent focus ...)), regardless of --json.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgentFocus,
}

var (
	flagAgentStatus       string
	flagAgentID           string
	flagAgentKind         string
	flagAgentPID          int
	flagAgentPane         string
	flagAgentClaudeHook   bool
	flagAgentSchema       bool
	flagAgentListJSON     bool
	flagAgentListAll      bool
	flagAgentFocusBlocked bool
	flagAgentFocusJSON    bool
)

func init() {
	agentReportCmd.Flags().StringVar(&flagAgentStatus, "status", "", "Agent status: working, blocked, done, idle, or gone")
	agentReportCmd.Flags().StringVar(&flagAgentID, "id", "", "Agent id (defaults to pid-<pid>)")
	agentReportCmd.Flags().StringVar(&flagAgentKind, "kind", "", "Agent kind: claude, codex, ...")
	agentReportCmd.Flags().IntVar(&flagAgentPID, "pid", 0, "Agent process id (defaults to the parent pid)")
	agentReportCmd.Flags().StringVar(&flagAgentPane, "pane", "", "Multiplexer pane the agent runs in (defaults to the current pane)")
	agentReportCmd.Flags().BoolVar(&flagAgentClaudeHook, "claude-hook", false, "Parse a Claude Code hook payload from stdin")
	agentReportCmd.Flags().BoolVar(&flagAgentSchema, "schema", false, "Print an example input document and exit")
	agentListCmd.Flags().BoolVar(&flagAgentListJSON, "json", false, "Output JSON instead of the table")
	agentListCmd.Flags().BoolVar(&flagAgentListAll, "all", false, "Span all registered projects (implied outside a git repo)")
	agentSummaryCmd.Flags().BoolVar(&flagAgentListAll, "all", false, "Span all registered projects (implied outside a git repo)")
	agentFocusCmd.Flags().BoolVar(&flagAgentFocusBlocked, "blocked", false, "Focus the most urgent blocked agent instead of naming one")
	agentFocusCmd.Flags().BoolVar(&flagAgentListAll, "all", false, "Span all registered projects (implied outside a git repo)")
	agentFocusCmd.Flags().BoolVar(&flagAgentFocusJSON, "json", false, "Output JSON even on a terminal (direct pane focus only; see Long help)")

	agentCmd.AddCommand(agentReportCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentSummaryCmd)
	agentCmd.AddCommand(agentReadCmd)
	agentCmd.AddCommand(agentSendCmd)
	agentCmd.AddCommand(agentFocusCmd)
	rootCmd.AddCommand(agentCmd)
}

// configuredMultiplexer returns the user's configured multiplexer without the
// TTY gating of chooseMultiplexer: pane reads and agent detection are exactly
// what orchestrating scripts/agents do, and neither steals client focus.
// Degrades to the no-op multiplexer on config problems.
func configuredMultiplexer(exec core.Exec) core.Multiplexer {
	userCfg, err := config.LoadUserConfig()
	if err != nil {
		return adapters.NewNoopMultiplexer()
	}
	mux, err := adapters.NewMultiplexer(userCfg.Multiplexer, exec)
	if err != nil {
		return adapters.NewNoopMultiplexer()
	}
	return mux
}

// newAgentHandler builds the agent handler with pane detection wired in.
func newAgentHandler(deps core.Deps) *agentcmd.Handler {
	mux := configuredMultiplexer(deps.Exec)
	h := agentcmd.NewHandlerWithMux(deps, mux)
	if pane, ok := mux.(core.PaneOps); ok {
		h.SelfPane = pane.CurrentPane()
	}
	return h
}

// paneMultiplexer returns the configured multiplexer's pane operations.
func paneMultiplexer(exec core.Exec) (core.PaneOps, error) {
	mux := configuredMultiplexer(exec)
	pane, ok := mux.(core.PaneOps)
	if !ok {
		return nil, fmt.Errorf("pane operations are not supported by multiplexer %q (tmux and herdr only)", mux.Name())
	}
	return pane, nil
}

func runAgentRead(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	root, state := classifyCwd(ctx)
	if state == cwdNotRepo {
		return fmt.Errorf("not in a git repository")
	}

	deps := newAgentDeps()
	item, err := newAgentHandler(deps).Find(ctx, root, args[0])
	if err != nil {
		return err
	}
	pane, err := paneMultiplexer(deps.Exec)
	if err != nil {
		return err
	}
	content, err := pane.CapturePane(ctx, item.Target())
	if err != nil {
		return err
	}
	fmt.Print(string(content))
	return nil
}

func runAgentSend(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	root, state := classifyCwd(ctx)
	if state == cwdNotRepo {
		return fmt.Errorf("not in a git repository")
	}

	deps := newAgentDeps()
	item, err := newAgentHandler(deps).Find(ctx, root, args[0])
	if err != nil {
		return err
	}
	pane, err := paneMultiplexer(deps.Exec)
	if err != nil {
		return err
	}
	text := strings.Join(args[1:], " ")
	if err := pane.SendText(ctx, item.Target(), text); err != nil {
		return err
	}
	return cli.PrintJSON(map[string]any{
		"sent":   text,
		"piece":  item.Piece,
		"agent":  item.ID,
		"target": item.Target(),
	})
}

// runAgentFocus resolves a target agent (by id/piece, or the most urgent
// blocked one) and moves the client to it: switch-client + pane select when
// its session is still live, else a plain piece switch. This is the one mp
// invocation behind the tmux plugin's agent picker (m a) and blocked-jump
// (m b) chords.
func runAgentFocus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	items, err := collectAgents(cmd)
	if err != nil {
		return err
	}

	if flagAgentFocusBlocked && len(args) == 1 {
		return fmt.Errorf("--blocked and a positional selector are mutually exclusive")
	}

	var item agentcmd.ListItem
	switch {
	case flagAgentFocusBlocked:
		found, ok := agentcmd.FirstBlocked(items)
		if !ok {
			fmt.Fprintln(os.Stderr, cli.GlyphWarn+" no blocked agents")
			return nil
		}
		item = found
	case len(args) == 1:
		found, err := agentcmd.FindInItems(items, args[0])
		if err != nil {
			return err
		}
		item = found
	default:
		return fmt.Errorf("no agent id or piece given; pass one, or use --blocked")
	}

	viaSwitch, err := focusOrSwitchAgent(ctx, item)
	if err != nil {
		return err
	}
	if viaSwitch {
		// runSwitchToExistingPiece already handled all output itself (an
		// attach message, or the worktree path for `cd $(...)`); printing our
		// own JSON on top would double-print onto stdout.
		return nil
	}
	return emitResult(map[string]any{
		"piece":   item.Piece,
		"project": item.Project,
		"agent":   item.ID,
		"target":  item.Target(),
	}, flagAgentFocusJSON)
}

// focusOrSwitchAgent moves the client to item's pane when its session is
// still live and the multiplexer supports pane focus (tmux); otherwise it
// falls back to a plain piece switch — same as `mp switch --piece`, so it
// attaches an existing worktree and never adopts or creates anything new.
// The returned bool is true when the switch fallback ran (it owns all of its
// own output — including its own fixed contract of "attach message on
// stderr, or a bare worktree path on stdout" from mp switch, regardless of
// --json — so the caller must not also emit JSON on top of it).
func focusOrSwitchAgent(ctx context.Context, item agentcmd.ListItem) (bool, error) {
	exec := adapters.NewOSExec()
	mux := configuredMultiplexer(exec)
	if item.SessionName != "" && mux.Exists(ctx, item.SessionName) {
		if pane, err := paneMultiplexer(exec); err == nil {
			if focusErr := pane.FocusPane(ctx, item.SessionName, item.Pane); focusErr != nil {
				// The session may have died in the gap between the Exists
				// check above and this call (TOCTOU) — only treat that as
				// the "session is gone" case and fall through to the switch
				// fallback; any other failure (no client attached to
				// switch, a permissions error) still surfaces as-is rather
				// than being silently swallowed into a different codepath.
				if mux.Exists(ctx, item.SessionName) {
					return false, focusErr
				}
			} else {
				return false, nil
			}
		}
	}
	proj, err := focusProject(ctx, item.Project)
	if err != nil {
		return false, err
	}
	return true, runSwitchToExistingPiece(ctx, proj, item.Piece)
}

// focusProject resolves the project an agent's fallback switch targets:
// registry lookup for a cross-project item, else the repo the caller is
// standing in (mirrors mp switch's own --project defaulting).
func focusProject(ctx context.Context, projectName string) (registry.Project, error) {
	if projectName == "" {
		return resolveSwitchProject(ctx, "")
	}
	reg, err := registry.Load()
	if err != nil {
		return registry.Project{}, err
	}
	proj, ok := reg.Find(projectName)
	if !ok {
		return registry.Project{}, fmt.Errorf("no registered project matching %q", projectName)
	}
	return proj, nil
}

func newAgentDeps() core.Deps {
	return core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupNoopLoading(),
	)
}

func runAgentReport(cmd *cobra.Command, args []string) error {
	if flagAgentSchema {
		schema, err := agentcmd.ReportSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	input, ok, err := getAgentReportInput(cmd)
	if err != nil {
		return err
	}
	if !ok {
		// Claude hook event that doesn't affect status: succeed quietly.
		return cli.PrintJSON(agentcmd.ReportResult{Reported: false})
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := newAgentDeps()
	status, err := newPieceHandler(deps).Status(ctx, wd)
	if err != nil || !status.InPiece {
		// Not in a piece worktree: no-op so globally-installed hooks are safe.
		return cli.PrintJSON(agentcmd.ReportResult{Reported: false})
	}

	result, err := agentcmd.NewHandler(deps).Report(ctx, agentcmd.Location{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     status.RepoRoot,
	}, input)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

// getAgentReportInput resolves report input from the Claude hook payload or
// stdin JSON + flags, then fills id/pid/pane defaults from the environment.
func getAgentReportInput(cmd *cobra.Command) (agentcmd.ReportInput, bool, error) {
	var input agentcmd.ReportInput

	if flagAgentClaudeHook {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return agentcmd.ReportInput{}, false, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, ok, err := agentcmd.FromClaudeHook(data)
		if err != nil || !ok {
			return agentcmd.ReportInput{}, false, err
		}
		return withAgentReportDefaults(cmd, input), true, nil
	}

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return agentcmd.ReportInput{}, false, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = agentcmd.ParseReportJSON(data)
		if err != nil {
			return agentcmd.ReportInput{}, false, err
		}
	}

	if cmd.Flags().Changed("status") {
		input.Status = flagAgentStatus
	}
	if cmd.Flags().Changed("id") {
		input.ID = flagAgentID
	}
	if cmd.Flags().Changed("kind") {
		input.Kind = flagAgentKind
	}
	return withAgentReportDefaults(cmd, input), true, nil
}

// withAgentReportDefaults overlays pid/pane flags and environment defaults.
// The pid default is mp's parent: hook commands run as children of the agent
// (via `sh -c`, which execs single commands), so ppid is the agent itself.
func withAgentReportDefaults(cmd *cobra.Command, input agentcmd.ReportInput) agentcmd.ReportInput {
	if cmd.Flags().Changed("pid") {
		input.PID = flagAgentPID
	}
	if cmd.Flags().Changed("pane") {
		input.Pane = flagAgentPane
	}
	if input.PID == 0 {
		input.PID = os.Getppid()
	}
	if input.Pane == "" {
		if pane, ok := configuredMultiplexer(adapters.NewOSExec()).(core.PaneOps); ok {
			input.Pane = pane.CurrentPane()
		}
	}
	return input
}

func runAgentList(cmd *cobra.Command, args []string) error {
	items, err := collectAgents(cmd)
	if err != nil {
		return err
	}
	// Human table always goes to stderr (like every other command's summary);
	// stdout stays JSON-only.
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "No live agents.")
	} else {
		for _, item := range items {
			kind := item.Kind
			if kind == "" {
				kind = "agent"
			}
			label := item.Piece
			if item.Project != "" {
				label = item.Project + "/" + item.Piece
			}
			fmt.Fprintf(os.Stderr, "%-10s %-28s %s (%s)\n", item.Status, label, kind, item.ID)
		}
	}
	return emitResult(map[string]any{"agents": items}, flagAgentListJSON)
}

func runAgentSummary(cmd *cobra.Command, args []string) error {
	items, err := collectAgents(cmd)
	if err != nil {
		return err
	}
	if summary := agentcmd.Summary(items); summary != "" {
		fmt.Println(summary)
	}
	return nil
}

func collectAgents(cmd *cobra.Command) ([]agentcmd.ListItem, error) {
	ctx := cmd.Context()
	handler := newAgentHandler(newAgentDeps())
	root, state := classifyCwd(ctx)
	// Outside a repo the project scope doesn't exist, so span the registry —
	// this is what makes the status-line segment work from any cwd.
	if flagAgentListAll || state == cwdNotRepo {
		return handler.ListAll(ctx)
	}
	return handler.List(ctx, root)
}
