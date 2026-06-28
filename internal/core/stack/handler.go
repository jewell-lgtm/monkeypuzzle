package stack

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// Handler executes stack-wide commands. It composes piece.Handler to reuse the
// proven, conflict-aborting topological walk primitives.
type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	pieces *piece.Handler
}

// NewHandler creates a new stack handler.
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:   deps,
		git:    adapters.NewGit(deps.Exec),
		pieces: piece.NewHandler(deps),
	}
}

// providerForRepo loads the configured PR/MR provider for the given repo root,
// defaulting to GitHub. Mirrors pr.Handler.providerForRepo (stack importing pr
// is acyclic: pr does not import stack).
func (h *Handler) providerForRepo(repoRoot string) (pr.Provider, error) {
	cfg, err := piece.ReadConfig(repoRoot, h.deps.FS)
	if err != nil {
		return nil, fmt.Errorf("failed to read config (run mp init first): %w", err)
	}
	providerType := cfg.PR.Provider
	if providerType == "" {
		providerType = "github"
	}
	return pr.NewProvider(pr.ProviderConfig{
		ProviderType: providerType,
		Config:       cfg.PR.Config,
		Deps:         pr.ProviderDeps{Exec: h.deps.Exec},
	})
}

func (h *Handler) emit(t core.MessageType, content string) {
	h.deps.Output.Write(core.Message{Type: t, Content: content})
}

// resolveRepo resolves the main repo root and pieces directory from workDir,
// working from either the main repo or a piece worktree.
func (h *Handler) resolveRepo(ctx context.Context, workDir string) (mainRepoRoot, piecesDir string, err error) {
	mainRepoRoot, err = h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		return "", "", fmt.Errorf("not in a git repository: %w", err)
	}
	piecesDir, err = projectdir.PiecesDir(mainRepoRoot)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve pieces directory: %w", err)
	}
	return mainRepoRoot, piecesDir, nil
}

// ---- Status ----------------------------------------------------------------

// Status reports the stack tree, PR/MR state per piece, and drift between local
// lineage and the forge's PR/MR bases. With FromRemote it rebuilds local lineage
// from PR/MR bases; with ApplyBases it edits PR/MR bases to match local lineage.
func (h *Handler) Status(ctx context.Context, workDir string, in StatusInput) (StackStatusResult, error) {
	in = WithStatusDefaults(in)

	mainRepoRoot, piecesDir, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return StackStatusResult{}, err
	}
	items, err := h.pieces.ListPieces(ctx, mainRepoRoot)
	if err != nil {
		return StackStatusResult{}, err
	}

	result := StackStatusResult{MainBranch: in.MainBranch}

	provider, err := h.providerForRepo(mainRepoRoot)
	if err != nil {
		return StackStatusResult{}, err
	}

	prs, listErr := provider.ListPRs(ctx, mainRepoRoot)
	forgeAvailable := true
	if errors.Is(listErr, pr.ErrProviderUnavailable) {
		forgeAvailable = false
		h.emit(core.MsgWarning, forgeUnavailableMsg())
	} else if listErr != nil {
		return StackStatusResult{}, listErr
	}
	result.GitHubChecked = forgeAvailable
	result.ForgeChecked = forgeAvailable

	prByHead := make(map[string]pr.PRInfo, len(prs))
	for _, p := range prs {
		prByHead[p.HeadRefName] = p
	}

	// Reconstruct local lineage from PR/MR bases (machine-#2 rebuild). Metadata-only.
	if in.fromRemote() && forgeAvailable {
		for _, rw := range reconstructParents(items, prByHead, in.MainBranch) {
			wt := filepath.Join(piecesDir, rw.Piece)
			meta, err := piece.ReadPieceMetadata(wt, h.deps.FS)
			if err != nil {
				return StackStatusResult{}, err
			}
			meta.Parent = rw.NewParent
			if err := piece.WritePieceMetadata(wt, *meta, h.deps.FS); err != nil {
				return StackStatusResult{}, err
			}
			result.Reconstructed = append(result.Reconstructed, rw.Piece)
		}
		if len(result.Reconstructed) > 0 {
			if items, err = h.pieces.ListPieces(ctx, mainRepoRoot); err != nil {
				return StackStatusResult{}, err
			}
		}
	}

	// Push local lineage to the forge by editing PR/MR bases (opt-in).
	if in.ApplyBases && forgeAvailable {
		for _, f := range computeBaseFixes(items, prByHead, in.MainBranch) {
			if err := provider.SetPRBase(ctx, mainRepoRoot, f.PRNumber, f.Base); err != nil {
				return StackStatusResult{}, err
			}
			result.Applied = append(result.Applied, fmt.Sprintf("#%d", f.PRNumber))
			// Reflect the fix so drift isn't re-reported for it below.
			for head, info := range prByHead {
				if info.Number == f.PRNumber {
					info.BaseRefName = f.Base
					prByHead[head] = info
				}
			}
		}
	}

	tree, drift := buildStackTree(items, prByHead, in.MainBranch)
	result.Tree = tree
	result.Drift = drift
	return result, nil
}

