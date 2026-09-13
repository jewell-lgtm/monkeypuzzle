// Package branch projects the branch layers recorded in mp piece metadata.
// Raw Git refs are deliberately outside this atom: they can be adopted into a
// piece, after which mp owns their lineage and lifecycle.
package branch

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

type Info struct {
	Name       string `json:"name"`
	Base       string `json:"base"`
	Piece      string `json:"piece"`
	Worktree   string `json:"worktree"`
	Current    bool   `json:"current,omitempty"`
	CheckedOut bool   `json:"checked_out,omitempty"`
	Initial    bool   `json:"initial,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Status     string `json:"status,omitempty"`
	Order      int    `json:"order"`
}

type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	pieces *piece.Handler
}

func NewHandler(deps core.Deps) *Handler {
	return &Handler{deps: deps, git: adapters.NewGit(deps.Exec), pieces: piece.NewHandler(deps)}
}

func (h *Handler) List(ctx context.Context, workDir string) ([]Info, error) {
	root, err := h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	items, err := h.pieces.ListPieces(ctx, root)
	if err != nil {
		return nil, err
	}
	var rows []Info
	seenWorktree := map[string]bool{}
	caller := canonicalPath(workDir)
	add := func(name, path, fallbackBranch, fallbackBase string) error {
		key := canonicalPath(path)
		if path == "" || seenWorktree[key] {
			return nil
		}
		seenWorktree[key] = true
		meta, err := piece.ReadPieceMetadata(path, h.deps.FS)
		if err != nil {
			return err
		}
		current, _ := h.git.CurrentBranch(ctx, path)
		isCaller := piece.IsPathInside(caller, key)
		entries := meta.Stack
		if len(entries) == 0 {
			base := meta.Parent
			if base == "" {
				base = fallbackBase
			}
			entries = []piece.StackEntry{{Branch: fallbackBranch, Base: base}}
		}
		for i, entry := range entries {
			rows = append(rows, Info{Name: entry.Branch, Base: entry.Base, Piece: name, Worktree: path, Current: isCaller && entry.Branch == current, CheckedOut: entry.Branch == current, Initial: i == 0, PRNumber: entry.PRNumber, PRURL: entry.PRURL, Status: entry.Status, Order: i})
		}
		return nil
	}
	for _, item := range piece.LocalPieces(items) {
		if err := add(item.Name, item.WorktreePath, item.Branch, item.Parent); err != nil {
			return nil, err
		}
	}
	// An editor may host an mp-coded worktree outside .monkeypuzzle/pieces.
	if st, statusErr := h.pieces.Status(ctx, workDir); statusErr == nil && st.InPiece {
		current, _ := h.git.CurrentBranch(ctx, st.WorktreePath)
		if err := add(st.PieceName, st.WorktreePath, current, "main"); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Piece != rows[j].Piece {
			return rows[i].Piece < rows[j].Piece
		}
		return rows[i].Order < rows[j].Order
	})
	return rows, nil
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func (h *Handler) Show(ctx context.Context, workDir, name string) (Info, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		var err error
		name, err = h.git.CurrentBranch(ctx, workDir)
		if err != nil {
			return Info{}, err
		}
	}
	rows, err := h.List(ctx, workDir)
	if err != nil {
		return Info{}, err
	}
	for _, row := range rows {
		if row.Name == name {
			return row, nil
		}
	}
	return Info{}, fmt.Errorf("branch %q is not managed by mp; adopt it as a piece first", name)
}
