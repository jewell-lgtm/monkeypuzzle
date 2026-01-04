package piece

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

const (
	symlinkName = ".monkeypuzzle-source"

	// DefaultDirPerm is the default permission for directories (0755 = rwxr-xr-x)
	DefaultDirPerm = 0755
)

// Handler executes piece-related commands
type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	github *adapters.GitHub
	tmux   *adapters.Tmux
	hooks  *HookRunner
}

// NewHandler creates a new piece handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:   deps,
		git:    adapters.NewGit(deps.Exec),
		github: adapters.NewGitHub(deps.Exec),
		tmux:   adapters.NewTmux(deps.Exec),
		hooks:  NewHookRunner(deps),
	}
}

// CreatePieceOptions configures piece creation behavior
type CreatePieceOptions struct {
	OverwriteSession bool // If true, replace existing main repo session
}

// CreatePieceWithInput creates a piece from validated input.
// Routes to CreatePieceFromIssue if IssuePath is set, otherwise CreatePiece.
func (h *Handler) CreatePieceWithInput(ctx context.Context, srcDir string, input NewPieceInput, opts CreatePieceOptions) (PieceInfo, error) {
	if input.IssuePath != "" {
		return h.CreatePieceFromIssue(ctx, srcDir, input.IssuePath, opts)
	}
	return h.CreatePiece(ctx, srcDir, input.Name, opts)
}

// CreatePiece creates a new git worktree with tmux session.
// If pieceName is provided and non-empty, it will be used (after checking it doesn't exist).
// If pieceName is empty, a name will be generated automatically.
func (h *Handler) CreatePiece(ctx context.Context, monkeypuzzleSourceDir string, pieceName string, opts CreatePieceOptions) (PieceInfo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect git repo root
	repoRoot, err := h.git.RepoRoot(ctx, wd)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
	}

	// Ensure main repo tmux session exists
	h.ensureMainSession(ctx, repoRoot, opts.OverwriteSession)

	// Get pieces directory
	piecesDir, err := getPiecesDir()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	// Use provided name or generate one
	if pieceName == "" {
		var err error
		pieceName, err = h.GeneratePieceName(piecesDir)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("failed to generate piece name: %w", err)
		}
	} else {
		// Sanitize the provided name (convert spaces to hyphens, lowercase, etc.)
		pieceName = SanitizePieceName(pieceName)
		// Validate that the sanitized name doesn't already exist
		piecePath := filepath.Join(piecesDir, pieceName)
		_, err := h.deps.FS.Stat(piecePath)
		if err == nil {
			return PieceInfo{}, fmt.Errorf("piece name %q already exists at %s", pieceName, piecePath)
		}
	}

	// Create pieces directory if it doesn't exist
	if err := h.deps.FS.MkdirAll(piecesDir, DefaultDirPerm); err != nil {
		return PieceInfo{}, fmt.Errorf("failed to create pieces directory at %s: %w", piecesDir, err)
	}

	// Create worktree
	worktreePath := filepath.Join(piecesDir, pieceName)
	if err := h.git.WorktreeAdd(ctx, repoRoot, worktreePath); err != nil {
		return PieceInfo{}, fmt.Errorf("failed to create worktree at %s: %w", worktreePath, err)
	}

	// Note: Currently, symlink and tmux creation failures are non-fatal (logged as warnings).
	// If we decide to make them fatal in the future, we should add cleanup logic here to
	// remove the worktree if those operations fail. The WorktreeRemove method is available
	// in the Git adapter for this purpose.

	// Create symlink to monkeypuzzle source
	symlinkPath := filepath.Join(worktreePath, symlinkName)
	if err := h.deps.FS.Symlink(monkeypuzzleSourceDir, symlinkPath); err != nil {
		// If symlink creation fails, log but don't fail the operation
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to create symlink: %v", err),
		})
	}

	// Create tmux session
	sessionName := fmt.Sprintf("mp-piece-%s", pieceName)
	tmuxCreated := false
	if err := h.tmux.NewSession(ctx, sessionName, worktreePath); err != nil {
		// If tmux fails, log but don't fail the operation
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to create tmux session: %v", err),
		})
	} else {
		tmuxCreated = true
	}

	info := PieceInfo{
		Name:         pieceName,
		WorktreePath: worktreePath,
		SessionName:  sessionName,
	}

	// Run on-piece-create hook
	hookCtx := HookContext{
		PieceName:    pieceName,
		WorktreePath: worktreePath,
		RepoRoot:     repoRoot,
		SessionName:  sessionName,
	}
	if err := h.hooks.RunHook(ctx, repoRoot, HookOnPieceCreate, hookCtx); err != nil {
		// Cleanup: remove worktree and tmux session on hook failure
		h.cleanupPiece(ctx, repoRoot, worktreePath, sessionName, tmuxCreated)
		return PieceInfo{}, fmt.Errorf("on-piece-create hook failed: %w", err)
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Created piece: %s at %s", pieceName, worktreePath),
		Data:    info,
	})

	return info, nil
}