// ---- Sync ------------------------------------------------------------------

// Sync fetches, fast-forwards main, then propagates each parent into its children
// across the stack. Default strategy is merge (safe, no force-push). On conflict it
// aborts cleanly and prints plain-English next steps.
func (h *Handler) Sync(ctx context.Context, workDir string, in SyncInput) (SyncResult, error) {
	in = WithSyncDefaults(in)
	if err := ValidateSyncInput(in); err != nil {
		return SyncResult{}, err
	}

	if in.DryRun {
		return h.previewSync(ctx, workDir, in)
	}

	mainRepoRoot, piecesDir, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{MainBranch: in.MainBranch, Strategy: in.Strategy, Status: "synced"}

	scope, err := h.scopeSet(ctx, workDir, piecesDir, in)
	if err != nil {
		return SyncResult{}, err
	}

	// Record every piece branch's SHA so 'mp stack undo' can restore them.
	if err := h.writeSnapshot(ctx, mainRepoRoot, piecesDir, "sync"); err != nil {
		return SyncResult{}, fmt.Errorf("failed to write undo snapshot: %w", err)
	}

	// Update main from the chosen upstream (no-op when its remote isn't configured).
	if err := h.updateMain(ctx, mainRepoRoot, in.MainBranch, in.From); err != nil {
		result.Status = "aborted"
		return result, err
	}

	roots, err := piece.GetPieceChildren("main", piecesDir, h.deps.FS)
	if err != nil {
		return SyncResult{}, err
	}
	sort.Strings(roots)

	if in.Strategy == StrategyRebase {
		result, err = h.syncRebase(ctx, mainRepoRoot, piecesDir, roots, scope, in, result)
	} else {
		result, err = h.syncMerge(ctx, piecesDir, roots, scope, in, result)
	}
	if err != nil {
		return result, err
	}

	h.emit(core.MsgSuccess, fmt.Sprintf("Stack synced (%s): %d piece(s) updated", in.Strategy, len(result.Updated)))
	return result, nil
}

