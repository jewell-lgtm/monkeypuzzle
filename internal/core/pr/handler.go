package pr

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// PRCreateResult contains the result of creating a PR
type PRCreateResult struct {
	PRNumber int    `json:"pr_number"`
	PRURL    string `json:"pr_url"`
	Branch   string `json:"branch"`
}

// Handler executes PR-related commands
type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	github *adapters.GitHub
}

// NewHandler creates a new PR handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:   deps,
		git:    adapters.NewGit(deps.Exec),
		github: adapters.NewGitHub(deps.Exec),
	}
}

// CreatePR creates a GitHub PR for the current piece.
// Must be run from within a piece worktree.
func (h *Handler) CreatePR(workDir string, input Input) (*PRCreateResult, error) {
	// Apply defaults
	input = WithDefaults(input)

	// Check if we're in a piece worktree
	pieceHandler := piece.NewHandler(h.deps)
	status, err := pieceHandler.Status(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return nil, fmt.Errorf("not in a piece worktree - run this command from within a piece")
	}

	// Get current branch
	branch, err := h.git.CurrentBranch(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Try to read issue marker to get title/body defaults
	issueMarker, issuePath := h.readIssueMarker(status.WorktreePath)

	// Use issue title if PR title not provided
	if input.Title == "" && issueMarker != nil {
		input.Title = issueMarker.IssueName
	}

	// Fallback to piece name if still no title
	if input.Title == "" {
		input.Title = status.PieceName
	}

	// Push branch to remote
	h.deps.Output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: fmt.Sprintf("Pushing branch %s to origin...", branch),
	})

	if err := h.github.Push(workDir); err != nil {
		return nil, fmt.Errorf("failed to push branch: %w", err)
	}

	// Create PR
	h.deps.Output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: "Creating PR...",
	})

	prResult, err := h.github.CreatePR(workDir, adapters.PRCreateInput{
		Title: input.Title,
		Body:  input.Body,
		Base:  input.Base,
	})
	if err != nil {
		return nil, err
	}

	// Store PR metadata
	metadata := piece.PRMetadata{
		PRNumber:   prResult.Number,
		PRURL:      prResult.URL,
		Branch:     branch,
		BaseBranch: input.Base,
		CreatedAt:  time.Now(),
		IssuePath:  issuePath,
	}

	if err := piece.WritePRMetadata(status.WorktreePath, metadata, h.deps.FS); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to write PR metadata: %v", err),
		})
	}

	result := &PRCreateResult{
		PRNumber: prResult.Number,
		PRURL:    prResult.URL,
		Branch:   branch,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Created PR #%d: %s", prResult.Number, prResult.URL),
		Data:    result,
	})

	return result, nil
}

// readIssueMarker reads the current issue marker from the piece worktree.
// Returns nil if no marker exists.
func (h *Handler) readIssueMarker(worktreePath string) (*piece.CurrentIssueMarker, string) {
	markerPath := filepath.Join(worktreePath, initcmd.DirName, "current-issue.json")
	data, err := h.deps.FS.ReadFile(markerPath)
	if err != nil {
		return nil, ""
	}

	var marker piece.CurrentIssueMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, ""
	}

	return &marker, marker.IssuePath
}
