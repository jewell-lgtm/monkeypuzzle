package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/inbox"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Your pieces across every project, in your order",
	Long: `List every piece across all registered projects as one ordered inbox.

Rows you have ranked come first, in your order; the rest follow by urgency
(blocked > review > working > idle > merged), newest first. Snoozed rows sit
at the bottom. --sort urgency puts urgency ahead of your order instead.

State file: $MP_CONFIG_DIR/inbox.json (default ~/.config/monkeypuzzle).
PR state is fetched once per project and cached for two minutes; --refresh
bypasses the cache. Works from any directory.

  mp inbox                      # table on a terminal
  mp inbox --json | jq .rows    # {"rows":[…]} for pickers and dashboards
  mp inbox --sort urgency`,
	Args: cobra.NoArgs,
	RunE: runInbox,
}

var (
	flagInboxSort    string
	flagInboxRefresh bool
	flagInboxJSON    bool
)

func init() {
	inboxCmd.Flags().StringVar(&flagInboxSort, "sort", inbox.SortRank, "Order: rank (your order, urgency breaks ties) or urgency")
	inboxCmd.Flags().BoolVar(&flagInboxRefresh, "refresh", false, "Re-fetch PR state instead of using the cache")
	inboxCmd.Flags().BoolVar(&flagInboxJSON, "json", false, "Output JSON even on a terminal")
	rootCmd.AddCommand(inboxCmd)

	_ = inboxCmd.RegisterFlagCompletionFunc("sort", cobra.FixedCompletions([]string{inbox.SortRank, inbox.SortUrgency}, cobra.ShellCompDirectiveNoFileComp))
}

func runInbox(cmd *cobra.Command, args []string) error {
	if flagInboxSort != inbox.SortRank && flagInboxSort != inbox.SortUrgency {
		return fmt.Errorf("--sort must be %s or %s", inbox.SortRank, inbox.SortUrgency)
	}
	ctx := cmd.Context()
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupNoopLoading(),
	)
	opts := inbox.Options{Sort: flagInboxSort, Refresh: flagInboxRefresh}
	// An init'd repo that was never registered still shows up when you stand
	// in it — the inbox is where you find out it's missing from the fleet.
	if root, state := classifyCwd(ctx); state == cwdInProject {
		opts.CwdRoot = root
	}
	rows, err := inbox.NewHandler(deps, newPieceHandler(deps)).List(ctx, opts)
	if err != nil {
		return err
	}
	// Human table is a stderr message like every other verb's summary; stdout
	// stays JSON-only so `mp inbox | jq` never sees a table.
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagInboxJSON {
		renderInboxTable(rows, time.Now())
		return nil
	}
	return cli.PrintJSON(map[string]any{"rows": rows})
}

// urgencyGlyph pairs each urgency with the shared status glyphs so the inbox
// reads like every other verb's summary.
func urgencyGlyph(urgency string) string {
	switch urgency {
	case inbox.UrgencyBlocked:
		return cli.GlyphWarn + " blocked"
	case inbox.UrgencyReview:
		return cli.GlyphInfo + " review"
	case inbox.UrgencyMerged:
		return cli.GlyphOK + " merged"
	default:
		return "  " + urgency
	}
}

func prCell(r inbox.Row) string {
	switch {
	case r.PR == nil:
		return ""
	case r.Merged:
		return "merged"
	case r.PR.Draft:
		return "#" + strconv.Itoa(r.PR.Number) + " draft"
	default:
		return "#" + strconv.Itoa(r.PR.Number) + " " + r.PR.State
	}
}

func truncateNote(note string, max int) string {
	r := []rune(note)
	if len(r) <= max {
		return note
	}
	return string(r[:max-1]) + "…"
}

func renderInboxTable(rows []inbox.Row, now time.Time) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "Inbox empty: no pieces in any registered project.")
		return
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "#\tURGENCY\tPIECE\tAGENT\tPR\tNOTE")
	for _, r := range rows {
		// Snoozed rows keep their rank but sit at the bottom under a zz marker.
		rank := strconv.Itoa(r.Rank)
		if r.Snoozed(now) {
			rank = "zz"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", rank, urgencyGlyph(r.Urgency), r.Key, r.AgentStatus, prCell(r), truncateNote(r.Note, 40))
	}
	_ = w.Flush()
}