// previewSync walks the stack exactly as a real sync would (parent before child)
// and reports which pieces would be updated, without fetching, snapshotting, or
// touching any branch. It is the read-only half of Sync used for dry-runs.
func (h *Handler) previewSync(ctx context.Context, workDir string, in SyncInput) (SyncResult, error) {
	_, piecesDir, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{MainBranch: in.MainBranch, Strategy: in.Strategy, Status: "dry-run"}

	scope, err := h.scopeSet(ctx, workDir, piecesDir, in)
	if err != nil {
		return SyncResult{}, err
	}

	h.emit(core.MsgInfo, fmt.Sprintf("[dry-run] Would update %q from %q (fetch + fast-forward)", in.MainBranch, in.From))

	roots, err := piece.GetPieceChildren("main", piecesDir, h.deps.FS)
	if err != nil {
		return SyncResult{}, err
	}
	sort.Strings(roots)

	type node struct{ name, parent string }
	queue := make([]node, 0, len(roots))
	for _, r := range roots {
		queue = append(queue, node{r, in.MainBranch})
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]

		if inScope(scope, n.name) {
			result.Updated = append(result.Updated, n.name)
			h.emit(core.MsgInfo, fmt.Sprintf("[dry-run] Would sync %q (%s %q into %q)", n.name, in.Strategy, n.parent, n.name))
		} else {
			result.Skipped = append(result.Skipped, n.name)
		}

		children, err := piece.GetPieceChildren(n.name, piecesDir, h.deps.FS)
		if err != nil {
			return result, err
		}
		sort.Strings(children)
		for _, c := range children {
			queue = append(queue, node{c, n.name})
		}
	}

	h.emit(core.MsgInfo, fmt.Sprintf("[dry-run] %d piece(s) would be synced; pass --apply to sync", len(result.Updated)))
	return result, nil
}

// syncMerge walks the stack breadth-first (parent before child), merging each
// parent branch into its child. Auto-stashes uncommitted changes around each merge.
func (h *Handler) syncMerge(ctx context.Context, piecesDir string, roots []string, scope map[string]bool, in SyncInput, result SyncResult) (SyncResult, error) {
	type node struct{ name, parentBranch string }
	queue := make([]node, 0, len(roots))
	for _, r := range roots {
		queue = append(queue, node{r, in.MainBranch})
	}

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		wt := filepath.Join(piecesDir, n.name)

		if inProgress, _ := h.git.RebaseInProgress(ctx, wt); inProgress {
			h.emit(core.MsgError, rebaseInProgressMsg(wt))
			result.Status = "aborted"
			return result, fmt.Errorf("rebase already in progress in %s", wt)
		}

		if inScope(scope, n.name) {
			popConflict, err := h.mergePieceWithStash(ctx, piecesDir, n.name, n.parentBranch)
			if err != nil {
				h.emit(core.MsgError, mergeConflictMsg(n.parentBranch, n.name, wt))
				result.Status = "aborted"
				return result, err
			}
			if popConflict {
				h.emit(core.MsgWarning, stashPopConflictMsg(n.name, wt))
			}
			result.Updated = append(result.Updated, n.name)
			if in.Push {
				if err := h.git.Push(ctx, wt); err != nil {
					h.emit(core.MsgWarning, fmt.Sprintf("Push failed for %q: %v", n.name, err))
				} else {
					result.Pushed = append(result.Pushed, n.name)
				}
			}
		} else {
			result.Skipped = append(result.Skipped, n.name)
		}

		children, err := piece.GetPieceChildren(n.name, piecesDir, h.deps.FS)
		if err != nil {
			return result, err
		}
		sort.Strings(children)
		for _, c := range children {
			queue = append(queue, node{c, n.name})
		}
	}

	return result, nil
}

// mergePieceWithStash stashes any uncommitted changes, merges parentBranch into the
// piece, then restores the stash. Returns popConflict=true when the merge succeeded
// but restoring local changes conflicted (non-fatal). On merge conflict the merge is
// already aborted and the stash is restored before returning the error.
func (h *Handler) mergePieceWithStash(ctx context.Context, piecesDir, name, parentBranch string) (popConflict bool, err error) {
	wt := filepath.Join(piecesDir, name)
	stashed, err := h.git.StashPush(ctx, wt)
	if err != nil {
		return false, err
	}
	if err := h.pieces.MergeIntoChild(ctx, piecesDir, name, parentBranch); err != nil {
		if stashed {
			_ = h.git.StashPop(ctx, wt)
		}
		return false, err
	}
	if stashed {
		if err := h.git.StashPop(ctx, wt); err != nil {
			return true, nil
		}
	}
	return false, nil
}

