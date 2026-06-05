package mp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/workflow"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// --- mp issue advance ---------------------------------------------------------

type advanceInput struct {
	ID string `json:"id"`
}

var (
	flagAdvanceID     string
	flagAdvanceSchema bool
)

var issueAdvanceCmd = &cobra.Command{
	Use:   "advance",
	Short: "Fire the next manual workflow event for an issue",
	Long: `Advance an issue by firing its single outbound manual event from the current
workflow state. If the state has more than one outbound manual event, advance
refuses and tells you to use 'mp issue fire' with an explicit --event.`,
	RunE: runIssueAdvance,
}

func init() {
	issueAdvanceCmd.Flags().StringVar(&flagAdvanceID, "id", "", "Issue ID (provider-specific)")
	issueAdvanceCmd.Flags().BoolVar(&flagAdvanceSchema, "schema", false, "Output JSON schema and exit")
	issueCmd.AddCommand(issueAdvanceCmd)
}

func runIssueAdvance(cmd *cobra.Command, args []string) error {
	if flagAdvanceSchema {
		return cli.PrintJSON(advanceInput{})
	}
	in, err := readSimpleIssueInput(flagAdvanceID)
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	wd, deps, wf, ih, currentStatus, err := loadIssueWorkflow(ctx, in.ID)
	if err != nil {
		return err
	}
	_ = wd

	// `advance` walks the progress axis. The cancel event (abandoned) lives
	// on the orthogonal cancel axis — reachable via `mp issue abandon`. Filter
	// it out so we don't conflate "next step" with "kill it".
	manuals := filterOutCancel(wf.OutboundManualEvents(currentStatus))
	switch len(manuals) {
	case 0:
		return fmt.Errorf("no progress events available from state %q", currentStatus)
	case 1:
		return applyEvent(ctx, deps, wf, ih, in.ID, currentStatus, manuals[0])
	default:
		return fmt.Errorf("multiple progress events available from state %q (%s); use `mp issue fire --event <name>`",
			currentStatus, strings.Join(manuals, ", "))
	}
}

func filterOutCancel(events []string) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		if e == workflow.EventAbandoned {
			continue
		}
		out = append(out, e)
	}
	return out
}

// --- mp issue fire ------------------------------------------------------------

type fireInput struct {
	ID    string `json:"id"`
	Event string `json:"event"`
}

var (
	flagFireID     string
	flagFireEvent  string
	flagFireSchema bool
)

var issueFireCmd = &cobra.Command{
	Use:   "fire",
	Short: "Fire a named workflow event for an issue",
	Long: `Fire an explicit event by name. Useful when a state has multiple outbound
events, or when scripting a transition that mp can't observe automatically.`,
	RunE: runIssueFire,
}

func init() {
	issueFireCmd.Flags().StringVar(&flagFireID, "id", "", "Issue ID (provider-specific)")
	issueFireCmd.Flags().StringVar(&flagFireEvent, "event", "", "Event name to fire (e.g. acceptance.passed, released)")
	issueFireCmd.Flags().BoolVar(&flagFireSchema, "schema", false, "Output JSON schema and exit")
	issueCmd.AddCommand(issueFireCmd)
}

func runIssueFire(cmd *cobra.Command, args []string) error {
	if flagFireSchema {
		return cli.PrintJSON(fireInput{})
	}
	in, err := readFireInput()
	if err != nil {
		return err
	}
	if in.Event == "" {
		return fmt.Errorf("event is required")
	}
	ctx := cmd.Context()

	_, deps, wf, ih, currentStatus, err := loadIssueWorkflow(ctx, in.ID)
	if err != nil {
		return err
	}
	return applyEvent(ctx, deps, wf, ih, in.ID, currentStatus, in.Event)
}

// --- mp issue abandon ---------------------------------------------------------

type abandonInput struct {
	ID    string `json:"id"`
	Force bool   `json:"force,omitempty"`
}

var (
	flagAbandonIssueID     string
	flagAbandonIssueForce  bool
	flagAbandonIssueSchema bool
)

