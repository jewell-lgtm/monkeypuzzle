package pr

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// PRCreateResult contains the result of creating a PR
type PRCreateResult struct {
	PRNumber int    `json:"pr_number"`
	PRURL    string `json:"pr_url"`
	Branch   string `json:"branch"`
}

// Handler executes PR-related commands
type Handler struct {
	deps  core.Deps
	git   *adapters.Git
	hooks *piece.HookRunner
}

// NewHandler creates a new PR handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:  deps,
		git:   adapters.NewGit(deps.Exec),
		hooks: piece.NewHookRunner(deps),
	}
}

// providerForRepo loads the configured PR provider for the given repo root.
func (h *Handler) providerForRepo(repoRoot string) (Provider, string, error) {
	cfg, err := piece.ReadConfig(repoRoot, h.deps.FS)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read config (run mp init first): %w", err)
	}
	providerType := cfg.PR.Provider
	if providerType == "" {
		providerType = "github"
	}
	p, err := NewProvider(ProviderConfig{
		ProviderType: providerType,
		Config:       cfg.PR.Config,
		Deps:         ProviderDeps{Exec: h.deps.Exec},
	})
	if err != nil {
		return nil, "", err
	}
	return p, providerType, nil
}

// CreatePR creates a PR/MR for the current piece via the configured provider.
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
		return nil, piece.ErrNotInPiece
	}

	provider, _, err := h.providerForRepo(status.RepoRoot)
	if err != nil {
		return nil, err
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

	// Default the PR title to the piece name when not provided.
	if input.Title == "" {
		input.Title = status.PieceName
	}

	// Build base hook context (PR fields filled in once we have the result)
	hookCtx := piece.HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     status.RepoRoot,
		PRBaseBranch: input.Base,
	}

	// before-pr-create hook (e.g. to write a description file)
	if err := h.hooks.RunHook(ctx, status.RepoRoot, piece.HookBeforePRCreate, hookCtx); err != nil {
		return nil, fmt.Errorf("before-pr-create hook failed: %w", err)
	}

	// If the hook (or the user) wrote a body file, prefer it over input.Body.
	// Convention: <worktree>/.monkeypuzzle/pr-body.txt. Hook responsibility to clean up.
	bodyPath := filepath.Join(status.WorktreePath, projectdir.DefaultDirName, "pr-body.txt")
	if data, err := h.deps.FS.ReadFile(bodyPath); err == nil {
		input.Body = string(data)
	}

	// Push branch to remote
	if err := provider.Push(ctx, workDir); err != nil {
		return nil, fmt.Errorf("failed to push branch: %w", err)
	}
	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("pushed %s → origin", branch),
	})

	prResult, err := provider.Create(ctx, workDir, CreateInput(input))
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
		Draft:      input.Draft,
	}

	if err := piece.WritePRMetadata(status.WorktreePath, metadata, h.deps.FS); err != nil {
		return nil, fmt.Errorf("failed to write PR metadata: %w", err)
	}

	result := &PRCreateResult{
		PRNumber: prResult.Number,
		PRURL:    prResult.URL,
		Branch:   branch,
	}

	kind := ""
	if input.Draft {
		kind = "draft "
	}
	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("opened %s#%d  %s → %s", kind, prResult.Number, branch, input.Base),
		Data:    result,
	})
	h.deps.Output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: "  " + prResult.URL,
	})

	// after-pr-create hook
	hookCtx.PRNumber = prResult.Number
	hookCtx.PRURL = prResult.URL
	if err := h.hooks.RunHook(ctx, status.RepoRoot, piece.HookAfterPRCreate, hookCtx); err != nil {
		// Non-fatal: PR is already created. Report and continue.
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("after-pr-create hook failed: %v", err),
		})
	}

	return result, nil
}

// MarkReady flips a draft PR/MR for the current piece to ready-for-review.
// Fires before-pr-ready / after-pr-ready hooks around the provider call.
func (h *Handler) MarkReady(ctx context.Context, workDir string) error {
	pieceHandler := piece.NewHandler(h.deps)
	status, err := pieceHandler.Status(ctx, workDir)
	if err != nil {
		return fmt.Errorf("failed to get piece status: %w", err)
	}
	if !status.InPiece {
		return piece.ErrNotInPiece
	}

	metadata, err := piece.ReadPRMetadata(status.WorktreePath, h.deps.FS)
	if err != nil {
		return fmt.Errorf("no PR metadata found; run `mp pr create` first: %w", err)
	}
	if metadata.PRNumber == 0 {
		return fmt.Errorf("PR metadata has no number")
	}

	provider, _, err := h.providerForRepo(status.RepoRoot)
	if err != nil {
		return err
	}

	hookCtx := piece.HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     status.RepoRoot,
		PRNumber:     metadata.PRNumber,
		PRURL:        metadata.PRURL,
		PRBaseBranch: metadata.BaseBranch,
	}

	if err := h.hooks.RunHook(ctx, status.RepoRoot, piece.HookBeforePRReady, hookCtx); err != nil {
		return fmt.Errorf("before-pr-ready hook failed: %w", err)
	}

	if err := provider.MarkReady(ctx, workDir, metadata.PRNumber); err != nil {
		return fmt.Errorf("failed to mark PR ready: %w", err)
	}

	// Keep the locally-mirrored draft flag in step with the forge. Best-effort:
	// the flip itself already succeeded.
	if metadata.Draft {
		metadata.Draft = false
		if err := piece.WritePRMetadata(status.WorktreePath, *metadata, h.deps.FS); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("failed to update PR metadata: %v", err),
			})
		}
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("#%d ready for review", metadata.PRNumber),
	})
	h.deps.Output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: "  " + metadata.PRURL,
	})

	if err := h.hooks.RunHook(ctx, status.RepoRoot, piece.HookAfterPRReady, hookCtx); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("after-pr-ready hook failed: %v", err),
		})
	}

	return nil
}