// syncRebase rebases each in-scope root subtree onto the updated main, recursively
// re-pointing descendants. Stashes the whole subtree up front and restores after.
func (h *Handler) syncRebase(ctx context.Context, mainRepoRoot, piecesDir string, roots []string, scope map[string]bool, in SyncInput, result SyncResult) (SyncResult, error) {
	// Determine which root subtrees to process and gather their members.
	var members []string
	seen := map[string]bool{}
	var process []string
	for _, r := range roots {
		if !inScope(scope, r) {
			continue
		}
		process = append(process, r)
		for _, p := range h.subtreeMembers(r, piecesDir) {
			if !seen[p] {
				seen[p] = true
				members = append(members, p)
			}
		}
	}

	// Pre-flight: no rebase already in progress anywhere we're about to touch.
	for _, p := range members {
		wt := filepath.Join(piecesDir, p)
		if inProgress, _ := h.git.RebaseInProgress(ctx, wt); inProgress {
			h.emit(core.MsgError, rebaseInProgressMsg(wt))
			result.Status = "aborted"
			return result, fmt.Errorf("rebase already in progress in %s", wt)
		}
	}

	// Stash uncommitted work across the subtree.
	stashed := map[string]bool{}
	for _, p := range members {
		wt := filepath.Join(piecesDir, p)
		did, err := h.git.StashPush(ctx, wt)
		if err != nil {
			result.Status = "aborted"
			return result, err
		}
		if did {
			stashed[p] = true
		}
	}

	// Rebase each root subtree onto the updated main.
	for _, r := range process {
		mb, err := h.git.MergeBase(ctx, mainRepoRoot, in.MainBranch, r)
		if err != nil {
			result.Status = "aborted"
			return result, err
		}
		migrated, err := h.pieces.RebaseSubtree(ctx, mainRepoRoot, piecesDir, r, in.MainBranch, mb)
		if err != nil {
			// Rebase left in progress; stashes stay stashed for `mp stack continue`.
			h.emit(core.MsgError, rebaseConflictMsg(r, in.MainBranch, filepath.Join(piecesDir, r)))
			result.Status = "aborted"
			return result, err
		}
		result.Updated = append(result.Updated, migrated...)
		if in.Push {
			for _, p := range migrated {
				wt := filepath.Join(piecesDir, p)
				h.emit(core.MsgWarning, forcePushMsg(p))
				if err := h.git.PushForceWithLease(ctx, wt); err != nil {
					h.emit(core.MsgWarning, fmt.Sprintf("Force-push failed for %q: %v", p, err))
				} else {
					result.Pushed = append(result.Pushed, p)
				}
			}
		}
	}

	// Restore stashes.
	for _, p := range members {
		if !stashed[p] {
			continue
		}
		wt := filepath.Join(piecesDir, p)
		if err := h.git.StashPop(ctx, wt); err != nil {
			h.emit(core.MsgWarning, stashPopConflictMsg(p, wt))
		}
	}

	return result, nil
}

// updateMain fetches the remote behind `from` and fast-forwards the local main
// branch to it. `from` is a ref like "origin/main"; its remote part is fetched
// and the branch part is the ref fetched. No-op when that remote isn't
// configured (e.g. a local-only repo, where local main is the source of truth).
func (h *Handler) updateMain(ctx context.Context, mainRepoRoot, mainBranch, from string) error {
	remote, ref := splitRemoteRef(from)

	remotes, err := h.git.Remotes(ctx, mainRepoRoot)
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}
	configured := false
	for _, r := range remotes {
		if r == remote {
			configured = true
			break
		}
	}
	if !configured {
		return nil
	}
	if err := h.git.Fetch(ctx, mainRepoRoot, remote, ref); err != nil {
		return err
	}
	if err := h.git.Checkout(ctx, mainRepoRoot, mainBranch); err != nil {
		return fmt.Errorf("failed to checkout %s in main repo: %w", mainBranch, err)
	}
	if err := h.git.MergeFFOnly(ctx, mainRepoRoot, from); err != nil {
		h.emit(core.MsgError, mainDivergedMsg(mainBranch, from, mainRepoRoot))
		return fmt.Errorf("local %s diverged from %s", mainBranch, from)
	}
	return nil
}

