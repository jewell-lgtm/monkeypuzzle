package piece

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
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

// CreatePiece creates a new git worktree with tmux session.
// If pieceName is provided and non-empty, it will be used (after checking it doesn't exist).
// If pieceName is empty, a name will be generated automatically.
func (h *Handler) CreatePiece(monkeypuzzleSourceDir string, pieceName string) (PieceInfo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect git repo root
	repoRoot, err := h.git.RepoRoot(wd)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
	}

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
		// Validate that the provided name doesn't already exist
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
	if err := h.git.WorktreeAdd(repoRoot, worktreePath); err != nil {
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
	if err := h.tmux.NewSession(sessionName, worktreePath); err != nil {
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
	if err := h.hooks.RunHook(repoRoot, HookOnPieceCreate, hookCtx); err != nil {
		// Cleanup: remove worktree and tmux session on hook failure
		h.cleanupPiece(repoRoot, worktreePath, sessionName, tmuxCreated)
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
func (h *Handler) CreatePieceFromIssue(monkeypuzzleSourceDir, issuePath string) (PieceInfo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect git repo root
	repoRoot, err := h.git.RepoRoot(wd)
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
	info, err := h.CreatePiece(monkeypuzzleSourceDir, pieceName)
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
func (h *Handler) cleanupPiece(repoRoot, worktreePath, sessionName string, tmuxCreated bool) {
	// Kill tmux session if it was created
	if tmuxCreated {
		if err := h.tmux.KillSession(sessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to cleanup tmux session: %v", err),
			})
		}
	}

	// Remove worktree
	if err := h.git.WorktreeRemove(repoRoot, worktreePath); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to cleanup worktree: %v", err),
		})
	}
}

// Status detects if we're currently in a piece worktree or main repo
func (h *Handler) Status(workDir string) (PieceStatus, error) {
	gitDir, err := h.git.RevParseGitDir(workDir)
	if err != nil {
		// Not in a git repo
		return PieceStatus{
			InPiece: false,
		}, nil
	}

	isWorktree := h.git.IsWorktree(gitDir)
	if !isWorktree {
		// In main repo
		repoRoot, err := h.git.RepoRoot(workDir)
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
	worktreePath, err := h.git.RepoRoot(workDir)
	if err != nil {
		// Fallback: use workDir if we can't get worktree path
		worktreePath = workDir
	}
	pieceName := filepath.Base(worktreePath)

	// Get main repo root from worktree
	repoRoot, err := h.git.GetMainRepoRoot(workDir)
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
func (h *Handler) UpdatePiece(workDir, mainBranch string) error {
	// Check if we're in a piece worktree
	status, err := h.Status(workDir)
	if err != nil {
		return fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return fmt.Errorf("not in a piece worktree")
	}

	// Get current branch to verify we're on a branch
	currentBranch, err := h.git.CurrentBranch(workDir)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Build hook context
	hookCtx := HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     status.RepoRoot,
		MainBranch:   mainBranch,
	}

	// Run before-piece-update hook
	if err := h.hooks.RunHook(status.RepoRoot, HookBeforePieceUpdate, hookCtx); err != nil {
		return fmt.Errorf("before-piece-update hook failed: %w", err)
	}

	// Merge the main branch
	if err := h.git.Merge(workDir, mainBranch); err != nil {
		return err
	}

	// Run after-piece-update hook
	if err := h.hooks.RunHook(status.RepoRoot, HookAfterPieceUpdate, hookCtx); err != nil {
		return fmt.Errorf("after-piece-update hook failed: %w", err)
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Merged %s into %s", mainBranch, currentBranch),
	})

	return nil
}

