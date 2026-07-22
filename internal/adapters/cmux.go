package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// CmuxMultiplexer implements core.Multiplexer for cmux
// (https://github.com/manaflow-ai/cmux), mapping each piece to a workspace.
//
// cmux targets workspaces by UUID/ref/index, not name, so every operation
// resolves the session name against `cmux workspace list --json` first: mp
// names the workspaces it creates via --name, which lands in custom_title.
// The list is scoped to the current window — a piece workspace opened in a
// different cmux window won't be found and would be recreated here.
type CmuxMultiplexer struct {
	exec core.Exec
}

// NewCmuxMultiplexer creates a CmuxMultiplexer.
func NewCmuxMultiplexer(exec core.Exec) *CmuxMultiplexer {
	return &CmuxMultiplexer{exec: exec}
}

type cmuxWorkspaceList struct {
	Workspaces []struct {
		Ref         string  `json:"ref"`
		CustomTitle *string `json:"custom_title"`
	} `json:"workspaces"`
}

// lookupRef resolves a session name to a workspace ref ("workspace:N").
// Returns "" when no workspace carries that custom title.
func (c *CmuxMultiplexer) lookupRef(ctx context.Context, sessionName string) (string, error) {
	out, err := c.exec.Run(ctx, "cmux", "workspace", "list", "--json")
	if err != nil {
		return "", fmt.Errorf("failed to list cmux workspaces: %w", err)
	}
	var list cmuxWorkspaceList
	if err := json.Unmarshal(out, &list); err != nil {
		return "", fmt.Errorf("failed to parse cmux workspace list: %w", err)
	}
	for _, w := range list.Workspaces {
		if w.CustomTitle != nil && *w.CustomTitle == sessionName {
			return w.Ref, nil
		}
	}
	return "", nil
}

// SwitchTo selects (or creates) the workspace named sessionName. Creation
// focuses the new workspace, so no separate select is needed.
func (c *CmuxMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	ref, err := c.lookupRef(ctx, sessionName)
	if err != nil {
		return err
	}
	if ref == "" {
		// --focus is explicit: the default is undocumented and SwitchTo must
		// always land the user in the new workspace.
		_, err := c.exec.Run(ctx, "cmux", "workspace", "create", "--name", sessionName, "--cwd", workDir, "--focus", "true")
		if err != nil {
			return fmt.Errorf("failed to create cmux workspace: %w", err)
		}
		return nil
	}
	if _, err := c.exec.Run(ctx, "cmux", "select-workspace", "--workspace", ref); err != nil {
		return fmt.Errorf("failed to select cmux workspace: %w", err)
	}
	return nil
}

// Kill closes the workspace named sessionName. A missing workspace is not an
// error (idempotent kill).
func (c *CmuxMultiplexer) Kill(ctx context.Context, sessionName string) error {
	ref, err := c.lookupRef(ctx, sessionName)
	if err != nil {
		return err
	}
	if ref == "" {
		return nil
	}
	if _, err := c.exec.Run(ctx, "cmux", "close-workspace", "--workspace", ref); err != nil {
		return fmt.Errorf("failed to close cmux workspace: %w", err)
	}
	return nil
}

// Exists checks if a workspace named sessionName exists in the current window.
func (c *CmuxMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	ref, err := c.lookupRef(ctx, sessionName)
	return err == nil && ref != ""
}

// InSession returns true if inside a cmux workspace.
func (c *CmuxMultiplexer) InSession() bool {
	return os.Getenv("CMUX_WORKSPACE_ID") != ""
}

// IsInstalled returns true if cmux is available.
func (c *CmuxMultiplexer) IsInstalled(ctx context.Context) bool {
	_, err := c.exec.Run(ctx, "which", "cmux")
	return err == nil
}

// Name returns "cmux".
func (c *CmuxMultiplexer) Name() string {
	return "cmux"
}