// splitRemoteRef splits a sync-from ref like "origin/main" into its remote
// ("origin") and branch ("main") parts. The split is on the first "/" so branch
// names that themselves contain slashes (e.g. "origin/feature/x") are preserved.
// A ref with no "/" is treated as a bare branch on no remote.
func splitRemoteRef(from string) (remote, ref string) {
	if i := strings.IndexByte(from, '/'); i >= 0 {
		return from[:i], from[i+1:]
	}
	return "", from
}

// scopeSet returns nil (all pieces) unless in.Stack, in which case it returns the
// set of pieces in the current piece's stack: ancestors + self + descendants.
func (h *Handler) scopeSet(ctx context.Context, workDir, piecesDir string, in SyncInput) (map[string]bool, error) {
	if !in.Stack {
		return nil, nil
	}
	st, err := h.pieces.Status(ctx, workDir)
	if err != nil {
		return nil, err
	}
	if !st.InPiece {
		return nil, fmt.Errorf("--stack requires running from inside a piece worktree")
	}
	set := map[string]bool{}
	// Ancestors up to main.
	cur := st.PieceName
	for cur != "" && cur != "main" {
		set[cur] = true
		meta, err := piece.ReadPieceMetadata(filepath.Join(piecesDir, cur), h.deps.FS)
		if err != nil {
			break
		}
		cur = meta.Parent
	}
	// Self + descendants.
	for _, p := range h.subtreeMembers(st.PieceName, piecesDir) {
		set[p] = true
	}
	return set, nil
}

// subtreeMembers returns name plus all transitive descendants.
func (h *Handler) subtreeMembers(name, piecesDir string) []string {
	out := []string{name}
	children, err := piece.GetPieceChildren(name, piecesDir, h.deps.FS)
	if err != nil {
		return out
	}
	sort.Strings(children)
	for _, c := range children {
		out = append(out, h.subtreeMembers(c, piecesDir)...)
	}
	return out
}

func inScope(scope map[string]bool, name string) bool {
	return scope == nil || scope[name]
}

// ---- Append / Prepend ------------------------------------------------------

// Append creates a new piece as a child of the current piece (a new branch on top).
func (h *Handler) Append(ctx context.Context, workDir string, in AppendInput) (piece.PieceInfo, error) {
	if err := ValidateAppendInput(in); err != nil {
		return piece.PieceInfo{}, err
	}
	st, err := h.pieces.Status(ctx, workDir)
	if err != nil {
		return piece.PieceInfo{}, err
	}
	if !st.InPiece {
		return piece.PieceInfo{}, fmt.Errorf("'mp stack append' must run from inside a piece worktree; use 'mp create' to start a new root piece")
	}
	mainRepoRoot, _, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return piece.PieceInfo{}, err
	}

	input := piece.WithNewPieceDefaults(piece.NewPieceInput{
		Name:       in.Name,
		Prompt:     in.Prompt,
		Parent:     st.PieceName,
		SkipSwitch: true,
	})
	if err := piece.ValidateNewPieceInput(input); err != nil {
		return piece.PieceInfo{}, err
	}
	return h.pieces.CreatePieceWithInput(ctx, input, piece.CreatePieceOptions{Parent: st.PieceName, RepoRoot: mainRepoRoot})
}