// CurrentIssueMarker represents the current issue marker file structure
type CurrentIssueMarker struct {
	IssuePath string `json:"issue_path"` // Relative path from repo root
	IssueName string `json:"issue_name"` // Display name from issue
	PieceName string `json:"piece_name"` // Sanitized piece name
}

// CreatePieceFromIssue creates a new piece from a markdown issue file.
// It extracts the issue name, sanitizes it for use as a piece name, creates the piece,
// and writes a marker file in the worktree to track the current issue.
func (h *Handler) CreatePieceFromIssue(ctx context.Context, monkeypuzzleSourceDir, issuePath string, opts CreatePieceOptions) (PieceInfo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect git repo root
	repoRoot, err := h.git.RepoRoot(ctx, wd)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
	}

	// Read monkeypuzzle config to find issues directory
	cfg, err := ReadConfig(repoRoot, h.deps.FS)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to read monkeypuzzle config: %w", err)
	}

	// Validate issue provider is markdown
	if cfg.Issues.Provider != "markdown" {
		return PieceInfo{}, fmt.Errorf("issue provider must be 'markdown', got: %s", cfg.Issues.Provider)
	}

	// Get and validate issues directory from config
	issuesDir, ok := cfg.Issues.Config["directory"]
	if !ok || issuesDir == "" {
		return PieceInfo{}, fmt.Errorf("issues directory not found in config")
	}

	// Resolve issue path (absolute or relative to repo root)
	// ResolveIssuePath already verifies the file exists
	absIssuePath, err := ResolveIssuePath(repoRoot, issuePath, h.deps.FS)
	if err != nil {
		return PieceInfo{}, err
	}

	// Validate that the issue file is within the configured issues directory
	// This prevents path traversal and ensures issues are in the correct location
	absIssuesDir := filepath.Join(repoRoot, issuesDir)
	absIssuesDir = filepath.Clean(absIssuesDir)
	relPath, err := filepath.Rel(absIssuesDir, absIssuePath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return PieceInfo{}, fmt.Errorf("issue file must be within the issues directory %q, got: %s", issuesDir, issuePath)
	}

	// Extract issue name
	issueName, err := ExtractIssueName(absIssuePath, h.deps.FS)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to extract issue name: %w", err)
	}

	// Sanitize issue name for piece name
	pieceName := SanitizePieceName(issueName)

	// Create the piece using the sanitized name
	info, err := h.CreatePiece(ctx, monkeypuzzleSourceDir, pieceName, opts)
	if err != nil {
		return PieceInfo{}, err
	}

	// Calculate relative issue path from repo root
	// Note: filepath.Rel can fail on Windows if paths are on different drives
	relIssuePath, err := filepath.Rel(repoRoot, absIssuePath)
	if err != nil {
		// If we can't compute relative path (e.g., different drives on Windows),
		// use the original path provided by the user
		relIssuePath = issuePath
	}

	// Write current issue marker file in worktree
	marker := CurrentIssueMarker{
		IssuePath: relIssuePath,
		IssueName: issueName,
		PieceName: pieceName,
	}
	if err := h.writeCurrentIssueMarker(info.WorktreePath, marker); err != nil {
		// Log warning but don't fail the operation
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to write current issue marker: %v", err),
		})
	}

	// Update issue status to in-progress (non-fatal)
	h.updateIssueStatusToInProgress(absIssuePath)

	return info, nil
}

