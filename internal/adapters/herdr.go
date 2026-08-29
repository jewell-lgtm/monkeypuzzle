package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// HerdrMultiplexer implements core.Multiplexer and core.PaneOps for herdr
// (https://herdr.dev), mapping each piece to a workspace — "one workspace per
// repo, task, or investigation" is herdr's own model, so no translation is
// involved. herdr targets workspaces by id, not label, so every operation
// resolves the session name against `herdr workspace list --json` first: mp
// names the workspaces it creates via --label.
//
// Everything goes through the herdr CLI, which mirrors the socket API 1:1 and
// resolves the right server socket from $HERDR_SESSION itself — keeping the
// adapter symmetric with its tmux/zellij/cmux siblings and testable with
// MockExec.
type HerdrMultiplexer struct {
	exec core.Exec
}

// NewHerdrMultiplexer creates a HerdrMultiplexer.
func NewHerdrMultiplexer(exec core.Exec) *HerdrMultiplexer {
	return &HerdrMultiplexer{exec: exec}
}

type herdrWorkspaceList struct {
	Workspaces []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"workspaces"`
}

type herdrPane struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	PID     int    `json:"pid"`
	Focused bool   `json:"focused"`
}

type herdrPaneList struct {
	Panes []herdrPane `json:"panes"`
}

// lookupWorkspace resolves a session name to a workspace id. Returns "" when
// no workspace carries that label. Matching is exact: "mp/dearest" must not
// match "mp/dearest-mobileapp".
func (h *HerdrMultiplexer) lookupWorkspace(ctx context.Context, sessionName string) (string, error) {
	out, err := h.exec.Run(ctx, "herdr", "workspace", "list", "--json")
	if err != nil {
		return "", fmt.Errorf("failed to list herdr workspaces: %w", err)
	}
	var list herdrWorkspaceList
	if err := json.Unmarshal(out, &list); err != nil {
		return "", fmt.Errorf("failed to parse herdr workspace list: %w", err)
	}
	for _, w := range list.Workspaces {
		if w.Label == sessionName {
			return w.ID, nil
		}
	}
	return "", nil
}

// SwitchTo focuses (or creates) the workspace named sessionName. Focus is
// explicit even on the create path: whether create focuses the new workspace
// is not contractual, and SwitchTo must always land the user in it.
func (h *HerdrMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	id, err := h.lookupWorkspace(ctx, sessionName)
	if err != nil {
		return err
	}
	if id == "" {
		if _, err := h.exec.Run(ctx, "herdr", "workspace", "create", "--cwd", workDir, "--label", sessionName); err != nil {
			return fmt.Errorf("failed to create herdr workspace: %w", err)
		}
		if id, err = h.lookupWorkspace(ctx, sessionName); err != nil {
			return err
		}
		if id == "" {
			return fmt.Errorf("herdr workspace %q not listed after create", sessionName)
		}
	}
	if _, err := h.exec.Run(ctx, "herdr", "workspace", "focus", id); err != nil {
		return fmt.Errorf("failed to focus herdr workspace: %w", err)
	}
	return nil
}

// Kill closes the workspace named sessionName. A missing workspace is not an
// error (idempotent kill). close targets an explicit workspace id and must
// not move the client's focus: the piece handler switches to main *before*
// killing, and a close that dragged focus into the dying workspace would
// defeat that invariant (the zellij close-tab bug all over again).
func (h *HerdrMultiplexer) Kill(ctx context.Context, sessionName string) error {
	id, err := h.lookupWorkspace(ctx, sessionName)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	if _, err := h.exec.Run(ctx, "herdr", "workspace", "close", id); err != nil {
		return fmt.Errorf("failed to close herdr workspace: %w", err)
	}
	return nil
}

// Exists checks if a workspace named sessionName exists.
func (h *HerdrMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	id, err := h.lookupWorkspace(ctx, sessionName)
	return err == nil && id != ""
}

// InSession returns true if inside a herdr-managed pane.
func (h *HerdrMultiplexer) InSession() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

// IsInstalled returns true if herdr is available.
func (h *HerdrMultiplexer) IsInstalled(ctx context.Context) bool {
	_, err := h.exec.Run(ctx, "which", "herdr")
	return err == nil
}

// Name returns "herdr".
func (h *HerdrMultiplexer) Name() string {
	return "herdr"
}

// listPanes enumerates the panes of a workspace by id.
func (h *HerdrMultiplexer) listPanes(ctx context.Context, workspaceID string) ([]herdrPane, error) {
	out, err := h.exec.Run(ctx, "herdr", "pane", "list", workspaceID, "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to list herdr panes: %w", err)
	}
	var list herdrPaneList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("failed to parse herdr pane list: %w", err)
	}
	return list.Panes, nil
}

// resolvePaneTarget maps a PaneOps target to a herdr pane id. Pane ids
// ("w1:p1") always contain a colon and session names never do —
// session.Sanitize strips ':' — so a colon means the target already is a
// pane; a session name resolves to its workspace's focused pane (first pane
// when herdr reports none focused, e.g. the workspace isn't the active one).
func (h *HerdrMultiplexer) resolvePaneTarget(ctx context.Context, target string) (string, error) {
	if strings.Contains(target, ":") {
		return target, nil
	}
	id, err := h.lookupWorkspace(ctx, target)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("no herdr workspace named %q", target)
	}
	panes, err := h.listPanes(ctx, id)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Focused {
			return p.ID, nil
		}
	}
	if len(panes) > 0 {
		return panes[0].ID, nil
	}
	return "", fmt.Errorf("no panes in herdr workspace %q", target)
}

// SendText types text into the target pane followed by Enter. The text goes
// via send-text (literal, no key-combo parsing; "--" keeps leading-dash text
// from parsing as flags) and Enter via a second send-keys call so it is
// interpreted as the key, not the word — same split as the tmux adapter.
func (h *HerdrMultiplexer) SendText(ctx context.Context, target, text string) error {
	pane, err := h.resolvePaneTarget(ctx, target)
	if err != nil {
		return err
	}
	if _, err := h.exec.Run(ctx, "herdr", "pane", "send-text", pane, "--", text); err != nil {
		return fmt.Errorf("failed to send text to pane: %w", err)
	}
	if _, err := h.exec.Run(ctx, "herdr", "pane", "send-keys", pane, "enter"); err != nil {
		return fmt.Errorf("failed to send Enter to pane: %w", err)
	}
	return nil
}

// CapturePane returns the visible contents of the target pane.
func (h *HerdrMultiplexer) CapturePane(ctx context.Context, target string) ([]byte, error) {
	pane, err := h.resolvePaneTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	out, err := h.exec.Run(ctx, "herdr", "pane", "read", pane, "--source", "visible")
	if err != nil {
		return nil, fmt.Errorf("failed to read pane: %w", err)
	}
	return out, nil
}

// ListPanes enumerates every pane in a workspace.
func (h *HerdrMultiplexer) ListPanes(ctx context.Context, sessionName string) ([]core.PaneInfo, error) {
	id, err := h.lookupWorkspace(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, fmt.Errorf("no herdr workspace named %q", sessionName)
	}
	panes, err := h.listPanes(ctx, id)
	if err != nil {
		return nil, err
	}
	var infos []core.PaneInfo
	for _, p := range panes {
		infos = append(infos, core.PaneInfo{ID: p.ID, Command: p.Command, PID: p.PID})
	}
	return infos, nil
}

// FocusPane focuses the workspace named sessionName, then (if pane is given)
// the pane itself. Best-effort past the workspace focus: a pane focus failure
// (e.g. a pane that closed) is not fatal — the client still lands in the
// right workspace.
func (h *HerdrMultiplexer) FocusPane(ctx context.Context, sessionName, pane string) error {
	id, err := h.lookupWorkspace(ctx, sessionName)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("no herdr workspace named %q", sessionName)
	}
	if _, err := h.exec.Run(ctx, "herdr", "workspace", "focus", id); err != nil {
		return fmt.Errorf("failed to focus herdr workspace: %w", err)
	}
	if pane == "" {
		return nil
	}
	_, _ = h.exec.Run(ctx, "herdr", "pane", "focus", pane)
	return nil
}

// CurrentPane returns the herdr pane mp itself runs in, "" outside herdr.
func (h *HerdrMultiplexer) CurrentPane() string {
	return os.Getenv("HERDR_PANE_ID")
}

type herdrAgentList struct {
	Agents []struct {
		Pane  string `json:"pane"`
		Agent string `json:"agent"`
		State string `json:"state"`
		PID   int    `json:"pid"`
	} `json:"agents"`
}

// ObserveAgents returns herdr's natively-tracked agents for one workspace.
// `herdr agent list` is server-wide; entries are scoped by pane-id prefix — a
// workspace's panes are "<ws>:pN", so no separate scoping flag is needed. A
// missing workspace observes as empty, not an error (mirrors detection
// against a dead session).
func (h *HerdrMultiplexer) ObserveAgents(ctx context.Context, sessionName string) ([]core.AgentObservation, error) {
	id, err := h.lookupWorkspace(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	out, err := h.exec.Run(ctx, "herdr", "agent", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to list herdr agents: %w", err)
	}
	var list herdrAgentList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("failed to parse herdr agent list: %w", err)
	}
	var observations []core.AgentObservation
	for _, a := range list.Agents {
		if !strings.HasPrefix(a.Pane, id+":") {
			continue
		}
		observations = append(observations, core.AgentObservation{
			Pane:   a.Pane,
			Kind:   strings.ToLower(a.Agent),
			Status: mapHerdrAgentState(a.State),
			PID:    a.PID,
		})
	}
	return observations, nil
}

// mapHerdrAgentState maps herdr's agent states onto mp's status vocabulary.
// The four shared states pass through; "unknown" (and anything herdr grows
// later) maps to idle — a false blocked/working trains people to ignore the
// status, the same strictness mp's own detection borrows from herdr.
func mapHerdrAgentState(state string) string {
	switch state {
	case "working", "blocked", "done", "idle":
		return state
	default:
		return "idle"
	}
}
