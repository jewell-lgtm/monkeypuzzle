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
	deps  core.Deps
	git   *adapters.Git
	tmux  *adapters.Tmux
	hooks *HookRunner
}

// NewHandler creates a new piece handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:  deps,
		git:   adapters.NewGit(deps.Exec),
		tmux:  adapters.NewTmux(deps.Exec),
		hooks: NewHookRunner(deps),
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
