package main

import (
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
	RunE: runAgentReport,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List live agents across the project's pieces",
	RunE:  runAgentList,
}

var agentSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Compact one-line agent summary (for status lines)",
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

var (
	flagAgentStatus     string
	flagAgentID         string
	flagAgentKind       string
	flagAgentPID        int
	flagAgentPane       string
	flagAgentClaudeHook bool
	flagAgentSchema     bool
	flagAgentListJSON   bool
)

func init() {
	agentReportCmd.Flags().StringVar(&flagAgentStatus, "status", "", "Agent status: working, blocked, done, idle, or gone")
	agentReportCmd.Flags().StringVar(&flagAgentID, "id", "", "Agent id (defaults to pid-<pid>)")
	agentReportCmd.Flags().StringVar(&flagAgentKind, "kind", "", "Agent kind: claude, codex, ...")
	agentReportCmd.Flags().IntVar(&flagAgentPID, "pid", 0, "Agent process id (defaults to the parent pid)")
	agentReportCmd.Flags().StringVar(&flagAgentPane, "pane", "", "Multiplexer pane the agent runs in (defaults to $TMUX_PANE)")
	agentReportCmd.Flags().BoolVar(&flagAgentClaudeHook, "claude-hook", false, "Parse a Claude Code hook payload from stdin")
	agentReportCmd.Flags().BoolVar(&flagAgentSchema, "schema", false, "Output JSON schema and exit")
	agentListCmd.Flags().BoolVar(&flagAgentListJSON, "json", false, "Output JSON instead of the table")

	agentCmd.AddCommand(agentReportCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentSummaryCmd)
	agentCmd.AddCommand(agentReadCmd)
	agentCmd.AddCommand(agentSendCmd)
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
	return agentcmd.NewHandlerWithMux(deps, configuredMultiplexer(deps.Exec))
}

// paneMultiplexer returns the configured multiplexer's pane operations.
func paneMultiplexer(exec core.Exec) (core.PaneOps, error) {
	mux := configuredMultiplexer(exec)
	pane, ok := mux.(core.PaneOps)
	if !ok {
		return nil, fmt.Errorf("pane operations are not supported by multiplexer %q (tmux only)", mux.Name())
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
		input.Pane = os.Getenv("TMUX_PANE")
	}
	return input
}

func runAgentList(cmd *cobra.Command, args []string) error {
	items, err := collectAgents(cmd)
	if err != nil {
		return err
	}
	if flagAgentListJSON || !cli.IsStdoutTerminal() {
		return cli.PrintJSON(map[string]any{"agents": items})
	}
	if len(items) == 0 {
		fmt.Println("No live agents.")
		return nil
	}
	for _, item := range items {
		kind := item.Kind
		if kind == "" {
			kind = "agent"
		}
		fmt.Printf("%-10s %-24s %s (%s)\n", item.Status, item.Piece, kind, item.ID)
	}
	return nil
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
	root, state := classifyCwd(ctx)
	if state == cwdNotRepo {
		return nil, fmt.Errorf("not in a git repository")
	}
	return newAgentHandler(newAgentDeps()).List(ctx, root)
}
