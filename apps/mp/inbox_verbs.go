package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/inbox"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

const inboxSelectorHelp = `PIECE is "project/piece", or a bare piece name: inside a repo that means
this project's piece first; elsewhere it must be unique across projects.`

var inboxMoveCmd = &cobra.Command{
	Use:   "move [piece] [N]",
	Short: "Re-rank a piece in the inbox",
	Long: `Move a piece to a new rank. Exactly one placement: --top, --bottom, --up [N],
--down [N], --before PIECE or --after PIECE. The first move pins the current
order of every row, so nothing else shifts.

` + inboxSelectorHelp + `

  mp inbox move api/fix-auth --top
  mp inbox move fix-auth --up 2          # two ranks higher
  mp inbox move fix-auth --after nav
  echo '{"piece":"api/fix-auth","bottom":true}' | mp inbox move`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runInboxMove,
}

var inboxNoteCmd = &cobra.Command{
	Use:   "note [piece] [text]",
	Short: "Attach a note to a piece (empty text or --clear removes it)",
	Long: `Set the free-text note the inbox shows next to a piece.

` + inboxSelectorHelp + `

  mp inbox note fix-auth "waiting on review"
  mp inbox note fix-auth --clear
  echo '{"piece":"api/fix-auth","note":"ship it"}' | mp inbox note`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runInboxNote,
}

var inboxSnoozeCmd = &cobra.Command{
	Use:   "snooze [piece]",
	Short: "Drop a piece to the bottom of the inbox until a time",
	Long: `Snooze a piece with --for DURATION ("2h", "2d") or --until RFC3339; --clear
un-snoozes it. Snoozed rows sit at the bottom of every sort and next/prev skip them.

` + inboxSelectorHelp + `

  mp inbox snooze fix-auth --for 2d
  mp inbox snooze api/fix-auth --until 2026-09-10T09:00:00Z
  mp inbox snooze fix-auth --clear`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInboxSnooze,
}

var inboxNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Switch to the piece after the current one in the inbox",
	Long: `Step through the inbox: from the piece you stand in, switch to the next row
(wrapping around; snoozed rows skipped). Outside a piece, next is rank 1.
Inside your multiplexer the session switches; otherwise the worktree path
is printed, as ` + "`mp switch`" + ` does.

  mp inbox next
  mp inbox next --sort urgency
  cd "$(mp inbox next)"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runInboxStep(cmd, inbox.Next) },
}

var inboxPrevCmd = &cobra.Command{
	Use:   "prev",
	Short: "Switch to the piece before the current one in the inbox",
	Long:  `The reverse of ` + "`mp inbox next`" + `; outside a piece, prev is the last row.`,
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return runInboxStep(cmd, inbox.Prev) },
}

var inboxRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "List the inbox with PR state re-fetched (mp inbox --refresh)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		flagInboxRefresh = true
		return runInbox(cmd, args)
	},
}

var (
	flagInboxMove   inbox.MoveInput
	flagInboxNote   inbox.NoteInput
	flagInboxSnooze inbox.SnoozeInput
	flagInboxPiece  string
	flagInboxSchema bool
	flagInboxVerbJS bool
	flagInboxStep   string
)

func init() {
	inboxMoveCmd.Flags().BoolVar(&flagInboxMove.Top, "top", false, "Rank 1")
	inboxMoveCmd.Flags().BoolVar(&flagInboxMove.Bottom, "bottom", false, "Last rank")
	inboxMoveCmd.Flags().IntVar(&flagInboxMove.Up, "up", 0, "N ranks higher (default 1)")
	inboxMoveCmd.Flags().IntVar(&flagInboxMove.Down, "down", 0, "N ranks lower (default 1)")
	inboxMoveCmd.Flags().Lookup("up").NoOptDefVal = "1"
	inboxMoveCmd.Flags().Lookup("down").NoOptDefVal = "1"
	inboxMoveCmd.Flags().StringVar(&flagInboxMove.Before, "before", "", "Place directly above this piece")
	inboxMoveCmd.Flags().StringVar(&flagInboxMove.After, "after", "", "Place directly below this piece")
	inboxNoteCmd.Flags().BoolVar(&flagInboxNote.Clear, "clear", false, "Remove the note")
	inboxSnoozeCmd.Flags().StringVar(&flagInboxSnooze.For, "for", "", "Duration, e.g. 90m, 2h, 2d")
	inboxSnoozeCmd.Flags().StringVar(&flagInboxSnooze.Until, "until", "", "RFC3339 time")
	inboxSnoozeCmd.Flags().BoolVar(&flagInboxSnooze.Clear, "clear", false, "Un-snooze")
	for _, c := range []*cobra.Command{inboxMoveCmd, inboxNoteCmd, inboxSnoozeCmd} {
		c.ValidArgsFunction = completePieceNames
		c.Flags().StringVar(&flagInboxPiece, "piece", "", "Piece selector (alternative to the positional)")
		c.Flags().BoolVar(&flagInboxSchema, "schema", false, "Print an example input document and exit")
		c.Flags().BoolVar(&flagInboxVerbJS, "json", false, "Output JSON even on a terminal")
		_ = c.RegisterFlagCompletionFunc("piece", completePieceNames)
	}
	for _, c := range []*cobra.Command{inboxNextCmd, inboxPrevCmd, inboxRefreshCmd} {
		c.Flags().StringVar(&flagInboxStep, "sort", inbox.SortRank, "Order to step through: rank or urgency")
		c.Flags().BoolVar(&flagInboxVerbJS, "json", false, "Output the switch result as JSON")
		_ = c.RegisterFlagCompletionFunc("sort", cobra.FixedCompletions([]string{inbox.SortRank, inbox.SortUrgency}, cobra.ShellCompDirectiveNoFileComp))
	}
	inboxCmd.AddCommand(inboxMoveCmd, inboxNoteCmd, inboxSnoozeCmd, inboxNextCmd, inboxPrevCmd, inboxRefreshCmd)
}

// inboxVerbInput fills in from stdin JSON (when piped), then lets the
// positional / --piece selector override the piece.
func inboxVerbInput(in any, args []string, setPiece func(string)) error {
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if err := json.Unmarshal(data, in); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}
	selector, err := pieceSelector(args, selectorFlag{"--piece", flagInboxPiece})
	if err != nil {
		return err
	}
	if selector != "" {
		setPiece(selector)
	}
	return nil
}

// inboxVerbHandler builds the inbox handler and the options that scope bare
// selectors to the repo the caller stands in.
func inboxVerbHandler(ctx context.Context) (*inbox.Handler, *piececmd.Handler, inbox.Options) {
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupNoopLoading(),
	)
	opts := inbox.Options{}
	if root, state := classifyCwd(ctx); state == cwdInProject {
		opts.CwdRoot = root
	}
	pieces := newPieceHandler(deps)
	return inbox.NewHandler(deps, pieces), pieces, opts
}

// emitInboxResult prints JSON off a terminal (or with --json); on a terminal
// the human line goes to stderr instead.
func emitInboxResult(v any, human string) error {
	if flagInboxVerbJS || !cli.IsStdoutTerminal() || !cli.IsTerminal() {
		return cli.PrintJSON(v)
	}
	fmt.Fprintln(os.Stderr, human)
	return nil
}

func runInboxMove(cmd *cobra.Command, args []string) error {
	if flagInboxSchema {
		return cli.PrintJSON(inbox.MoveInput{Piece: "api/fix-auth", Up: 2})
	}
	in := flagInboxMove
	// `--up 3` parses as --up=1 plus a positional 3 (pflag optional values),
	// so a second positional is the step count.
	if len(args) == 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 {
			return fmt.Errorf("step count must be a positive integer, got %q", args[1])
		}
		switch {
		case in.Up != 0:
			in.Up = n
		case in.Down != 0:
			in.Down = n
		default:
			return fmt.Errorf("a step count needs --up or --down")
		}
		args = args[:1]
	}
	if err := inboxVerbInput(&in, args, func(s string) { in.Piece = s }); err != nil {
		return err
	}
	h, _, opts := inboxVerbHandler(cmd.Context())
	res, err := h.Move(cmd.Context(), opts, in)
	if err != nil {
		return err
	}
	return emitInboxResult(res, fmt.Sprintf("%s %s: #%d → #%d", cli.GlyphOK, res.Key, res.FromRank, res.Rank))
}

func runInboxNote(cmd *cobra.Command, args []string) error {
	if flagInboxSchema {
		return cli.PrintJSON(inbox.NoteInput{Piece: "api/fix-auth", Note: "waiting on review"})
	}
	in := flagInboxNote
	if len(args) == 2 {
		in.Note = args[1]
		args = args[:1]
	}
	if err := inboxVerbInput(&in, args, func(s string) { in.Piece = s }); err != nil {
		return err
	}
	h, _, opts := inboxVerbHandler(cmd.Context())
	res, err := h.Note(cmd.Context(), opts, in)
	if err != nil {
		return err
	}
	human := fmt.Sprintf("%s %s: note cleared", cli.GlyphOK, res.Key)
	if res.Note != "" {
		human = fmt.Sprintf("%s %s: %s", cli.GlyphOK, res.Key, res.Note)
	}
	return emitInboxResult(res, human)
}

func runInboxSnooze(cmd *cobra.Command, args []string) error {
	if flagInboxSchema {
		return cli.PrintJSON(inbox.SnoozeInput{Piece: "api/fix-auth", For: "2d"})
	}
	in := flagInboxSnooze
	if err := inboxVerbInput(&in, args, func(s string) { in.Piece = s }); err != nil {
		return err
	}
	h, _, opts := inboxVerbHandler(cmd.Context())
	res, err := h.Snooze(cmd.Context(), opts, in)
	if err != nil {
		return err
	}
	human := fmt.Sprintf("%s %s: snooze cleared", cli.GlyphOK, res.Key)
	if res.SnoozedUntil != nil {
		human = fmt.Sprintf("%s %s: snoozed until %s", cli.GlyphOK, res.Key, res.SnoozedUntil.Local().Format(time.RFC3339))
	}
	return emitInboxResult(res, human)
}

// runInboxStep is `mp inbox next` / `prev`: pick the neighbouring row and
// hand it to the piece handler's SwitchPiece — the same switch (multiplexer
// switch-client, else the path) `mp switch`, `mp create` and `mp adopt` run.
// SwitchPiece resolves the repo from cwd, hence the temporary chdir.
func runInboxStep(cmd *cobra.Command, dir int) error {
	if flagInboxStep != inbox.SortRank && flagInboxStep != inbox.SortUrgency {
		return fmt.Errorf("--sort must be %s or %s", inbox.SortRank, inbox.SortUrgency)
	}
	ctx := cmd.Context()
	h, pieces, opts := inboxVerbHandler(ctx)
	opts.Sort = flagInboxStep
	rows, err := h.List(ctx, opts)
	if err != nil {
		return err
	}
	current, err := currentPieceWorktree(ctx, pieces)
	if err != nil {
		return err
	}
	target, ok := inbox.Step(rows, current, dir, time.Now())
	if !ok {
		return inbox.ErrEmpty
	}
	if current != "" && filepath.Clean(target.WorktreePath) == filepath.Clean(current) {
		fmt.Fprintf(os.Stderr, "%s is the only piece in the inbox; staying put\n", target.Key)
		return nil
	}
	restore, err := tempChdir(target.WorktreePath)
	if err != nil {
		return err
	}
	defer restore()
	res, err := pieces.SwitchPiece(ctx, target.Piece)
	if err != nil {
		return err
	}
	if flagInboxVerbJS {
		if res.Method == "path" {
			noteCwd(res.Piece.WorktreePath)
		}
		return cli.PrintJSON(res)
	}
	if res.Method == "path" {
		surfacePath(res.Piece.WorktreePath)
	}
	return nil
}

// currentPieceWorktree is the symlink-resolved worktree of the piece the
// caller stands in, or "" outside any piece.
func currentPieceWorktree(ctx context.Context, pieces *piececmd.Handler) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	status, err := pieces.Status(ctx, wd)
	if err != nil || !status.InPiece {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(status.WorktreePath); err == nil {
		return resolved, nil
	}
	return status.WorktreePath, nil
}