// MergePiece squash-merges the piece branch back into main as a single commit.
// Fails if main has commits that are not in the piece worktree.
func (h *Handler) MergePiece(workDir, mainBranch string) error {
	// Check if we're in a piece worktree
	status, err := h.Status(workDir)
	if err != nil {
		return fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return fmt.Errorf("not in a piece worktree")
	}

	// Get current branch (piece branch)
	pieceBranch, err := h.git.CurrentBranch(workDir)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get main repo root
	mainRepoRoot, err := h.git.GetMainRepoRoot(workDir)
	if err != nil {
		return fmt.Errorf("failed to get main repo root: %w", err)
	}

	// Build hook context
	hookCtx := HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     mainRepoRoot,
		MainBranch:   mainBranch,
	}

	// Run before-piece-merge hook
	if err := h.hooks.RunHook(mainRepoRoot, HookBeforePieceMerge, hookCtx); err != nil {
		return fmt.Errorf("before-piece-merge hook failed: %w", err)
	}

	// Check if main has commits not in the piece branch
	isAhead, err := h.git.IsMainAhead(mainRepoRoot, mainBranch, pieceBranch)
	if err != nil {
		return fmt.Errorf("failed to check if main is ahead: %w", err)
	}

	if isAhead {
		return fmt.Errorf("cannot merge: main branch has commits not in piece worktree. Run 'mp piece update' first")
	}

	// Get commit messages from piece branch for the squash commit message
	commitMsgs, err := h.git.GetCommitMessages(mainRepoRoot, mainBranch, pieceBranch)
	if err != nil {
		return fmt.Errorf("failed to get commit messages: %w", err)
	}

	// Build squash commit message
	commitMsg := h.buildSquashCommitMessage(status.PieceName, commitMsgs)

	// Switch to main branch
	if err := h.git.Checkout(mainRepoRoot, mainBranch); err != nil {
		return fmt.Errorf("failed to checkout main branch: %w", err)
	}

	// Squash merge the piece branch into main
	if err := h.git.MergeSquash(mainRepoRoot, pieceBranch); err != nil {
		return fmt.Errorf("failed to squash merge piece branch into main: %w", err)
	}

	// Commit the squashed changes
	if err := h.git.Commit(mainRepoRoot, commitMsg); err != nil {
		return fmt.Errorf("failed to commit squashed changes: %w", err)
	}

	// Run after-piece-merge hook
	if err := h.hooks.RunHook(mainRepoRoot, HookAfterPieceMerge, hookCtx); err != nil {
		return fmt.Errorf("after-piece-merge hook failed: %w", err)
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Squash merged %s into %s", pieceBranch, mainBranch),
	})

	return nil
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

// getPiecesDir returns the directory for storing pieces, using XDG_DATA_HOME
func getPiecesDir() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "monkeypuzzle", "pieces"), nil
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
// Detection priority: 1) GitHub PR status, 2) git branch --merged, 3) commit history
func (h *Handler) IsBranchMerged(repoRoot, branchName, mainBranch string) (MergeStatus, error) {
	status := MergeStatus{}

	// Check if branch exists on remote
	existsOnRemote, err := h.git.BranchExistsOnRemote(repoRoot, branchName)
	if err != nil {
		// Non-fatal: continue with other checks
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to check remote branch: %v", err),
		})
	}
	status.ExistsOnRemote = existsOnRemote

	// Method 1: Check via GitHub PR status (most reliable for remote PRs)
	merged, prNumber, err := h.checkPRMergeStatus(repoRoot)
	if err == nil && merged {
		status.IsMerged = true
		status.Method = "pr"
		status.PRNumber = prNumber
		return status, nil
	}

	// Method 2: Check via git branch --merged
	merged, err = h.git.IsBranchMerged(repoRoot, mainBranch, branchName)
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

	// Method 3: Fallback - check if branch HEAD commit is in main history
	merged, err = h.checkCommitMerged(repoRoot, branchName, mainBranch)
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
func (h *Handler) checkPRMergeStatus(worktreePath string) (bool, int, error) {
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
	merged, err := h.github.IsPRMerged(worktreePath, metadata.PRNumber)
	if err != nil {
		return false, metadata.PRNumber, fmt.Errorf("failed to check PR status: %w", err)
	}

	return merged, metadata.PRNumber, nil
}

// checkCommitMerged checks if the branch's HEAD commit exists in main's history.
func (h *Handler) checkCommitMerged(repoRoot, branchName, mainBranch string) (bool, error) {
	// Get the branch's HEAD commit
	branchCommit, err := h.git.GetBranchCommit(repoRoot, branchName)
	if err != nil {
		return false, fmt.Errorf("failed to get branch commit: %w", err)
	}

	// Check if this commit is in main's history
	return h.git.IsCommitInBranch(repoRoot, branchCommit, mainBranch)
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
func (h *Handler) CleanupMergedPieces(repoRoot string, opts CleanupOptions) ([]CleanupResult, error) {
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
		branchName, err := h.git.CurrentBranch(worktreePath)
		if err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Skipping %s: failed to get branch: %v", pieceName, err),
			})
			continue
		}

		// Check if branch is merged
		mergeStatus, err := h.IsBranchMerged(worktreePath, branchName, opts.MainBranch)
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
		if err := h.removePiece(repoRoot, pieceName, worktreePath); err != nil {
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
func (h *Handler) removePiece(repoRoot, pieceName, worktreePath string) error {
	sessionName := fmt.Sprintf("mp-piece-%s", pieceName)

	// Kill tmux session (ignore errors - session may not exist)
	_ = h.tmux.KillSession(sessionName)

	// Remove worktree
	if err := h.git.WorktreeRemove(repoRoot, worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
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
