package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const (
	claudeSettingsDir  = ".claude"
	claudeSettingsFile = "settings.json"

	// reportCommand is installed for every event; the payload's
	// hook_event_name drives the status mapping in mp. $PPID inside the
	// `sh -c` wrapper is the shell's parent — the claude process itself —
	// giving liveness reaping the right pid.
	reportCommand = "mp agent report --claude-hook --pid $PPID"

	DefaultDirPerm  = 0755
	DefaultFilePerm = 0644
)

// claudeEvents are the Claude Code hook events mp derives agent status from.
// PreToolUse/PostToolUse matter for un-sticking "blocked": after a permission
// prompt (Notification → blocked) is approved in the pane, the next tool
// event is what flips the status back to working. They fire per tool call;
// the report is a fast local upsert, so the overhead is fine.
var claudeEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Notification", "Stop", "SessionEnd"}

// Result is the output of an integration install.
type Result struct {
	Tool    string   `json:"tool"`
	Path    string   `json:"path"`
	Events  []string `json:"events"`
	Updated bool     `json:"updated"`
}

// Handler installs agent-status integrations for supported tools.
type Handler struct {
	deps core.Deps
}

// NewHandler creates an integration handler.
func NewHandler(deps core.Deps) *Handler {
	return &Handler{deps: deps}
}

// InstallClaude merges mp's agent-report hooks into the repo's
// .claude/settings.json, preserving everything already there. Idempotent: an
// event that already carries the report command is left untouched.
func (h *Handler) InstallClaude(repoRoot string) (Result, error) {
	settingsPath := filepath.Join(repoRoot, claudeSettingsDir, claudeSettingsFile)

	settings := map[string]any{}
	if data, err := h.deps.FS.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return Result{}, fmt.Errorf("existing %s is not valid JSON: %w", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return Result{}, err
	}

	hooks := map[string]any{}
	if raw, exists := settings["hooks"]; exists {
		var ok bool
		if hooks, ok = raw.(map[string]any); !ok {
			// Never silently replace config we don't understand.
			return Result{}, fmt.Errorf("%s: \"hooks\" is not an object; fix it and re-run", settingsPath)
		}
	} else {
		settings["hooks"] = hooks
	}

	updated := false
	for _, event := range claudeEvents {
		var groups []any
		if raw, exists := hooks[event]; exists {
			var ok bool
			if groups, ok = raw.([]any); !ok {
				return Result{}, fmt.Errorf("%s: hooks.%s is not an array; fix it and re-run", settingsPath, event)
			}
		}
		if hasReportCommand(groups) {
			continue
		}
		hooks[event] = append(groups, map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": reportCommand},
			},
		})
		updated = true
	}

	if updated {
		if err := h.deps.FS.MkdirAll(filepath.Dir(settingsPath), DefaultDirPerm); err != nil {
			return Result{}, err
		}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return Result{}, err
		}
		if err := h.deps.FS.WriteFile(settingsPath, append(data, '\n'), DefaultFilePerm); err != nil {
			return Result{}, err
		}
		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: "Installed mp agent hooks in " + settingsPath,
		})
	} else {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgInfo,
			Content: "mp agent hooks already installed in " + settingsPath,
		})
	}

	return Result{
		Tool:    "claude",
		Path:    settingsPath,
		Events:  claudeEvents,
		Updated: updated,
	}, nil
}

// hasReportCommand reports whether any hook group in the event already runs
// the mp agent report command.
func hasReportCommand(groups []any) bool {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		cmds, _ := group["hooks"].([]any)
		for _, c := range cmds {
			cmd, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if command, _ := cmd["command"].(string); command == reportCommand {
				return true
			}
		}
	}
	return false
}
