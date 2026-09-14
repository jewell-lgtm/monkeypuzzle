// Package worktree presents the storage occupied by mp pieces. Git worktrees
// outside mp are visible as adoption candidates, but mp never mutates them.
package worktree

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

type Info struct {
	Path        string `json:"path"`
	Branch      string `json:"branch,omitempty"`
	Main        bool   `json:"main,omitempty"`
	Current     bool   `json:"current,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	Prunable    bool   `json:"prunable,omitempty"`
	Managed     bool   `json:"managed,omitempty"`
	Piece       string `json:"piece,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	Inbox       bool   `json:"in_inbox,omitempty"`
	Lifecycle   string `json:"lifecycle,omitempty"`
	AgentStatus string `json:"agent_status,omitempty"`
	PRNumber    int    `json:"pr_number,omitempty"`
}

type DeleteInput struct {
	Selector     string `json:"selector"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"delete_branch,omitempty"`
}

type DeleteResult struct {
	Path          string   `json:"path"`
	Branch        string   `json:"branch,omitempty"`
	Piece         string   `json:"piece,omitempty"`
	BranchDeleted bool     `json:"branch_deleted,omitempty"`
	Reparented    []string `json:"reparented_children,omitempty"`
}

type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	pieces *piece.Handler
}

func NewHandler(deps core.Deps, pieces *piece.Handler) *Handler {
	if pieces == nil {
		pieces = piece.NewHandler(deps)
	}
	return &Handler{deps: deps, git: adapters.NewGit(deps.Exec), pieces: pieces}
}

// List reports every worktree, including the on-disk size of each checkout.
func (h *Handler) List(ctx context.Context, workDir string) ([]Info, error) {
	return h.list(ctx, workDir, true)
}

// ListLight is List without SizeBytes. Sizing walks every file in every
// checkout, which is far too slow for shell completion.
func (h *Handler) ListLight(ctx context.Context, workDir string) ([]Info, error) {
	return h.list(ctx, workDir, false)
}

func (h *Handler) list(ctx context.Context, workDir string, withSize bool) ([]Info, error) {
	repoRoot, err := h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	worktrees, err := h.git.Worktrees(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	type managedInfo struct {
		name, lifecycle, agent string
		pr                     int
	}
	managed := map[string]managedInfo{}
	if pieces, err := h.pieces.ListPieces(ctx, repoRoot); err == nil {
		for _, p := range pieces {
			if !p.IsPlaced() {
				info := managedInfo{name: p.Name, lifecycle: "idle", agent: p.AgentStatus}
				if meta, metaErr := piece.ReadPieceMetadata(p.WorktreePath, h.deps.FS); metaErr == nil {
					if meta.Merged {
						info.lifecycle = "merged"
					} else if p.AgentStatus != "" {
						info.lifecycle = p.AgentStatus
					}
					for _, entry := range meta.Stack {
						if entry.PRNumber != 0 {
							info.pr = entry.PRNumber
							if info.lifecycle == "idle" {
								info.lifecycle = "review"
							}
						}
					}
				}
				managed[canonicalPath(p.WorktreePath)] = info
			}
		}
	}
	current := canonicalPath(workDir)
	// Piece worktrees sit inside the main checkout, so several paths can
	// contain the cwd. The deepest one is the worktree you are actually in.
	currentWorktree := ""
	for _, wt := range worktrees {
		path := canonicalPath(wt.Path)
		if path != current && !piece.IsPathInside(current, path) {
			continue
		}
		if len(path) > len(currentWorktree) {
			currentWorktree = path
		}
	}
	piecesRel := filepath.Join(projectdir.RelDir(repoRoot), "pieces")
	rows := make([]Info, 0, len(worktrees))
	for _, wt := range worktrees {
		path := canonicalPath(wt.Path)
		isMain := path == canonicalPath(repoRoot)
		managedRow, isManaged := managed[path]
		var size int64
		if withSize {
			size = checkoutSize(wt.Path, isMain, piecesRel)
		}
		rows = append(rows, Info{
			Path: wt.Path, Branch: wt.Branch, Main: isMain, Current: path == currentWorktree,
			Locked: wt.Locked, Prunable: wt.Prunable, Managed: isManaged, Piece: managedRow.name,
			SizeBytes: size, Inbox: isManaged, Lifecycle: managedRow.lifecycle, AgentStatus: managedRow.agent, PRNumber: managedRow.pr,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Main != rows[j].Main {
			return rows[i].Main
		}
		return rows[i].Path < rows[j].Path
	})
	return rows, nil
}

func checkoutSize(root string, main bool, piecesRel string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel == ".git" || (main && rel == piecesRel) {
				return filepath.SkipDir
			}
		}
		if entry.Type().IsRegular() {
			if info, infoErr := entry.Info(); infoErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func (h *Handler) Show(ctx context.Context, workDir, selector string) (Info, error) {
	rows, err := h.List(ctx, workDir)
	if err != nil {
		return Info{}, err
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		for _, row := range rows {
			if row.Current {
				return row, nil
			}
		}
		return Info{}, fmt.Errorf("current directory is not a registered worktree")
	}
	var matches []Info
	for _, row := range rows {
		if selector == row.Path || canonicalPath(selector) == canonicalPath(row.Path) || selector == row.Branch || selector == row.Piece {
			matches = append(matches, row)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Info{}, fmt.Errorf("worktree selector %q is ambiguous; use its path", selector)
	}
	return Info{}, fmt.Errorf("unknown worktree %q", selector)
}

func (h *Handler) Delete(ctx context.Context, workDir string, in DeleteInput) (DeleteResult, error) {
	target, err := h.Show(ctx, workDir, in.Selector)
	if err != nil {
		return DeleteResult{}, err
	}
	if target.Main {
		return DeleteResult{}, fmt.Errorf("cannot delete the main worktree")
	}
	if target.Current {
		return DeleteResult{}, fmt.Errorf("cannot delete the worktree containing the current process; run from another worktree")
	}
	if !target.Managed {
		return DeleteResult{}, fmt.Errorf("worktree %q is not mp-managed; use 'mp piece adopt %s' to manage it, or 'git worktree remove' to remove the raw Git worktree", target.Path, target.Branch)
	}
	result, err := h.pieces.AbandonPiece(ctx, target.Piece, piece.AbandonOptions{Force: in.Force, DeleteBranch: in.DeleteBranch})
	if err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{Path: result.WorktreePath, Branch: result.BranchName, Piece: result.PieceName, BranchDeleted: result.BranchDeleted, Reparented: result.ReparentedChildren}, nil
}

func canonicalPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return path
}