var issueAbandonCmd = &cobra.Command{
	Use:   "abandon",
	Short: "Move an issue to the cancel state",
	Long: `Fire the workflow's abandoned event for the given issue. Moves it to the
workflow's cancel state (cancelled, by default). Distinct from 'done' — the
issue did not ship.`,
	RunE: runIssueAbandon,
}

func init() {
	issueAbandonCmd.Flags().StringVar(&flagAbandonIssueID, "id", "", "Issue ID")
	issueAbandonCmd.Flags().BoolVar(&flagAbandonIssueForce, "force", false, "Skip pre-flight checks")
	issueAbandonCmd.Flags().BoolVar(&flagAbandonIssueSchema, "schema", false, "Output JSON schema and exit")
	issueCmd.AddCommand(issueAbandonCmd)
}

func runIssueAbandon(cmd *cobra.Command, args []string) error {
	if flagAbandonIssueSchema {
		return cli.PrintJSON(abandonInput{})
	}
	in, err := readAbandonInput()
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	_, deps, wf, ih, currentStatus, err := loadIssueWorkflow(ctx, in.ID)
	if err != nil {
		return err
	}
	_ = in.Force // Reserved: future pre-flight (e.g. refuse if open PR).
	return applyEvent(ctx, deps, wf, ih, in.ID, currentStatus, workflow.EventAbandoned)
}

// --- mp issue reopen ----------------------------------------------------------

type reopenInput struct {
	ID string `json:"id"`
	To string `json:"to"`
}

var (
	flagReopenIssueID     string
	flagReopenTo          string
	flagReopenIssueSchema bool
)

var issueReopenCmd = &cobra.Command{
	Use:   "reopen",
	Short: "Move an issue out of the cancel state",
	Long: `Reopen a cancelled issue to a named workflow state. Reopen is a loud,
direct write — it bypasses the transition graph because the cancel state has
no outbound edges.`,
	RunE: runIssueReopen,
}

func init() {
	issueReopenCmd.Flags().StringVar(&flagReopenIssueID, "id", "", "Issue ID")
	issueReopenCmd.Flags().StringVar(&flagReopenTo, "to", "", "Target workflow state name")
	issueReopenCmd.Flags().BoolVar(&flagReopenIssueSchema, "schema", false, "Output JSON schema and exit")
	issueCmd.AddCommand(issueReopenCmd)
}

func runIssueReopen(cmd *cobra.Command, args []string) error {
	if flagReopenIssueSchema {
		return cli.PrintJSON(reopenInput{})
	}
	in, err := readReopenInput()
	if err != nil {
		return err
	}
	if in.To == "" {
		return fmt.Errorf("to is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	deps := newCLIDeps()
	wf, err := workflow.LoadForRepo(wd, deps.FS)
	if err != nil {
		return err
	}
	if !wf.HasState(in.To) {
		return fmt.Errorf("%q is not a workflow state for this project", in.To)
	}
	ih := issue.NewHandler(deps, wd)
	if err := ih.SyncStatus(in.ID, in.To); err != nil {
		return err
	}
	return cli.PrintJSON(map[string]any{"id": in.ID, "status": in.To})
}

// --- shared helpers -----------------------------------------------------------

func newCLIDeps() core.Deps {
	return core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
}

// loadIssueWorkflow resolves the project's workflow + the issue's current
// status. Returns (workDir, deps, workflow, issueHandler, currentStatus).
func loadIssueWorkflow(_ context.Context, issueID string) (string, core.Deps, workflow.Workflow, *issue.Handler, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", core.Deps{}, workflow.Workflow{}, nil, "", fmt.Errorf("failed to get working directory: %w", err)
	}
	deps := newCLIDeps()
	wf, err := workflow.LoadForRepo(wd, deps.FS)
	if err != nil {
		return "", core.Deps{}, workflow.Workflow{}, nil, "", err
	}
	ih := issue.NewHandler(deps, wd)
	current, err := currentStatusByID(ih, issueID)
	if err != nil {
		return "", core.Deps{}, workflow.Workflow{}, nil, "", err
	}
	return wd, deps, wf, ih, current, nil
}

func currentStatusByID(ih *issue.Handler, issueID string) (string, error) {
	if issueID == "" {
		return "", fmt.Errorf("id is required")
	}
	items, err := ih.SearchIssues(issue.SearchInput{Limit: 0})
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ID == issueID || strings.HasSuffix(it.ID, "/"+issueID) || it.Path == issueID {
			return it.Status, nil
		}
	}
	return "", fmt.Errorf("issue %q not found", issueID)
}