// Prepend inserts a new piece between the current piece and its parent. The new
// piece branches from the current parent's tip, and the current piece is re-pointed
// onto it. No history is rewritten at creation; run `mp stack sync` to restack.
func (h *Handler) Prepend(ctx context.Context, workDir string, in PrependInput) (piece.PieceInfo, error) {
	if err := ValidatePrependInput(in); err != nil {
		return piece.PieceInfo{}, err
	}
	st, err := h.pieces.Status(ctx, workDir)
	if err != nil {
		return piece.PieceInfo{}, err
	}
	if !st.InPiece {
		return piece.PieceInfo{}, fmt.Errorf("'mp stack prepend' must run from inside a piece worktree")
	}
	mainRepoRoot, _, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return piece.PieceInfo{}, err
	}

	curMeta, err := piece.ReadPieceMetadata(st.WorktreePath, h.deps.FS)
	if err != nil {
		return piece.PieceInfo{}, err
	}
	grandParent := curMeta.Parent
	if grandParent == "" {
		grandParent = "main"
	}

	input := piece.WithNewPieceDefaults(piece.NewPieceInput{
		Name:       in.Name,
		Prompt:     in.Prompt,
		Parent:     grandParent,
		SkipSwitch: true,
	})
	if err := piece.ValidateNewPieceInput(input); err != nil {
		return piece.PieceInfo{}, err
	}
	info, err := h.pieces.CreatePieceWithInput(ctx, input, piece.CreatePieceOptions{Parent: grandParent, RepoRoot: mainRepoRoot})
	if err != nil {
		return piece.PieceInfo{}, err
	}

	// Re-point the current piece onto the newly inserted piece.
	curMeta.Parent = info.Name
	if err := piece.WritePieceMetadata(st.WorktreePath, *curMeta, h.deps.FS); err != nil {
		return piece.PieceInfo{}, fmt.Errorf("created %q but failed to re-point %q onto it: %w", info.Name, st.PieceName, err)
	}
	h.emit(core.MsgSuccess, fmt.Sprintf("Inserted %q between %q and %q. Run 'mp stack sync' to restack.", info.Name, st.PieceName, grandParent))
	return info, nil
}

// ---- SetParent --------------------------------------------------------------

// SetParentResult reports a completed re-parenting.
type SetParentResult struct {
	Piece     string `json:"piece"`
	OldParent string `json:"old_parent"`
	NewParent string `json:"new_parent"`
}

// SetParent re-points a piece onto a new parent (another piece, or "main").
// Metadata-only: no history is rewritten; run `mp stack sync` to restack.
func (h *Handler) SetParent(ctx context.Context, workDir string, in SetParentInput) (SetParentResult, error) {
	var result SetParentResult
	if err := ValidateSetParentInput(in); err != nil {
		return result, err
	}
	_, piecesDir, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return result, err
	}

	pieceName := strings.TrimSpace(in.Piece)
	if pieceName == "" {
		st, err := h.pieces.Status(ctx, workDir)
		if err != nil {
			return result, err
		}
		if !st.InPiece {
			return result, fmt.Errorf("no piece given: pass --piece, or run from inside a piece worktree")
		}
		pieceName = st.PieceName
	}

	// ReadPieceMetadata returns defaults for missing files, so existence is
	// checked on the worktree directories themselves.
	worktree := filepath.Join(piecesDir, pieceName)
	if _, err := h.deps.FS.Stat(worktree); err != nil {
		return result, fmt.Errorf("unknown piece %q: no worktree at %s", pieceName, worktree)
	}
	meta, err := piece.ReadPieceMetadata(worktree, h.deps.FS)
	if err != nil {
		return result, fmt.Errorf("unknown piece %q: %w", pieceName, err)
	}

	newParent := strings.TrimSpace(in.Parent)
	if newParent == pieceName {
		return result, fmt.Errorf("cannot make %q its own parent", pieceName)
	}
	if newParent != "main" {
		if _, err := h.deps.FS.Stat(filepath.Join(piecesDir, newParent)); err != nil {
			return result, fmt.Errorf("unknown parent piece %q (use \"main\" for a root piece)", newParent)
		}
	}
	for _, d := range h.subtreeMembers(pieceName, piecesDir) {
		if d == newParent {
			return result, fmt.Errorf("%q is a descendant of %q; re-parenting would create a cycle", newParent, pieceName)
		}
	}

	oldParent := meta.Parent
	if oldParent == "" {
		oldParent = "main"
	}
	result = SetParentResult{Piece: pieceName, OldParent: oldParent, NewParent: newParent}
	if oldParent == newParent {
		h.emit(core.MsgInfo, fmt.Sprintf("%q already has parent %q; nothing to do.", pieceName, newParent))
		return result, nil
	}

	meta.Parent = newParent
	if err := piece.WritePieceMetadata(worktree, *meta, h.deps.FS); err != nil {
		return result, fmt.Errorf("failed to write piece metadata: %w", err)
	}
	h.emit(core.MsgSuccess, fmt.Sprintf("Re-parented %q: %s -> %s. Run 'mp stack sync' to restack.", pieceName, oldParent, newParent))
	return result, nil
}