// writeCurrentIssueMarker writes the current issue marker file to the worktree.
func (h *Handler) writeCurrentIssueMarker(worktreePath string, marker CurrentIssueMarker) error {
	// Create .monkeypuzzle directory in worktree if it doesn't exist
	mpDir := filepath.Join(worktreePath, initcmd.DirName)
	if err := h.deps.FS.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create .monkeypuzzle directory: %w", err)
	}

	// Write marker file
	markerPath := filepath.Join(mpDir, "current-issue.json")
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal marker: %w", err)
	}

	if err := h.deps.FS.WriteFile(markerPath, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write marker file: %w", err)
	}

	return nil
}

// updateIssueStatusToInProgress updates the issue status to in-progress if it's currently todo.
// Logs a warning on failure but doesn't fail the piece creation.
func (h *Handler) updateIssueStatusToInProgress(issuePath string) {
	// Check current status
	currentStatus, err := ParseStatus(issuePath, h.deps.FS)
	if err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to read issue status: %v", err),
		})
		return
	}

	// Only update if status is todo
	if currentStatus != StatusTodo {
		return
	}

	// Update to in-progress
	if err := UpdateStatus(issuePath, StatusInProgress, h.deps.FS); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to update issue status: %v", err),
		})
	}
}

// cleanupPiece removes a partially created piece (worktree and tmux session).
// Errors during cleanup are logged as warnings but not returned.
func (h *Handler) cleanupPiece(ctx context.Context, repoRoot, worktreePath, sessionName string, tmuxCreated bool) {
	// Kill tmux session if it was created
	if tmuxCreated {
		if err := h.tmux.KillSession(ctx, sessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to cleanup tmux session: %v", err),
			})
		}
	}

	// Remove worktree
	if err := h.git.WorktreeRemove(ctx, repoRoot, worktreePath); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to cleanup worktree: %v", err),
		})
	}
}

// Status detects if we're currently in a piece worktree or main repo
func (h *Handler) Status(ctx context.Context, workDir string) (PieceStatus, error) {
	gitDir, err := h.git.RevParseGitDir(ctx, workDir)
	if err != nil {
		// Not in a git repo
		return PieceStatus{
			InPiece: false,
		}, nil
	}

	isWorktree := h.git.IsWorktree(gitDir)
	if !isWorktree {
		// In main repo
		repoRoot, err := h.git.RepoRoot(ctx, workDir)
		if err != nil {
			// If we can't get repo root, leave it empty
			repoRoot = ""
		}
		return PieceStatus{
			InPiece:  false,
			RepoRoot: repoRoot,
		}, nil
	}

	// In worktree - extract piece name from path
	worktreePath, err := h.git.RepoRoot(ctx, workDir)
	if err != nil {
		// Fallback: use workDir if we can't get worktree path
		worktreePath = workDir
	}
	pieceName := filepath.Base(worktreePath)

	// Get main repo root from worktree
	repoRoot, err := h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		// If we can't get main repo root, leave it empty
		repoRoot = ""
	}

	return PieceStatus{
		InPiece:      true,
		PieceName:    pieceName,
		WorktreePath: worktreePath,
		RepoRoot:     repoRoot,
	}, nil
}

// GeneratePieceName generates a unique piece name with timestamp and counter
func (h *Handler) GeneratePieceName(baseDir string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	baseName := fmt.Sprintf("piece-%s", timestamp)

	// Check for existing pieces and increment counter if needed
	counter := 0
	for {
		pieceName := baseName
		if counter > 0 {
			pieceName = fmt.Sprintf("%s-%d", baseName, counter)
		}

		piecePath := filepath.Join(baseDir, pieceName)
		_, err := h.deps.FS.Stat(piecePath)
		if err != nil {
			// Path doesn't exist, we can use this name
			return pieceName, nil
		}

		counter++
		// Safety limit to avoid infinite loop
		if counter > 1000 {
			return "", fmt.Errorf("too many pieces with similar names")
		}
	}
}

