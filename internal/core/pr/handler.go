package pr

import (
	"context"
	"fmt"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
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
// Expects input to be pre-validated via WithDefaults() and Validate().
func (h *Handler) CreatePR(ctx context.Context, workDir string, input Input) (*PRCreateResult, error) {
	// Check if we're in a piece worktree
	pieceHandler := piece.NewHandler(h.deps)
	status, err := pieceHandler.Status(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return nil, fmt.Errorf("not in a piece worktree - run this command from within a piece")
	}

	// Auto-detect base branch from piece metadata if not explicitly provided
	if input.Base == "" {
		pieceMetadata, err := piece.ReadPieceMetadata(status.WorktreePath, h.deps.FS)
		if err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to read piece metadata, defaulting to main: %v", err),
			})
			input.Base = "main"
		} else {
			input.Base = pieceMetadata.Parent
			if pieceMetadata.Parent != "main" {
				h.deps.Output.Write(core.Message{
					Type:    core.MsgInfo,
					Content: fmt.Sprintf("Using parent piece '%s' as PR base", pieceMetadata.Parent),
				})
			}
		}
	}

	// Get current branch
	branch, err := h.git.CurrentBranch(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Try to read the issue ref recorded in piece metadata for title defaults.
	issueRef := h.readIssueRef(status.WorktreePath)

	// Use issue title if PR title not provided
	if input.Title == "" && !issueRef.IsEmpty() {
		input.Title = issueRef.Title
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

	if err := h.github.Push(ctx, workDir); err != nil {
		return nil, fmt.Errorf("failed to push branch: %w", err)
	}

	// Create PR
	h.deps.Output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: "Creating PR...",
	})

	prResult, err := h.github.CreatePR(ctx, workDir, adapters.PRCreateInput{
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
		Issue:      issueRef,
	}

	if err := piece.WritePRMetadata(status.WorktreePath, metadata, h.deps.FS); err != nil {
		return nil, fmt.Errorf("failed to write PR metadata: %w", err)
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

// readIssueRef reads the issue ref recorded in the piece's metadata.
// Returns an empty ref if none is recorded or metadata can't be read.
func (h *Handler) readIssueRef(worktreePath string) piece.IssueRef {
	meta, err := piece.ReadPieceMetadata(worktreePath, h.deps.FS)
	if err != nil || meta == nil {
		return piece.IssueRef{}
	}
	return meta.Issue
}
