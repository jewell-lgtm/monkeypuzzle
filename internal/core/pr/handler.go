package pr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	branchcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/branch"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// PRCreateResult contains the result of creating a PR
type PRCreateResult struct {
	PRNumber int    `json:"pr_number"`
	PRURL    string `json:"pr_url"`
	Branch   string `json:"branch"`
}

// ManagedInfo is a PR association recorded on an mp branch layer. The forge
// remains authoritative for live state; this local record is the mp atom that
// workflows can compose without discovering unrelated repository PRs.
type ManagedInfo struct {
	Number  int    `json:"number"`
	URL     string `json:"url,omitempty"`
	Branch  string `json:"branch"`
	Base    string `json:"base"`
	Piece   string `json:"piece"`
	Status  string `json:"status,omitempty"`
	Current bool   `json:"current,omitempty"`
}

// List returns PRs recorded against mp-managed branch layers.
func (h *Handler) List(ctx context.Context, workDir string) ([]ManagedInfo, error) {
	branches, err := branchcmd.NewHandler(h.deps).List(ctx, workDir)
	if err != nil {
		return nil, err
	}
	rows := make([]ManagedInfo, 0)
	for _, branch := range branches {
		if branch.PRNumber != 0 {
			rows = append(rows, ManagedInfo{Number: branch.PRNumber, URL: branch.PRURL, Branch: branch.Name, Base: branch.Base, Piece: branch.Piece, Status: branch.Status, Current: branch.Current})
			continue
		}
		legacy, legacyErr := piece.ReadPRMetadata(branch.Worktree, h.deps.FS)
		if legacyErr != nil {
			if errors.Is(legacyErr, os.ErrNotExist) {
				continue
			}
			return nil, legacyErr
		}
		// A legacy record predates stacks, so it names no branch. It belongs to
		// the piece's initial layer; attaching it to every layer would report
		// the same PR once per layer.
		if legacy.PRNumber != 0 && (legacy.Branch == branch.Name || (legacy.Branch == "" && branch.Initial)) {
			rows = append(rows, ManagedInfo{Number: legacy.PRNumber, URL: legacy.PRURL, Branch: branch.Name, Base: legacy.BaseBranch, Piece: branch.Piece, Status: "OPEN", Current: branch.Current})
		}
	}
	return rows, nil
}

// Show resolves a recorded PR by number or branch; an empty selector defaults
// to the current managed branch.
func (h *Handler) Show(ctx context.Context, workDir, selector string) (ManagedInfo, error) {
	rows, err := h.List(ctx, workDir)
	if err != nil {
		return ManagedInfo{}, err
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		for _, row := range rows {
			if row.Current {
				return row, nil
			}
		}
		return ManagedInfo{}, fmt.Errorf("the current managed branch has no recorded PR")
	}
	number, _ := strconv.Atoi(strings.TrimPrefix(selector, "#"))
	for _, row := range rows {
		if row.Branch == selector || (number != 0 && row.Number == number) {
			return row, nil
		}
	}
	return ManagedInfo{}, fmt.Errorf("no mp-managed PR matches %q", selector)
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

	// The active branch identifies the stack layer being shipped.
	branch, err := h.git.CurrentBranch(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Auto-detect the base from the current stack entry. V1 pieces without a
	// stack retain their parent-based behavior.
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
			for _, entry := range pieceMetadata.Stack {
				if entry.Branch == branch {
					input.Base = entry.Base
					break
				}
			}
			if input.Base == "" {
				input.Base = "main"
			}
			if input.Base != "main" {
				h.deps.Output.Write(core.Message{
					Type:    core.MsgInfo,
					Content: fmt.Sprintf("Using stack base '%s' for branch '%s'", input.Base, branch),
				})
			}
		}
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
		Branch:       branch,
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
	if pieceMetadata, err := piece.ReadPieceMetadata(status.WorktreePath, h.deps.FS); err == nil {
		recorded := false
		for i := range pieceMetadata.Stack {
			if pieceMetadata.Stack[i].Branch == branch {
				pieceMetadata.Stack[i].PRNumber = prResult.Number
				pieceMetadata.Stack[i].PRURL = prResult.URL
				pieceMetadata.Stack[i].Status = "OPEN"
				if err := piece.WritePieceMetadata(status.WorktreePath, *pieceMetadata, h.deps.FS); err != nil {
					return nil, fmt.Errorf("failed to record PR in piece stack: %w", err)
				}
				recorded = true
				break
			}
		}
		if !recorded && len(pieceMetadata.Stack) == 0 {
			pieceMetadata.Stack = []piece.StackEntry{{Branch: branch, Base: input.Base, PRNumber: prResult.Number, PRURL: prResult.URL, Status: "OPEN"}}
			if err := piece.WritePieceMetadata(status.WorktreePath, *pieceMetadata, h.deps.FS); err != nil {
				return nil, fmt.Errorf("failed to bootstrap piece stack with PR: %w", err)
			}
		}
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
		Branch:       metadata.Branch,
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