// UpdatePiece merges the main branch into the current piece's history
func (h *Handler) UpdatePiece(ctx context.Context, workDir, mainBranch string) (UpdateResult, error) {
	// Check if we're in a piece worktree
	status, err := h.Status(ctx, workDir)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return UpdateResult{}, fmt.Errorf("not in a piece worktree")
	}

	// Get current branch to verify we're on a branch
	_, err = h.git.CurrentBranch(ctx, workDir)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Build hook context
	hookCtx := HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     status.RepoRoot,
		MainBranch:   mainBranch,
	}

	// Run before-piece-update hook
	if err := h.hooks.RunHook(ctx, status.RepoRoot, HookBeforePieceUpdate, hookCtx); err != nil {
		return UpdateResult{}, fmt.Errorf("before-piece-update hook failed: %w", err)
	}

	// Merge the main branch
	if err := h.git.Merge(ctx, workDir, mainBranch); err != nil {
		return UpdateResult{}, err
	}

	// Run after-piece-update hook
	if err := h.hooks.RunHook(ctx, status.RepoRoot, HookAfterPieceUpdate, hookCtx); err != nil {
		return UpdateResult{}, fmt.Errorf("after-piece-update hook failed: %w", err)
	}

	result := UpdateResult{
		PieceName:  status.PieceName,
		MainBranch: mainBranch,
		Status:     "updated",
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Merged %s into %s", mainBranch, status.PieceName),
		Data:    result,
	})

	return result, nil
}

// MergePiece squash-merges the piece branch back into main as a single commit.
// Fails if main has commits that are not in the piece worktree.
func (h *Handler) MergePiece(ctx context.Context, workDir, mainBranch string) (MergeResult, error) {
	// Check if we're in a piece worktree
	status, err := h.Status(ctx, workDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return MergeResult{}, fmt.Errorf("not in a piece worktree")
	}

	// Get current branch (piece branch)
	pieceBranch, err := h.git.CurrentBranch(ctx, workDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get main repo root
	mainRepoRoot, err := h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get main repo root: %w", err)
	}

	// Build hook context
	hookCtx := HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     mainRepoRoot,
		MainBranch:   mainBranch,
	}

	// Run before-piece-merge hook
	if err := h.hooks.RunHook(ctx, mainRepoRoot, HookBeforePieceMerge, hookCtx); err != nil {
		return MergeResult{}, fmt.Errorf("before-piece-merge hook failed: %w", err)
	}

	// Check if main has commits not in the piece branch
	isAhead, err := h.git.IsMainAhead(ctx, mainRepoRoot, mainBranch, pieceBranch)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to check if main is ahead: %w", err)
	}

	if isAhead {
		return MergeResult{}, fmt.Errorf("cannot merge: main branch has commits not in piece worktree. Run 'mp piece update' first")
	}

	// Get commit messages from piece branch for the squash commit message
	commitMsgs, err := h.git.GetCommitMessages(ctx, mainRepoRoot, mainBranch, pieceBranch)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get commit messages: %w", err)
	}

	// Build squash commit message
	commitMsg := h.buildSquashCommitMessage(status.PieceName, commitMsgs)

	// Switch to main branch
	if err := h.git.Checkout(ctx, mainRepoRoot, mainBranch); err != nil {
		return MergeResult{}, fmt.Errorf("failed to checkout main branch: %w", err)
	}

	// Squash merge the piece branch into main
	if err := h.git.MergeSquash(ctx, mainRepoRoot, pieceBranch); err != nil {
		return MergeResult{}, fmt.Errorf("failed to squash merge piece branch into main: %w", err)
	}

	// Commit the squashed changes
	if err := h.git.Commit(ctx, mainRepoRoot, commitMsg); err != nil {
		return MergeResult{}, fmt.Errorf("failed to commit squashed changes: %w", err)
	}

	// Run after-piece-merge hook
	if err := h.hooks.RunHook(ctx, mainRepoRoot, HookAfterPieceMerge, hookCtx); err != nil {
		return MergeResult{}, fmt.Errorf("after-piece-merge hook failed: %w", err)
	}

	result := MergeResult{
		PieceName:   status.PieceName,
		PieceBranch: pieceBranch,
		MainBranch:  mainBranch,
		Status:      "merged",
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Squash merged %s into %s", pieceBranch, mainBranch),
		Data:    result,
	})

	return result, nil
}

