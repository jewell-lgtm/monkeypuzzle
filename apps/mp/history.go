package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show what mp did, across every project",
	Long: `Read the append-only history log: one line per lifecycle event (piece
created/switched/merged/done, PR created/ready, agent blocked/done, stack
synced) across every repository on this machine.

Log file: $MP_HISTORY_FILE, else ${XDG_STATE_HOME:-~/.local/state}/monkeypuzzle/history.jsonl.

  mp history                        # last 50 events
  mp history --project mp --since 24h
  mp history --event 'pr.*' -n 10
  mp history --piece login --json | jq .`,
	Args: cobra.NoArgs,
	RunE: runHistory,
}

var (
	flagHistoryProject string
	flagHistoryPiece   string
	flagHistoryEvent   string
	flagHistorySince   time.Duration
	flagHistoryLimit   int
	flagHistoryJSON    bool
)

func init() {
	historyCmd.Flags().StringVar(&flagHistoryProject, "project", "", "Only events for this project name")
	historyCmd.Flags().StringVar(&flagHistoryPiece, "piece", "", "Only events for this piece")
	historyCmd.Flags().StringVar(&flagHistoryEvent, "event", "", "Only this event; trailing * matches a prefix (pr.*)")
	historyCmd.Flags().DurationVar(&flagHistorySince, "since", 0, "Only events newer than this (Go duration, e.g. 24h)")
	historyCmd.Flags().IntVarP(&flagHistoryLimit, "limit", "n", 50, "Show the last N matching events (0 = all)")
	historyCmd.Flags().BoolVar(&flagHistoryJSON, "json", false, "Emit JSON lines on stdout even on a terminal")
	rootCmd.AddCommand(historyCmd)

	_ = historyCmd.RegisterFlagCompletionFunc("since", cobra.NoFileCompletions)
}

func runHistory(cmd *cobra.Command, args []string) error {
	opts := history.ReadOptions{
		Project: flagHistoryProject,
		Piece:   flagHistoryPiece,
		Event:   flagHistoryEvent,
		Limit:   flagHistoryLimit,
	}
	if flagHistorySince > 0 {
		opts.Since = time.Now().Add(-flagHistorySince)
	}
	events, err := history.Read(opts)
	if err != nil {
		return err
	}

	// Human table is a stderr message like every other verb's summary; stdout
	// stays JSON-only so `mp history | jq` never sees a table.
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagHistoryJSON {
		renderHistoryTable(events)
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("failed to encode event: %w", err)
		}
	}
	return nil
}

func renderHistoryTable(events []history.Event) {
	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "No history yet.")
		return
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WHEN\tEVENT\tWHERE\tACTOR\tDETAILS")
	for _, ev := range events {
		when := ev.TS
		if t := ev.Time(); !t.IsZero() {
			when = t.Local().Format("2006-01-02 15:04")
		}
		where := ev.Project
		if ev.Piece != "" {
			where += "/" + ev.Piece
		}
		actor := ev.Actor.Kind
		if ev.Actor.ID != "" {
			actor += ":" + ev.Actor.ID
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", when, ev.Event, where, actor, summarizeData(ev))
	}
	_ = w.Flush()
}

// summarizeData renders branch/parent plus data as one "k=v" line, keys sorted.
func summarizeData(ev history.Event) string {
	var parts []string
	if ev.Branch != "" && ev.Branch != ev.Piece {
		parts = append(parts, "branch="+ev.Branch)
	}
	if ev.Parent != "" && ev.Parent != "main" {
		parts = append(parts, "parent="+ev.Parent)
	}
	keys := make([]string, 0, len(ev.Data))
	for k := range ev.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := ev.Data[k]
		var s string
		switch x := v.(type) {
		case nil:
			continue
		case []any:
			items := make([]string, 0, len(x))
			for _, it := range x {
				items = append(items, fmt.Sprint(it))
			}
			s = strings.Join(items, ",")
		default:
			s = fmt.Sprint(x) // ints round-trip through JSON as float64; %v prints 7 not 7.0
		}
		if s == "" {
			continue
		}
		parts = append(parts, k+"="+s)
	}
	return strings.Join(parts, " ")
}