// applyEvent walks the workflow for (current, event) and writes the resulting
// state to the provider. Reports a no-op when the event doesn't fire from the
// current state — informational, not an error.
func applyEvent(_ context.Context, _ core.Deps, wf workflow.Workflow, ih *issue.Handler, issueID, current, event string) error {
	result, ok := wf.Fire(current, event)
	if !ok {
		fmt.Fprintf(os.Stderr, "Event %q does not fire from state %q; nothing changed.\n", event, current)
		return cli.PrintJSON(map[string]any{"id": issueID, "status": current, "event": event, "fired": false})
	}
	// Resolve the canonical issue ID by re-reading the matching issue (the
	// caller may have passed a path-like alias). Use the first SearchIssues
	// hit; currentStatusByID already validated existence.
	id, err := resolveCanonicalIssueID(ih, issueID)
	if err != nil {
		return err
	}
	if err := ih.SyncStatus(id, result.To); err != nil {
		return err
	}
	return cli.PrintJSON(map[string]any{
		"id":     id,
		"status": result.To,
		"event":  event,
		"from":   result.From,
		"fired":  true,
	})
}

func resolveCanonicalIssueID(ih *issue.Handler, issueID string) (string, error) {
	items, err := ih.SearchIssues(issue.SearchInput{Limit: 0})
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ID == issueID || strings.HasSuffix(it.ID, "/"+issueID) || it.Path == issueID {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("issue %q not found", issueID)
}

// readSimpleIssueInput reads an {id: string} input from flag or stdin.
func readSimpleIssueInput(flagID string) (advanceInput, error) {
	if flagID != "" {
		return advanceInput{ID: flagID}, nil
	}
	if !cli.HasStdinData() {
		return advanceInput{}, fmt.Errorf("id is required (use --id or JSON stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return advanceInput{}, err
	}
	var in advanceInput
	if err := json.Unmarshal(data, &in); err != nil {
		return advanceInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

func readFireInput() (fireInput, error) {
	if flagFireID != "" || flagFireEvent != "" {
		return fireInput{ID: flagFireID, Event: flagFireEvent}, nil
	}
	if !cli.HasStdinData() {
		return fireInput{}, fmt.Errorf("id and event are required (use --id/--event or JSON stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fireInput{}, err
	}
	var in fireInput
	if err := json.Unmarshal(data, &in); err != nil {
		return fireInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

func readAbandonInput() (abandonInput, error) {
	if flagAbandonIssueID != "" {
		return abandonInput{ID: flagAbandonIssueID, Force: flagAbandonIssueForce}, nil
	}
	if !cli.HasStdinData() {
		return abandonInput{}, fmt.Errorf("id is required (use --id or JSON stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return abandonInput{}, err
	}
	var in abandonInput
	if err := json.Unmarshal(data, &in); err != nil {
		return abandonInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

func readReopenInput() (reopenInput, error) {
	if flagReopenIssueID != "" || flagReopenTo != "" {
		return reopenInput{ID: flagReopenIssueID, To: flagReopenTo}, nil
	}
	if !cli.HasStdinData() {
		return reopenInput{}, fmt.Errorf("id and to are required (use --id/--to or JSON stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return reopenInput{}, err
	}
	var in reopenInput
	if err := json.Unmarshal(data, &in); err != nil {
		return reopenInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}