// buildSquashCommitMessage creates a commit message for squash merge
func (h *Handler) buildSquashCommitMessage(pieceName string, commitMsgs []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("feat: %s\n", pieceName))

	if len(commitMsgs) > 0 {
		b.WriteString("\nSquashed commits:\n")
		for _, msg := range commitMsgs {
			b.WriteString(fmt.Sprintf("- %s\n", msg))
		}
	}

	return b.String()
}

// getPiecesDir returns the directory for storing pieces.
// Uses GAP (go-app-paths) for platform-appropriate paths.
func getPiecesDir() (string, error) {
	return paths.PiecesDir()
}

// MergeStatus represents the merge status of a branch
type MergeStatus struct {
	// IsMerged is true if the branch has been merged to main
	IsMerged bool `json:"is_merged"`
	// Method indicates how the merge was detected: "pr", "git", or "commit"
	Method string `json:"method,omitempty"`
	// PRNumber is set if merge was detected via PR status
	PRNumber int `json:"pr_number,omitempty"`
	// ExistsOnRemote is true if the branch still exists on the remote
	ExistsOnRemote bool `json:"exists_on_remote"`
}

// IsBranchMerged checks if a piece branch has been merged to main.
// Detection priority: 1) PR metadata, 2) gh pr list by branch, 3) git branch --merged, 4) commit history
func (h *Handler) IsBranchMerged(ctx context.Context, repoRoot, branchName, mainBranch string) (MergeStatus, error) {
	status := MergeStatus{}

	// Check if branch exists on remote
	existsOnRemote, err := h.git.BranchExistsOnRemote(ctx, repoRoot, branchName)
	if err != nil {
		// Non-fatal: continue with other checks
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to check remote branch: %v", err),
		})
	}
	status.ExistsOnRemote = existsOnRemote

	// Method 1: Check via PR metadata file (fastest, no API call)
	merged, prNumber, err := h.checkPRMergeStatus(ctx, repoRoot)
	if err == nil && merged {
		status.IsMerged = true
		status.Method = "pr"
		status.PRNumber = prNumber
		return status, nil
	}

	// Method 2: Check via gh pr list by branch name (catches squash-merged PRs without metadata)
	merged, prNumber, err = h.github.FindMergedPRByBranch(ctx, repoRoot, branchName)
	if err == nil && merged {
		status.IsMerged = true
		status.Method = "pr-branch"
		status.PRNumber = prNumber
		return status, nil
	}

	// Method 3: Check via git branch --merged
	merged, err = h.git.IsBranchMerged(ctx, repoRoot, mainBranch, branchName)
	if err != nil {
		// Log warning but continue to fallback
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("git branch --merged check failed: %v", err),
		})
	} else if merged {
		status.IsMerged = true
		status.Method = "git"
		return status, nil
	}

	// Method 4: Fallback - check if branch HEAD commit is in main history
	merged, err = h.checkCommitMerged(ctx, repoRoot, branchName, mainBranch)
	if err != nil {
		// This is the last resort, so return error
		return status, fmt.Errorf("failed to check commit history: %w", err)
	}
	if merged {
		status.IsMerged = true
		status.Method = "commit"
		return status, nil
	}

	return status, nil
}

// checkPRMergeStatus checks if a PR associated with the piece has been merged.
// Returns (merged, prNumber, error).
func (h *Handler) checkPRMergeStatus(ctx context.Context, worktreePath string) (bool, int, error) {
	// Try to read PR metadata from the piece
	metadata, err := ReadPRMetadata(worktreePath, h.deps.FS)
	if err != nil {
		// No PR metadata - skip this check
		return false, 0, fmt.Errorf("no PR metadata found: %w", err)
	}

	if metadata.PRNumber == 0 {
		return false, 0, fmt.Errorf("PR number not set in metadata")
	}

	// Check if PR is merged using gh CLI
	merged, err := h.github.IsPRMerged(ctx, worktreePath, metadata.PRNumber)
	if err != nil {
		return false, metadata.PRNumber, fmt.Errorf("failed to check PR status: %w", err)
	}

	return merged, metadata.PRNumber, nil
}