// ---- Continue --------------------------------------------------------------

// Continue resumes a single conflicted rebase in workDir (after the user resolved
// the conflicts), then directs them to re-run sync.
func (h *Handler) Continue(ctx context.Context, workDir string) (SyncResult, error) {
	result := SyncResult{Status: "synced"}
	inProgress, err := h.git.RebaseInProgress(ctx, workDir)
	if err != nil {
		return result, err
	}
	if !inProgress {
		return result, fmt.Errorf("no rebase in progress in %s; nothing to continue", workDir)
	}
	if err := h.git.RebaseContinue(ctx, workDir); err != nil {
		h.emit(core.MsgError, rebaseStillConflictedMsg(workDir))
		result.Status = "aborted"
		return result, err
	}
	h.emit(core.MsgSuccess, "Rebase continued. Re-run 'mp stack sync' to finish syncing the rest of the stack.")
	return result, nil
}

// ---- Plain-English fallback messages ---------------------------------------

func mergeConflictMsg(parentBranch, piece, path string) string {
	return fmt.Sprintf("Stack sync stopped: merging %s into %q hit conflicts. Open %s, resolve the conflicts, run 'git commit', then run 'mp stack sync' again. Nothing else in the stack was changed.", parentBranch, piece, path)
}

func rebaseConflictMsg(piece, parentBranch, path string) string {
	return fmt.Sprintf("Stack sync stopped: rebasing %q onto %s hit conflicts. The rebase is in progress in %s. Resolve the conflicts, then run 'mp stack continue'. Or run 'git rebase --abort' there to back out.", piece, parentBranch, path)
}

func rebaseInProgressMsg(path string) string {
	return fmt.Sprintf("Stack sync aborted: a rebase is already in progress in %s. Finish it (git rebase --continue) or abort it (git rebase --abort) before syncing.", path)
}

func mainDivergedMsg(mainBranch, from, repoRoot string) string {
	return fmt.Sprintf("Stack sync aborted: local %s has diverged from %s and can't fast-forward (it was likely force-pushed). Fix: 'git -C %s checkout %s && git reset --hard %s', then run 'mp stack sync'.", mainBranch, from, repoRoot, mainBranch, from)
}

func stashPopConflictMsg(piece, path string) string {
	return fmt.Sprintf("Stack synced %q, but restoring your uncommitted changes hit conflicts in %s. Your work is safe — resolve the conflict markers there (your stash is also recoverable via 'git stash list'). The rest of the stack was synced.", piece, path)
}

func forgeUnavailableMsg() string {
	return "The PR/MR provider is unavailable (gh/glab not installed or not logged in). Showing local lineage only; PR/MR state and drift checks are skipped."
}

func forcePushMsg(piece string) string {
	return fmt.Sprintf("Rebasing rewrote history on %q. Force-pushing with --force-with-lease. If a PR review exists, its comments may detach from the new commits.", piece)
}

func rebaseStillConflictedMsg(path string) string {
	return fmt.Sprintf("Rebase still has unresolved conflicts in %s. Resolve them (the conflicted files), 'git add' them, then run 'mp stack continue' again. Or 'git rebase --abort' to back out.", path)
}
