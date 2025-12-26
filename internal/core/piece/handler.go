package piece

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const (
	symlinkName = ".monkeypuzzle-source"
)

// Handler executes piece-related commands
type Handler struct {
	deps core.Deps
	git  *adapters.Git
	tmux *adapters.Tmux
}

// NewHandler creates a new piece handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps: deps,
		git:  adapters.NewGit(deps.Exec),
		tmux: adapters.NewTmux(deps.Exec),
	}
}

// CreatePiece creates a new git worktree with tmux session
func (h *Handler) CreatePiece(monkeypuzzleSourceDir string) (PieceInfo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect git repo root
	repoRoot, err := h.git.RepoRoot(wd)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
	}

	// Generate piece name
	piecesDir, err := getPiecesDir()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	pieceName, err := h.GeneratePieceName(piecesDir)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to generate piece name: %w", err)
	}

	// Create pieces directory if it doesn't exist
	if err := h.deps.FS.MkdirAll(piecesDir, 0755); err != nil {
		return PieceInfo{}, fmt.Errorf("failed to create pieces directory: %w", err)
	}

	// Create worktree
	worktreePath := filepath.Join(piecesDir, pieceName)
	if err := h.git.WorktreeAdd(repoRoot, worktreePath); err != nil {
		return PieceInfo{}, fmt.Errorf("failed to create worktree: %w", err)
	}

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
	if err := h.tmux.NewSession(sessionName, worktreePath); err != nil {
		// If tmux fails, log but don't fail the operation
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to create tmux session: %v", err),
		})
	}

	info := PieceInfo{
		Name:         pieceName,
		WorktreePath: worktreePath,
		SessionName:  sessionName,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Created piece: %s at %s", pieceName, worktreePath),
		Data:    info,
	})

	return info, nil
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
		repoRoot, _ := h.git.RepoRoot(workDir)
		return PieceStatus{
			InPiece:  false,
			RepoRoot: repoRoot,
		}, nil
	}

	// In worktree - extract piece name from path
	worktreePath, _ := h.git.RepoRoot(workDir)
	pieceName := filepath.Base(worktreePath)
	repoRoot := ""
	// Try to get main repo root by going up from worktree
	if piecesDir, err := getPiecesDir(); err == nil {
		if strings.HasPrefix(worktreePath, piecesDir) {
			// This is in the pieces directory, so try to get the original repo root
			// by checking the worktree
		}
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

	// Merge the main branch
	if err := h.git.Merge(workDir, mainBranch); err != nil {
		return err
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Merged %s into %s", mainBranch, currentBranch),
	})

	return nil
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