// checkCommitMerged checks if the branch's HEAD commit exists in main's history.
func (h *Handler) checkCommitMerged(ctx context.Context, repoRoot, branchName, mainBranch string) (bool, error) {
	// Get the branch's HEAD commit
	branchCommit, err := h.git.GetBranchCommit(ctx, repoRoot, branchName)
	if err != nil {
		return false, fmt.Errorf("failed to get branch commit: %w", err)
	}

	// Check if this commit is in main's history
	return h.git.IsCommitInBranch(ctx, repoRoot, branchCommit, mainBranch)
}

// CleanupResult contains information about a cleaned up piece
type CleanupResult struct {
	PieceName    string `json:"piece_name"`
	WorktreePath string `json:"worktree_path"`
	IssuePath    string `json:"issue_path,omitempty"`
	IssueUpdated bool   `json:"issue_updated,omitempty"`
}

// CleanupOptions configures the cleanup behavior
type CleanupOptions struct {
	DryRun     bool   // If true, only report what would be cleaned
	Force      bool   // If true, skip confirmation prompts (unused for now)
	MainBranch string // Main branch name to check for merged status
}

// CleanupMergedPieces finds and cleans up pieces whose branches have been merged.
// It removes worktrees, kills tmux sessions, and updates issue status to done.
func (h *Handler) CleanupMergedPieces(ctx context.Context, repoRoot string, opts CleanupOptions) ([]CleanupResult, error) {
	// Get pieces directory
	piecesDir, err := getPiecesDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	// List all piece directories
	entries, err := h.deps.FS.ReadDir(piecesDir)
	if err != nil {
		// If pieces directory doesn't exist, no pieces to clean
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read pieces directory: %w", err)
	}

	var results []CleanupResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pieceName := entry.Name()
		worktreePath := filepath.Join(piecesDir, pieceName)

		// Get the branch name from the worktree
		branchName, err := h.git.CurrentBranch(ctx, worktreePath)
		if err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Skipping %s: failed to get branch: %v", pieceName, err),
			})
			continue
		}

		// Check if branch is merged
		mergeStatus, err := h.IsBranchMerged(ctx, worktreePath, branchName, opts.MainBranch)
		if err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Skipping %s: failed to check merge status: %v", pieceName, err),
			})
			continue
		}

		if !mergeStatus.IsMerged {
			continue
		}

		result := CleanupResult{
			PieceName:    pieceName,
			WorktreePath: worktreePath,
		}

		// Read issue marker if exists
		marker, err := h.readCurrentIssueMarker(worktreePath)
		if err == nil && marker != nil {
			result.IssuePath = marker.IssuePath
		}

		if opts.DryRun {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgInfo,
				Content: fmt.Sprintf("[dry-run] Would cleanup: %s (merged via %s)", pieceName, mergeStatus.Method),
			})
			results = append(results, result)
			continue
		}

		// Cleanup the piece
		if err := h.removePiece(ctx, repoRoot, pieceName, worktreePath); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to cleanup %s: %v", pieceName, err),
			})
			continue
		}

		// Update issue status to done if marker exists
		if result.IssuePath != "" {
			absIssuePath := filepath.Join(repoRoot, result.IssuePath)
			if err := h.updateIssueStatusToDone(absIssuePath); err != nil {
				h.deps.Output.Write(core.Message{
					Type:    core.MsgWarning,
					Content: fmt.Sprintf("Failed to update issue status: %v", err),
				})
			} else {
				result.IssueUpdated = true
			}
		}

		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: fmt.Sprintf("Cleaned up: %s", pieceName),
		})

		results = append(results, result)
	}

	return results, nil
}

// readCurrentIssueMarker reads the current issue marker from a piece worktree.
func (h *Handler) readCurrentIssueMarker(worktreePath string) (*CurrentIssueMarker, error) {
	markerPath := filepath.Join(worktreePath, initcmd.DirName, "current-issue.json")
	data, err := h.deps.FS.ReadFile(markerPath)
	if err != nil {
		return nil, err
	}

	var marker CurrentIssueMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, err
	}

	return &marker, nil
}

