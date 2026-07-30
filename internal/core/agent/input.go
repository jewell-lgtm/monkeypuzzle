package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// ReportInput holds input for `mp agent report`. ID/Kind/PID/Pane default at
// the CLI edge (ppid, $TMUX_PANE) so hooks can call it with just --status.
type ReportInput struct {
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status"`
	PID    int    `json:"pid,omitempty"`
	Pane   string `json:"pane,omitempty"`
}

// StatusGone removes the agent's record instead of updating it (clean exit).
const StatusGone = "gone"

// ReportSchema returns the JSON schema for agent report input.
func ReportSchema() ([]byte, error) {
	schema := map[string]any{
		"id":     "",
		"kind":   "",
		"status": strings.Join([]string{piece.AgentWorking, piece.AgentBlocked, piece.AgentDone, piece.AgentIdle, StatusGone}, "|"),
		"pid":    0,
		"pane":   "",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseReportJSON parses JSON input into ReportInput.
func ParseReportJSON(data []byte) (ReportInput, error) {
	var input ReportInput
	if err := json.Unmarshal(data, &input); err != nil {
		return ReportInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidateReportInput checks the status is one of the known values.
func ValidateReportInput(input ReportInput) error {
	switch input.Status {
	case piece.AgentWorking, piece.AgentBlocked, piece.AgentDone, piece.AgentIdle, StatusGone:
		return nil
	case "":
		return fmt.Errorf("status is required")
	default:
		return fmt.Errorf("invalid status %q (valid: %s, %s, %s, %s, %s)",
			input.Status, piece.AgentWorking, piece.AgentBlocked, piece.AgentDone, piece.AgentIdle, StatusGone)
	}
}

// ReportResult is the output of an agent report.
type ReportResult struct {
	// Reported is false when the caller was not inside a piece worktree (a
	// silent no-op so integration hooks are safe to install globally).
	Reported bool   `json:"reported"`
	Piece    string `json:"piece,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	Status   string `json:"status,omitempty"`
	// Aggregate is the piece-level status after this report.
	Aggregate string `json:"aggregate,omitempty"`
}

// ListItem is one agent in `mp agent list` output.
type ListItem struct {
	// Project is set in cross-project listings (`mp agent list --all`).
	Project     string    `json:"project,omitempty"`
	Piece       string    `json:"piece"`
	SessionName string    `json:"session_name"`
	ID          string    `json:"id"`
	Kind        string    `json:"kind,omitempty"`
	Status      string    `json:"status"`
	PID         int       `json:"pid,omitempty"`
	Pane        string    `json:"pane,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// claudeHookPayload is the subset of Claude Code's hook stdin JSON we use.
type claudeHookPayload struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
}

// FromClaudeHook maps a Claude Code hook payload (delivered on the hook's
// stdin) to a ReportInput. ok is false for events that don't affect status.
func FromClaudeHook(data []byte) (input ReportInput, ok bool, err error) {
	var payload claudeHookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ReportInput{}, false, fmt.Errorf("invalid claude hook payload: %w", err)
	}

	status := ""
	switch payload.HookEventName {
	case "SessionStart":
		status = piece.AgentIdle
	case "UserPromptSubmit", "PreToolUse", "PostToolUse":
		status = piece.AgentWorking
	case "Notification":
		// Permission prompts and waiting-for-input reminders both arrive here.
		status = piece.AgentBlocked
	case "Stop":
		status = piece.AgentDone
	case "SessionEnd":
		status = StatusGone
	default:
		return ReportInput{}, false, nil
	}

	return ReportInput{
		ID:     payload.SessionID,
		Kind:   "claude",
		Status: status,
	}, true, nil
}