// removePiece removes a piece worktree and associated tmux session.
func (h *Handler) removePiece(ctx context.Context, repoRoot, pieceName, worktreePath string) error {
	sessionName := fmt.Sprintf("mp-piece-%s", pieceName)

	// Kill tmux session (ignore errors - session may not exist)
	_ = h.tmux.KillSession(ctx, sessionName)

	// Remove worktree
	if err := h.git.WorktreeRemove(ctx, repoRoot, worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// AbandonOptions configures the abandon behavior
type AbandonOptions struct {
	Force        bool // Force removal even with uncommitted changes
	DeleteBranch bool // Also delete the git branch
}

// AbandonResult contains information about the abandoned piece
type AbandonResult struct {
	PieceName     string `json:"piece_name"`
	WorktreePath  string `json:"worktree_path"`
	BranchName    string `json:"branch_name,omitempty"`
	BranchDeleted bool   `json:"branch_deleted,omitempty"`
}

// AbandonPiece removes a piece worktree, tmux session, and optionally the branch.
// Unlike cleanup, this works on unmerged pieces.
func (h *Handler) AbandonPiece(ctx context.Context, pieceName string, opts AbandonOptions) (AbandonResult, error) {
	result := AbandonResult{PieceName: pieceName}

	// Find the piece
	pieces, err := h.ListPieces(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to list pieces: %w", err)
	}

	var target *PieceListItem
	for i := range pieces {
		if pieces[i].Name == pieceName {
			target = &pieces[i]
			break
		}
	}
	if target == nil {
		var names []string
		for _, p := range pieces {
			names = append(names, p.Name)
		}
		if len(names) == 0 {
			return result, fmt.Errorf("piece %q not found (no pieces exist)", pieceName)
		}
		return result, fmt.Errorf("piece %q not found. Available: %s", pieceName, strings.Join(names, ", "))
	}

	result.WorktreePath = target.WorktreePath

	// Get branch name and repo root before removing worktree
	branchName, err := h.git.CurrentBranch(ctx, target.WorktreePath)
	if err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Could not get branch name: %v", err),
		})
	} else {
		result.BranchName = branchName
	}

	repoRoot, err := h.git.GetMainRepoRoot(ctx, target.WorktreePath)
	if err != nil {
		return result, fmt.Errorf("failed to get repo root: %w", err)
	}

	// Kill tmux session if exists
	if target.HasSession {
		if err := h.tmux.KillSession(ctx, target.SessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to kill tmux session: %v", err),
			})
		}
	}

	// Remove worktree (force if requested)
	if opts.Force {
		if err := h.git.WorktreeRemoveForce(ctx, repoRoot, target.WorktreePath); err != nil {
			return result, fmt.Errorf("failed to remove worktree: %w", err)
		}
	} else {
		if err := h.git.WorktreeRemove(ctx, repoRoot, target.WorktreePath); err != nil {
			return result, fmt.Errorf("failed to remove worktree (use --force to discard changes): %w", err)
		}
	}

	// Delete branch if requested
	if opts.DeleteBranch && branchName != "" {
		if err := h.git.BranchDelete(ctx, repoRoot, branchName, true); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to delete branch: %v", err),
			})
		} else {
			result.BranchDeleted = true
		}
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Abandoned piece: %s", pieceName),
		Data:    result,
	})

	return result, nil
}

// getMainSessionName returns the tmux session name for a repo's main worktree.
// Format: mp-<repo-directory-name>
func getMainSessionName(repoRoot string) string {
	repoName := filepath.Base(repoRoot)
	return fmt.Sprintf("mp-%s", repoName)
}

// ensureMainSession creates a tmux session for the main repo if it doesn't exist.
// If overwrite is true, kills existing session and creates a new one.
// Errors are logged as warnings but don't fail the operation.
func (h *Handler) ensureMainSession(ctx context.Context, repoRoot string, overwrite bool) {
	sessionName := getMainSessionName(repoRoot)

	// Check if session already exists
	if h.tmux.HasSession(ctx, sessionName) {
		if !overwrite {
			return
		}
		// Kill existing session before recreating
		if err := h.tmux.KillSession(ctx, sessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to kill existing session: %v", err),
			})
		}
	}

	// Create the session
	if err := h.tmux.NewSession(ctx, sessionName, repoRoot); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to create main repo session: %v", err),
		})
	}
}

// updateIssueStatusToDone updates the issue status to done if currently in-progress.
func (h *Handler) updateIssueStatusToDone(issuePath string) error {
	// Check current status
	currentStatus, err := ParseStatus(issuePath, h.deps.FS)
	if err != nil {
		return fmt.Errorf("failed to read issue status: %w", err)
	}

	// Only update if status is in-progress
	if currentStatus != StatusInProgress {
		return nil
	}

	// Update to done
	if err := UpdateStatus(issuePath, StatusDone, h.deps.FS); err != nil {
		return fmt.Errorf("failed to update issue status: %w", err)
	}

	return nil
}

// ListPieces returns all available pieces in the pieces directory.
// Results are sorted by modification time (newest first).
func (h *Handler) ListPieces(ctx context.Context) ([]PieceListItem, error) {
	piecesDir, err := getPiecesDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	entries, err := h.deps.FS.ReadDir(piecesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PieceListItem{}, nil
		}
		return nil, fmt.Errorf("failed to read pieces directory: %w", err)
	}

	var pieces []PieceListItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		worktreePath := filepath.Join(piecesDir, name)
		sessionName := fmt.Sprintf("mp-piece-%s", name)

		// Get modification time
		info, err := entry.Info()
		var modTime time.Time
		if err == nil {
			modTime = info.ModTime()
		}

		// Check if tmux session exists
		hasSession := h.tmux.HasSession(ctx, sessionName)

		pieces = append(pieces, PieceListItem{
			Name:         name,
			WorktreePath: worktreePath,
			SessionName:  sessionName,
			HasSession:   hasSession,
			ModTime:      modTime,
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(pieces, func(i, j int) bool {
		return pieces[i].ModTime.After(pieces[j].ModTime)
	})

	return pieces, nil
}

// SwitchPiece switches to a piece by name.
// It tries tmux attach/switch first, falls back to printing path.
func (h *Handler) SwitchPiece(ctx context.Context, name string) (SwitchResult, error) {
	pieces, err := h.ListPieces(ctx)
	if err != nil {
		return SwitchResult{}, err
	}

	// Find the piece
	var target *PieceListItem
	for i := range pieces {
		if pieces[i].Name == name {
			target = &pieces[i]
			break
		}
	}
	if target == nil {
		// Build list of available pieces for error message
		var names []string
		for _, p := range pieces {
			names = append(names, p.Name)
		}
		if len(names) == 0 {
			return SwitchResult{}, fmt.Errorf("piece %q not found (no pieces exist)", name)
		}
		return SwitchResult{}, fmt.Errorf("piece %q not found. Available: %s", name, strings.Join(names, ", "))
	}

	result := SwitchResult{Piece: *target}

	// Try tmux if session exists
	if target.HasSession {
		if h.tmux.InTmux() {
			// Already in tmux, use switch-client
			if err := h.tmux.SwitchClient(ctx, target.SessionName); err == nil {
				result.Method = "tmux-switch"
				h.deps.Output.Write(core.Message{
					Type:    core.MsgSuccess,
					Content: fmt.Sprintf("Switched to piece: %s", name),
					Data:    result,
				})
				return result, nil
			}
			// Fall through to path on error
		} else {
			// Not in tmux, use attach-session
			if err := h.tmux.AttachSession(ctx, target.SessionName); err == nil {
				result.Method = "tmux-attach"
				h.deps.Output.Write(core.Message{
					Type:    core.MsgSuccess,
					Content: fmt.Sprintf("Attached to piece: %s", name),
					Data:    result,
				})
				return result, nil
			}
			// Fall through to path on error
		}
	}

	// Fallback: print path for cd $(mp piece switch --name foo)
	result.Method = "path"
	fmt.Println(target.WorktreePath)

	return result, nil
}
