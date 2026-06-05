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
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

const (
	// DefaultDirPerm is the default permission for directories (0755 = rwxr-xr-x)
	DefaultDirPerm = 0755
)

// Handler executes piece-related commands
type Handler struct {
	deps   core.Deps
	git    *adapters.Git
	github *adapters.GitHub
	mux    core.Multiplexer
	hooks  *HookRunner
}

// NewHandler creates a new piece handler with dependencies.
// Uses NoopMultiplexer by default. Use NewHandlerWithMultiplexer to specify a multiplexer.
func NewHandler(deps core.Deps) *Handler {
	return NewHandlerWithMultiplexer(deps, adapters.NewNoopMultiplexer())
}

// NewHandlerWithMultiplexer creates a new piece handler with a specific multiplexer.
func NewHandlerWithMultiplexer(deps core.Deps, mux core.Multiplexer) *Handler {
	return &Handler{
		deps:   deps,
		git:    adapters.NewGit(deps.Exec),
		github: adapters.NewGitHub(deps.Exec),
		mux:    mux,
		hooks:  NewHookRunner(deps),
	}
}

// CreatePieceOptions configures piece creation behavior
type CreatePieceOptions struct {
	OverwriteSession bool   // If true, replace existing main repo session
	Parent           string // Parent piece name, defaults to "main"
	// RepoRoot, when set, anchors creation at this main repo root instead of
	// resolving from the current working directory. Lets callers create a piece
	// from inside a worktree (e.g. `mp stack append`) without scoping the pieces
	// dir to the worktree.
	RepoRoot string
}

// CreatePieceWithInput creates a piece from validated input.
// Routes to CreatePieceFromIssue if Issue is set, CreatePieceFromPrompt if Prompt is set, otherwise CreatePiece.
func (h *Handler) CreatePieceWithInput(ctx context.Context, input NewPieceInput, opts CreatePieceOptions) (PieceInfo, error) {
	// Pass parent from input to options
	opts.Parent = input.Parent
	if opts.Parent == "" {
		opts.Parent = "main"
	}

	if !input.Issue.IsEmpty() {
		return h.CreatePieceFromIssue(ctx, input.Issue, opts)
	}
	if input.Prompt != "" {
		return h.CreatePieceFromPrompt(ctx, input.Name, input.Prompt, opts)
	}
	return h.CreatePiece(ctx, input.Name, opts)
}

// CreatePiece creates a new git worktree for a piece.
// If pieceName is provided and non-empty, it will be used (after checking it doesn't exist).
// If pieceName is empty, a name will be generated automatically.
// Session management is handled separately by SwitchPiece.
func (h *Handler) CreatePiece(ctx context.Context, pieceName string, opts CreatePieceOptions) (PieceInfo, error) {
	// Resolve repo root: prefer an explicit one (lets callers create from inside a
	// worktree), otherwise detect from the current working directory.
	var wd string
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot != "" {
		wd = repoRoot
	} else {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
		}
		repoRoot, err = h.git.RepoRoot(ctx, wd)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
		}
	}

	// Get current branch before creating worktree (for piece metadata)
	currentBranch, err := h.git.CurrentBranch(ctx, wd)
	if err != nil {
		// Non-fatal: use empty string if we can't determine branch
		currentBranch = ""
	}

	// Get pieces directory (scoped to this repo)
	piecesDir, err := getPiecesDir(repoRoot)
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

	// Determine parent and start point for worktree
	parent := opts.Parent
	if parent == "" {
		parent = "main"
	}

	// Create worktree
	worktreePath := filepath.Join(piecesDir, pieceName)
	if parent != "main" {
		// Branching from a parent piece - validate it exists and get its branch
		parentPath := filepath.Join(piecesDir, parent)
		if _, err := h.deps.FS.Stat(parentPath); err != nil {
			return PieceInfo{}, fmt.Errorf("parent piece %q not found", parent)
		}
		// The parent's branch name is the same as the parent piece name; the new
		// piece gets its own branch named after the piece.
		if err := h.git.WorktreeAddFrom(ctx, repoRoot, worktreePath, pieceName, parent); err != nil {
			return PieceInfo{}, fmt.Errorf("failed to create worktree from parent %s: %w", parent, err)
		}
	} else {
		if err := h.git.WorktreeAdd(ctx, repoRoot, worktreePath); err != nil {
			return PieceInfo{}, fmt.Errorf("failed to create worktree at %s: %w", worktreePath, err)
		}
	}

	// Write piece metadata (parent-child relationship)
	pieceMetadata := PieceMetadata{
		Parent:            parent,
		CreatedFromBranch: currentBranch,
	}
	if err := WritePieceMetadata(worktreePath, pieceMetadata, h.deps.FS); err != nil {
		// Non-fatal: log warning but continue
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to write piece metadata: %v", err),
		})
	}

	sessionName := h.pieceSessionName(repoRoot, pieceName)
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
		// A failing on-piece-create hook is non-fatal: keep the worktree and
		// continue. Setup steps (dependency installs, submodule init) can fail
		// for transient reasons, and discarding the worktree is worse than a
		// warning the user can act on.
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("on-piece-create hook failed (piece kept): %v", err),
		})
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Created piece: %s at %s", pieceName, worktreePath),
		Data:    info,
	})

	return info, nil
}

// AdoptPiece converts an existing git branch into a piece.
// Must be run from the main repo (not a worktree).
// If input.Branch is provided, adopts that branch; otherwise uses current branch.
// Creates a worktree for the branch and sets up piece metadata.
func (h *Handler) AdoptPiece(ctx context.Context, input AdoptPieceInput) (PieceInfo, error) {
	// Determine the directory we use to probe git state. If the caller passes
	// an explicit RepoRoot, we trust it and skip the cwd-based discovery — this
	// lets callers like `mp switch` adopt a branch in a project other than the
	// one they're running from.
	var wd string
	var repoRoot string
	if strings.TrimSpace(input.RepoRoot) != "" {
		repoRoot = strings.TrimSpace(input.RepoRoot)
		wd = repoRoot
	} else {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return PieceInfo{}, fmt.Errorf("failed to get working directory: %w", err)
		}
		repoRoot, err = h.git.GetMainRepoRoot(ctx, wd)
		if err != nil {
			repoRoot, err = h.git.RepoRoot(ctx, wd)
			if err != nil {
				return PieceInfo{}, fmt.Errorf("not in a git repository: %w", err)
			}
		}
	}

	// Detect if we're inside a worktree — affects defaulting and clean check.
	// When the caller provides RepoRoot we treat it as a main repo (not a
	// worktree); the new entrypoint never points us at a worktree.
	gitDir, err := h.git.RevParseGitDir(ctx, wd)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get git dir: %w", err)
	}
	inWorktree := h.git.IsWorktree(gitDir)

	// Determine branch to adopt. From a worktree, the current branch is the
	// piece's own branch (already a piece), so require an explicit --branch.
	branchToAdopt := strings.TrimSpace(input.Branch)
	if branchToAdopt == "" {
		if inWorktree {
			return PieceInfo{}, fmt.Errorf("--branch is required when running from inside a piece worktree")
		}
		branchToAdopt, err = h.git.CurrentBranch(ctx, wd)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	// Resolve remote-ref form (e.g. "origin/foo") by consulting the actual remotes.
	remoteName, remoteBranch := h.detectRemoteRef(ctx, repoRoot, branchToAdopt)
	isRemote := remoteName != ""

	// Disallow adopting main/master branches (after stripping the remote prefix).
	plainBranch := branchToAdopt
	if isRemote {
		plainBranch = remoteBranch
	}
	if plainBranch == "main" || plainBranch == "master" {
		return PieceInfo{}, fmt.Errorf("cannot adopt main or master branch as a piece")
	}

	// Only enforce a clean working directory when running from the main repo,
	// since the adopt path used to checkout there. From a worktree we don't
	// touch wd at all.
	if !inWorktree {
		clean, err := h.git.IsClean(ctx, wd)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("failed to check working directory status: %w", err)
		}
		if !clean {
			return PieceInfo{}, fmt.Errorf("working directory has uncommitted changes, please commit or stash first")
		}
	}

	// Determine piece name (use input.Name or branch name).
	pieceName := input.Name
	if pieceName == "" {
		pieceName = SanitizePieceName(plainBranch)
	} else {
		pieceName = SanitizePieceName(pieceName)
	}

	// Get pieces directory
	piecesDir, err := getPiecesDir(repoRoot)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	// Check if piece name already exists
	piecePath := filepath.Join(piecesDir, pieceName)
	if _, err := h.deps.FS.Stat(piecePath); err == nil {
		return PieceInfo{}, fmt.Errorf("piece name %q already exists at %s", pieceName, piecePath)
	}

	// Create pieces directory if it doesn't exist
	if err := h.deps.FS.MkdirAll(piecesDir, DefaultDirPerm); err != nil {
		return PieceInfo{}, fmt.Errorf("failed to create pieces directory: %w", err)
	}

	// Determine parent
	parent := input.Parent
	if parent == "" {
		parent = "main"
	}

	worktreePath := filepath.Join(piecesDir, pieceName)

	if isRemote {
		// Fetch the remote branch, then add a worktree with a new local branch
		// tracking it.
		if err := h.git.Fetch(ctx, repoRoot, remoteName, remoteBranch); err != nil {
			return PieceInfo{}, err
		}
		if h.git.LocalBranchExists(ctx, repoRoot, remoteBranch) {
			return PieceInfo{}, fmt.Errorf("local branch %q already exists; pass --name to adopt under a different piece name, or adopt the local branch directly", remoteBranch)
		}
		if err := h.git.WorktreeAddTracking(ctx, repoRoot, worktreePath, remoteBranch, branchToAdopt); err != nil {
			return PieceInfo{}, err
		}
		// Record the local branch we just created as the adopted branch for
		// metadata + the success message.
		branchToAdopt = remoteBranch
	} else {
		// Local branch path. If the branch is currently checked out in the main
		// repo (the historical default), the old code would checkout main on wd
		// first; now we just surface git's own error so the user can sort it
		// out. From a worktree we don't touch wd.
		if err := h.git.WorktreeAddExisting(ctx, repoRoot, worktreePath, branchToAdopt); err != nil {
			return PieceInfo{}, fmt.Errorf("failed to create worktree for branch %s: %w", branchToAdopt, err)
		}
	}

	// Write piece metadata
	pieceMetadata := PieceMetadata{
		Parent:            parent,
		CreatedFromBranch: branchToAdopt,
	}
	if err := WritePieceMetadata(worktreePath, pieceMetadata, h.deps.FS); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to write piece metadata: %v", err),
		})
	}

	sessionName := h.pieceSessionName(repoRoot, pieceName)
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
		// Non-fatal: keep the adopted worktree and warn. See CreatePiece.
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("on-piece-create hook failed (piece kept): %v", err),
		})
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Adopted branch %s as piece: %s at %s", branchToAdopt, pieceName, worktreePath),
		Data:    info,
	})

	return info, nil
}

// detectRemoteRef checks if branchRef has the form "<remote>/<name>" where
// <remote> is a configured git remote. Returns the remote and branch name when
// it matches; otherwise returns ("", "") so the caller treats branchRef as a
// local branch (which may itself contain slashes, e.g. "feature/foo").
func (h *Handler) detectRemoteRef(ctx context.Context, repoRoot, branchRef string) (string, string) {
	idx := strings.Index(branchRef, "/")
	if idx <= 0 || idx == len(branchRef)-1 {
		return "", ""
	}
	candidate := branchRef[:idx]
	remotes, err := h.git.Remotes(ctx, repoRoot)
	if err != nil {
		return "", ""
	}
	for _, r := range remotes {
		if r == candidate {
			return candidate, branchRef[idx+1:]
		}
	}
	return "", ""
}

// CurrentIssueMarker represents the current issue marker file structure
type CurrentIssueMarker struct {
	Issue     IssueRef `json:"issue"`
	PieceName string   `json:"piece_name"`
	Status    string   `json:"status,omitempty"` // Local status, synced via `mp issue sync`
	Dirty     bool     `json:"dirty,omitempty"`  // True if status has unsynced changes
}

// CreatePieceFromIssue creates a new piece linked to an issue.
// The caller is responsible for looking up the issue via the provider.
func (h *Handler) CreatePieceFromIssue(ctx context.Context, issueRef IssueRef, opts CreatePieceOptions) (PieceInfo, error) {
	if issueRef.IsEmpty() {
		return PieceInfo{}, fmt.Errorf("issue reference is empty")
	}

	// Sanitize issue title for piece name
	pieceName := SanitizePieceName(issueRef.Title)

	// Create the piece using the sanitized name
	info, err := h.CreatePiece(ctx, pieceName, opts)
	if err != nil {
		return PieceInfo{}, err
	}

	// Write current issue marker file in worktree
	marker := CurrentIssueMarker{
		Issue:     issueRef,
		PieceName: pieceName,
		Status:    StatusInProgress,
		Dirty:     true, // Will be cleared after sync
	}
	if err := h.writeCurrentIssueMarker(info.WorktreePath, marker); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to write current issue marker: %v", err),
		})
	}

	// Publish sync event to update issue status in provider
	h.pubIssueSync(core.IssueSyncEvent{
		Provider:     issueRef.Provider,
		IssueID:      issueRef.ID,
		NewStatus:    StatusInProgress,
		PieceName:    pieceName,
		WorktreePath: info.WorktreePath,
	})

	return info, nil
}

// CreatePieceFromPrompt creates a new piece, recording the prompt it was
// created from in the piece's (gitignored) metadata.
func (h *Handler) CreatePieceFromPrompt(ctx context.Context, name, prompt string, opts CreatePieceOptions) (PieceInfo, error) {
	if prompt == "" {
		return PieceInfo{}, fmt.Errorf("prompt is empty")
	}

	// Derive name from prompt if not provided
	if name == "" {
		name = SanitizePieceName(prompt)
	}

	// Create the piece worktree
	info, err := h.CreatePiece(ctx, name, opts)
	if err != nil {
		return PieceInfo{}, err
	}

	// Record the prompt in piece metadata so `mp status` can surface it.
	metadata, err := ReadPieceMetadata(info.WorktreePath, h.deps.FS)
	if err != nil {
		def := DefaultPieceMetadata()
		metadata = &def
	}
	metadata.Prompt = prompt
	if err := WritePieceMetadata(info.WorktreePath, *metadata, h.deps.FS); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to record piece prompt: %v", err),
		})
	}

	return info, nil
}

// writeCurrentIssueMarker writes the current issue marker file to the worktree.
func (h *Handler) writeCurrentIssueMarker(worktreePath string, marker CurrentIssueMarker) error {
	// Create the monkeypuzzle state directory in the worktree if it doesn't exist
	mpDir, err := projectdir.WorktreeDir(worktreePath)
	if err != nil {
		return err
	}
	if err := h.deps.FS.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create monkeypuzzle directory: %w", err)
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

// pubIssueSync publishes an issue sync event if IssueSync is configured.
func (h *Handler) pubIssueSync(event core.IssueSyncEvent) {
	if h.deps.IssueSync != nil {
		h.deps.IssueSync.Pub(event)
	}
}

// cleanupPiece removes a partially created piece (worktree and session).
// Errors during cleanup are logged as warnings but not returned.
func (h *Handler) cleanupPiece(ctx context.Context, repoRoot, worktreePath, sessionName string, _ bool) {
	// Kill session if it exists
	if h.mux.Exists(ctx, sessionName) {
		if err := h.mux.Kill(ctx, sessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to cleanup session: %v", err),
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

// GetPieceHierarchyStatus returns piece status with hierarchy information.
// It includes parent, children, stack depth, and merge readiness.
func (h *Handler) GetPieceHierarchyStatus(ctx context.Context, workDir, mainBranch string) (PieceHierarchyStatus, error) {
	status, err := h.Status(ctx, workDir)
	if err != nil {
		return PieceHierarchyStatus{}, err
	}

	result := PieceHierarchyStatus{
		PieceStatus: status,
		Children:    []string{},
		StackDepth:  0,
		CanMerge:    true,
	}

	// If not in a piece, return basic status
	if !status.InPiece {
		return result, nil
	}

	// Get pieces directory
	piecesDir, err := getPiecesDir(status.RepoRoot)
	if err != nil {
		return result, fmt.Errorf("failed to get pieces directory: %w", err)
	}

	// Read piece metadata to get parent
	metadata, err := ReadPieceMetadata(status.WorktreePath, h.deps.FS)
	if err != nil {
		// Non-fatal: use default parent
		defaultMeta := DefaultPieceMetadata()
		metadata = &defaultMeta
	}
	result.Parent = metadata.Parent

	// Get children
	children, err := GetPieceChildren(status.PieceName, piecesDir, h.deps.FS)
	if err != nil {
		// Non-fatal: continue without children info
		children = []string{}
	}
	result.Children = children

	// Calculate stack depth by traversing up the parent chain
	result.StackDepth = h.calculateStackDepth(status.PieceName, piecesDir, metadata.Parent)

	// Check if can merge: no children OR all children are merged
	if len(children) == 0 {
		result.CanMerge = true
	} else {
		// Check if all children are merged
		allMerged := true
		for _, childName := range children {
			childPath := filepath.Join(piecesDir, childName)
			branchName, err := h.git.CurrentBranch(ctx, childPath)
			if err != nil {
				// If we can't get branch, assume not merged
				allMerged = false
				break
			}

			mergeStatus, err := h.IsBranchMerged(ctx, childPath, branchName, mainBranch)
			if err != nil || !mergeStatus.IsMerged {
				allMerged = false
				break
			}
		}
		result.CanMerge = allMerged
	}

	return result, nil
}

// calculateStackDepth calculates the stack depth by traversing up the parent chain.
// Returns 1 for direct children of main, 2 for grandchildren, etc.
func (h *Handler) calculateStackDepth(pieceName, piecesDir, parent string) int {
	if parent == "main" {
		return 1
	}

	// Traverse up the parent chain
	depth := 1
	currentParent := parent
	visited := make(map[string]bool) // Prevent infinite loops

	for currentParent != "main" && !visited[currentParent] {
		visited[currentParent] = true
		depth++

		// Read parent's metadata
		parentPath := filepath.Join(piecesDir, currentParent)
		metadata, err := ReadPieceMetadata(parentPath, h.deps.FS)
		if err != nil {
			// If we can't read parent metadata, stop traversing
			break
		}

		currentParent = metadata.Parent
	}

	return depth
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

// MergePiece squash-merges the piece branch into its target branch.
// For root pieces (parent=main), merges into main.
// For child pieces, merges into the parent piece's branch.
// Blocks if piece has unmerged children unless Force is set.
func (h *Handler) MergePiece(ctx context.Context, workDir string, input MergeInput) (MergeResult, error) {
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

	// Read piece metadata to determine parent
	pieceMetadata, err := ReadPieceMetadata(status.WorktreePath, h.deps.FS)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to read piece metadata: %w", err)
	}

	// Determine target branch: parent piece's branch or main
	targetBranch := input.MainBranch
	if pieceMetadata.Parent != "main" && pieceMetadata.Parent != "" {
		targetBranch = pieceMetadata.Parent
		h.deps.Output.Write(core.Message{
			Type:    core.MsgInfo,
			Content: fmt.Sprintf("Merging into parent piece '%s'", pieceMetadata.Parent),
		})
	}

	// Determine direct children of this piece.
	piecesDir, err := getPiecesDir(mainRepoRoot)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get pieces directory: %w", err)
	}
	children, err := GetPieceChildren(status.PieceName, piecesDir, h.deps.FS)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to check for children: %w", err)
	}
	if len(children) > 0 && !input.Force && !input.ReparentChildren {
		return MergeResult{}, fmt.Errorf("cannot merge: piece has child pieces: %v. Merge them first, or pass --reparent-children to rebase them onto %s and merge anyway", children, targetBranch)
	}
	// When re-homing children, every descendant worktree must be clean (rebase
	// would refuse otherwise, possibly leaving the stack half-migrated).
	if len(children) > 0 && input.ReparentChildren {
		if err := h.ensureSubtreeClean(ctx, piecesDir, children); err != nil {
			return MergeResult{}, err
		}
	}

	// Build hook context
	hookCtx := HookContext{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		RepoRoot:     mainRepoRoot,
		MainBranch:   targetBranch,
	}

	// Run before-piece-merge hook
	if err := h.hooks.RunHook(ctx, mainRepoRoot, HookBeforePieceMerge, hookCtx); err != nil {
		return MergeResult{}, fmt.Errorf("before-piece-merge hook failed: %w", err)
	}

	// Check if target branch has commits not in the piece branch
	isAhead, err := h.git.IsMainAhead(ctx, mainRepoRoot, targetBranch, pieceBranch)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to check if target is ahead: %w", err)
	}

	if isAhead {
		return MergeResult{}, fmt.Errorf("cannot merge: %s has commits not in piece worktree. Run 'mp update' first", targetBranch)
	}

	// Get commit messages from piece branch for the squash commit message
	commitMsgs, err := h.git.GetCommitMessages(ctx, mainRepoRoot, targetBranch, pieceBranch)
	if err != nil {
		return MergeResult{}, fmt.Errorf("failed to get commit messages: %w", err)
	}

	// Build squash commit message
	commitMsg := h.buildSquashCommitMessage(status.PieceName, commitMsgs)

	// Switch to target branch
	if err := h.git.Checkout(ctx, mainRepoRoot, targetBranch); err != nil {
		return MergeResult{}, fmt.Errorf("failed to checkout %s: %w", targetBranch, err)
	}

	// Squash merge the piece branch into target
	if err := h.git.MergeSquash(ctx, mainRepoRoot, pieceBranch); err != nil {
		return MergeResult{}, fmt.Errorf("failed to squash merge piece branch into %s: %w", targetBranch, err)
	}

	// Commit the squashed changes
	if err := h.git.Commit(ctx, mainRepoRoot, commitMsg); err != nil {
		return MergeResult{}, fmt.Errorf("failed to commit squashed changes: %w", err)
	}

	// Run after-piece-merge hook
	if err := h.hooks.RunHook(ctx, mainRepoRoot, HookAfterPieceMerge, hookCtx); err != nil {
		return MergeResult{}, fmt.Errorf("after-piece-merge hook failed: %w", err)
	}

	// Re-home child pieces onto the merge target, if requested. The merged piece's
	// branch (pieceBranch) still points at its pre-merge tip — the cut point for a
	// rebase of its direct children.
	strategy := input.ReparentStrategy
	if strategy == "" {
		strategy = ReparentRebase
	}
	var reparented []string
	if input.ReparentChildren && len(children) > 0 {
		newParent := pieceMetadata.Parent
		if newParent == "" {
			newParent = "main"
		}
		for _, child := range children {
			var migrated []string
			switch strategy {
			case ReparentMerge:
				if err := h.mergeIntoChild(ctx, piecesDir, child, targetBranch); err != nil {
					return MergeResult{}, fmt.Errorf("piece merged into %s, but re-homing child %q failed: %w", targetBranch, child, err)
				}
				migrated = []string{child}
			default: // ReparentRebase
				m, err := h.rebaseSubtree(ctx, mainRepoRoot, piecesDir, child, targetBranch, pieceBranch)
				if err != nil {
					return MergeResult{}, fmt.Errorf("piece merged into %s, but re-homing children failed: %w", targetBranch, err)
				}
				migrated = m
			}
			childPath := filepath.Join(piecesDir, child)
			meta, err := ReadPieceMetadata(childPath, h.deps.FS)
			if err != nil {
				return MergeResult{}, fmt.Errorf("piece merged, but failed to read metadata for child %q: %w", child, err)
			}
			meta.Parent = newParent
			if err := WritePieceMetadata(childPath, *meta, h.deps.FS); err != nil {
				return MergeResult{}, fmt.Errorf("piece merged, but failed to update metadata for child %q: %w", child, err)
			}
			reparented = append(reparented, migrated...)
			note := fmt.Sprintf("Re-homed %s onto %s", child, targetBranch)
			if strategy == ReparentRebase {
				note += " (force-push it if it has an open PR)"
			}
			h.deps.Output.Write(core.Message{Type: core.MsgInfo, Content: note})
		}
	}

	result := MergeResult{
		PieceName:          status.PieceName,
		PieceBranch:        pieceBranch,
		TargetBranch:       targetBranch,
		Status:             "merged",
		ReparentedChildren: reparented,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Squash merged %s into %s", pieceBranch, targetBranch),
		Data:    result,
	})

	return result, nil
}

// buildSquashCommitMessage creates a commit message for squash merge
func (h *Handler) buildSquashCommitMessage(pieceName string, commitMsgs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "feat: %s\n", pieceName)

	if len(commitMsgs) > 0 {
		b.WriteString("\nSquashed commits:\n")
		for _, msg := range commitMsgs {
			fmt.Fprintf(&b, "- %s\n", msg)
		}
	}

	return b.String()
}

// getPiecesDir returns the directory for storing pieces scoped to the given repo.
// Honors any relocation of the monkeypuzzle directory.
func getPiecesDir(repoRoot string) (string, error) {
	return projectdir.PiecesDir(repoRoot)
}

// currentIssueMarkerPath returns the path to a worktree's current-issue.json,
// honoring any relocation of the monkeypuzzle directory.
func currentIssueMarkerPath(worktreePath string) (string, error) {
	mpDir, err := projectdir.WorktreeDir(worktreePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(mpDir, "current-issue.json"), nil
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

	status.Method = "none"
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
	PieceName    string   `json:"piece_name"`
	WorktreePath string   `json:"worktree_path"`
	Issue        IssueRef `json:"issue,omitempty"`
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
	// Get pieces directory (scoped to this repo)
	piecesDir, err := getPiecesDir(repoRoot)
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
		marker, err := h.ReadCurrentIssueMarker(worktreePath)
		if err == nil && marker != nil && !marker.Issue.IsEmpty() {
			result.Issue = marker.Issue

			// Publish sync event to mark issue as done
			h.pubIssueSync(core.IssueSyncEvent{
				Provider:     marker.Issue.Provider,
				IssueID:      marker.Issue.ID,
				NewStatus:    StatusDone,
				PieceName:    pieceName,
				WorktreePath: worktreePath,
			})
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

		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: fmt.Sprintf("Cleaned up: %s", pieceName),
		})

		results = append(results, result)
	}

	return results, nil
}

// ReadCurrentIssueMarker reads the current issue marker from a piece worktree.
func (h *Handler) ReadCurrentIssueMarker(worktreePath string) (*CurrentIssueMarker, error) {
	return ReadCurrentIssueMarkerFS(worktreePath, h.deps.FS)
}

// ReadCurrentIssueMarkerFS reads the current issue marker from a worktree using a FS interface.
// This standalone function is useful when you don't have a Handler instance.
func ReadCurrentIssueMarkerFS(worktreePath string, fs core.FS) (*CurrentIssueMarker, error) {
	markerPath, err := currentIssueMarkerPath(worktreePath)
	if err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(markerPath)
	if err != nil {
		return nil, err
	}

	var marker CurrentIssueMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, err
	}

	return &marker, nil
}

// ClearIssueDirtyFlag clears the dirty flag on the issue marker in a worktree.
// Called after successful sync to provider.
func ClearIssueDirtyFlag(worktreePath string, fs core.FS) error {
	marker, err := ReadCurrentIssueMarkerFS(worktreePath, fs)
	if err != nil {
		return err
	}

	if !marker.Dirty {
		return nil // Already clean
	}

	marker.Dirty = false

	// Write updated marker
	markerPath, err := currentIssueMarkerPath(worktreePath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal marker: %w", err)
	}

	if err := fs.WriteFile(markerPath, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write marker file: %w", err)
	}

	return nil
}

// removePiece removes a piece worktree and associated session.
func (h *Handler) removePiece(ctx context.Context, repoRoot, pieceName, worktreePath string) error {
	sessionName := h.pieceSessionName(repoRoot, pieceName)

	// Kill session (ignore errors - session may not exist)
	_ = h.mux.Kill(ctx, sessionName)

	// Remove worktree
	if err := h.git.WorktreeRemove(ctx, repoRoot, worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// removePieceForce removes a piece worktree forcefully (for removing from inside the worktree).
func (h *Handler) removePieceForce(ctx context.Context, repoRoot, pieceName, worktreePath string) error {
	sessionName := h.pieceSessionName(repoRoot, pieceName)

	// Kill session (ignore errors - session may not exist)
	_ = h.mux.Kill(ctx, sessionName)

	// Force remove worktree (needed when running from inside the worktree)
	if err := h.git.WorktreeRemoveForce(ctx, repoRoot, worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// switchClientToMainIfInside moves the active multiplexer client to the main
// repo session when the caller is running from inside the worktree about to
// be removed. Without this, abandoning/finishing the piece you're currently
// attached to strands the client in a dying session.
func (h *Handler) switchClientToMainIfInside(ctx context.Context, mainRepoRoot, worktreePath string) {
	if adapters.IsNoopMultiplexer(h.mux) || !h.mux.InSession() {
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	if !isPathInside(wd, worktreePath) {
		return
	}
	mainSession := h.mainSessionName(mainRepoRoot)
	if err := h.mux.SwitchTo(ctx, mainSession, mainRepoRoot); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Failed to switch to main session: %v", err),
		})
	}
}

// isPathInside reports whether child is the same as or nested under parent.
func isPathInside(child, parent string) bool {
	absChild, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absParent, absChild)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	MainPath      string `json:"main_path,omitempty"`
}

// AbandonPiece removes a piece worktree, tmux session, and optionally the branch.
// Unlike cleanup, this works on unmerged pieces.
func (h *Handler) AbandonPiece(ctx context.Context, pieceName string, opts AbandonOptions) (AbandonResult, error) {
	result := AbandonResult{PieceName: pieceName}

	// Detect repo root from current working directory first
	// Use GetMainRepoRoot to handle running from within a worktree
	repoRoot := ""
	wd, err := os.Getwd()
	if err == nil {
		// Try GetMainRepoRoot first (handles worktrees)
		detectedRoot, err := h.git.GetMainRepoRoot(ctx, wd)
		if err != nil {
			// Fallback to RepoRoot
			detectedRoot, err = h.git.RepoRoot(ctx, wd)
			if err == nil {
				repoRoot = detectedRoot
			}
		} else {
			repoRoot = detectedRoot
		}
	}

	// Find the piece
	pieces, err := h.ListPieces(ctx, repoRoot)
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

	// Get repo root from piece worktree (more reliable than current directory)
	detectedRepoRoot, err := h.git.GetMainRepoRoot(ctx, target.WorktreePath)
	if err != nil {
		return result, fmt.Errorf("failed to get repo root: %w", err)
	}
	repoRoot = detectedRepoRoot

	// If we're running inside the worktree being removed, switch the active
	// client to the main repo session before killing this one.
	h.switchClientToMainIfInside(ctx, repoRoot, target.WorktreePath)

	// Kill session if exists
	if target.HasSession {
		if err := h.mux.Kill(ctx, target.SessionName); err != nil {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to kill session: %v", err),
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

	// Set main repo path for caller to navigate to
	result.MainPath = repoRoot

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Abandoned piece: %s", pieceName),
		Data:    result,
	})

	return result, nil
}

// FlattenOptions configures the flatten behavior.
type FlattenOptions struct {
	Force          bool // Force removal even with uncommitted changes
	DeleteBranches bool // Also delete each piece's git branch
	DryRun         bool // Only report what would be removed
}

// FlattenItem describes the outcome for a single piece during a flatten.
type FlattenItem struct {
	PieceName     string `json:"piece_name"`
	WorktreePath  string `json:"worktree_path"`
	BranchName    string `json:"branch_name,omitempty"`
	BranchDeleted bool   `json:"branch_deleted,omitempty"`
	Error         string `json:"error,omitempty"`
}

// FlattenResult contains the result of a flatten operation.
type FlattenResult struct {
	Removed  []FlattenItem `json:"removed"`          // pieces removed (or that would be removed, in dry-run)
	Failed   []FlattenItem `json:"failed,omitempty"` // pieces that could not be removed
	Count    int           `json:"count"`            // number removed
	MainPath string        `json:"main_path"`        // main repo root to navigate back to
	DryRun   bool          `json:"dry_run,omitempty"`
}

// FlattenPieces removes every piece worktree for the repo, returning it to a
// flat main-only state. Unlike CleanupMergedPieces it does not check merge
// status, and unlike AbandonPiece it operates on all pieces at once. A failure
// removing one piece does not abort the rest; failures are collected in Failed.
func (h *Handler) FlattenPieces(ctx context.Context, repoRoot string, opts FlattenOptions) (FlattenResult, error) {
	result := FlattenResult{DryRun: opts.DryRun, Removed: []FlattenItem{}}

	// Detect the main repo root, handling the case of running from within a
	// piece worktree.
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return result, fmt.Errorf("failed to get working directory: %w", err)
		}
		detected, err := h.git.GetMainRepoRoot(ctx, wd)
		if err != nil {
			detected, err = h.git.RepoRoot(ctx, wd)
			if err != nil {
				return result, fmt.Errorf("not in a git repository")
			}
		}
		repoRoot = detected
	}
	result.MainPath = repoRoot

	pieces, err := h.ListPieces(ctx, repoRoot)
	if err != nil {
		return result, fmt.Errorf("failed to list pieces: %w", err)
	}

	for i := range pieces {
		p := pieces[i]
		item := FlattenItem{PieceName: p.Name, WorktreePath: p.WorktreePath}

		// Capture the branch name before the worktree goes away.
		if branch, err := h.git.CurrentBranch(ctx, p.WorktreePath); err == nil {
			item.BranchName = branch
		}

		if opts.DryRun {
			h.deps.Output.Write(core.Message{
				Type:    core.MsgInfo,
				Content: fmt.Sprintf("[dry-run] Would remove: %s", p.Name),
			})
			result.Removed = append(result.Removed, item)
			continue
		}

		// If we're inside the worktree about to be removed, move the active
		// client to the main repo session first.
		h.switchClientToMainIfInside(ctx, repoRoot, p.WorktreePath)

		// Kill the session if it exists (ignore errors — it may be gone).
		if p.HasSession {
			_ = h.mux.Kill(ctx, p.SessionName)
		}

		var removeErr error
		if opts.Force {
			removeErr = h.git.WorktreeRemoveForce(ctx, repoRoot, p.WorktreePath)
		} else {
			removeErr = h.git.WorktreeRemove(ctx, repoRoot, p.WorktreePath)
		}
		if removeErr != nil {
			item.Error = removeErr.Error()
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: fmt.Sprintf("Failed to remove %s (use --force to discard changes): %v", p.Name, removeErr),
			})
			result.Failed = append(result.Failed, item)
			continue
		}

		if opts.DeleteBranches && item.BranchName != "" {
			if err := h.git.BranchDelete(ctx, repoRoot, item.BranchName, true); err != nil {
				h.deps.Output.Write(core.Message{
					Type:    core.MsgWarning,
					Content: fmt.Sprintf("Removed %s but failed to delete branch %s: %v", p.Name, item.BranchName, err),
				})
			} else {
				item.BranchDeleted = true
			}
		}

		result.Removed = append(result.Removed, item)
		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: fmt.Sprintf("Removed piece: %s", p.Name),
		})
	}

	result.Count = len(result.Removed)
	return result, nil
}

// DonePiece cleans up the current piece after it has been merged.
// Must be run from within a piece worktree. Verifies the piece is merged before cleanup.
func (h *Handler) DonePiece(ctx context.Context, workDir string, input DoneInput) (DoneResult, error) {
	// Check if we're in a piece worktree
	status, err := h.Status(ctx, workDir)
	if err != nil {
		return DoneResult{}, fmt.Errorf("failed to get piece status: %w", err)
	}

	if !status.InPiece {
		return DoneResult{}, fmt.Errorf("not in a piece worktree; run this from inside a piece")
	}

	// Get main repo root
	mainRepoRoot, err := h.git.GetMainRepoRoot(ctx, workDir)
	if err != nil {
		return DoneResult{}, fmt.Errorf("failed to get main repo root: %w", err)
	}

	// Get current branch
	branchName, err := h.git.CurrentBranch(ctx, workDir)
	if err != nil {
		return DoneResult{}, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Verify piece is merged
	mergeStatus, err := h.IsBranchMerged(ctx, status.WorktreePath, branchName, input.MainBranch)
	if err != nil {
		return DoneResult{}, fmt.Errorf("failed to check merge status: %w", err)
	}

	if !mergeStatus.IsMerged {
		return DoneResult{}, fmt.Errorf("piece is not merged; use 'mp abandon' to remove unmerged pieces")
	}

	result := DoneResult{
		PieceName:    status.PieceName,
		WorktreePath: status.WorktreePath,
		MainPath:     mainRepoRoot,
	}

	// Read issue marker if exists and sync "done" status
	marker, _ := h.ReadCurrentIssueMarker(status.WorktreePath)
	if marker != nil && !marker.Issue.IsEmpty() {
		// Publish sync event to mark issue as done
		h.pubIssueSync(core.IssueSyncEvent{
			Provider:     marker.Issue.Provider,
			IssueID:      marker.Issue.ID,
			NewStatus:    StatusDone,
			PieceName:    status.PieceName,
			WorktreePath: status.WorktreePath,
		})
	}

	// If we're running inside the worktree being removed, switch the active
	// client to the main repo session before removePieceForce kills it.
	h.switchClientToMainIfInside(ctx, mainRepoRoot, status.WorktreePath)

	// Remove the piece (force removal since we're likely inside the worktree)
	if err := h.removePieceForce(ctx, mainRepoRoot, status.PieceName, status.WorktreePath); err != nil {
		return result, fmt.Errorf("failed to cleanup piece: %w", err)
	}

	result.Cleaned = true

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Done with piece: %s", status.PieceName),
		Data:    result,
	})

	return result, nil
}

// ensureSubtreeClean errors if any worktree in the subtree rooted at the given
// piece names has uncommitted changes (a rebase would refuse).
func (h *Handler) ensureSubtreeClean(ctx context.Context, piecesDir string, names []string) error {
	for _, name := range names {
		wt := filepath.Join(piecesDir, name)
		clean, err := h.git.IsClean(ctx, wt)
		if err != nil {
			return fmt.Errorf("failed to check worktree status for piece %q: %w", name, err)
		}
		if !clean {
			return fmt.Errorf("piece %q has uncommitted changes; commit or stash before merging the stack base", name)
		}
		grandchildren, err := GetPieceChildren(name, piecesDir, h.deps.FS)
		if err != nil {
			return err
		}
		if err := h.ensureSubtreeClean(ctx, piecesDir, grandchildren); err != nil {
			return err
		}
	}
	return nil
}

// mergeIntoChild merges targetBranch into the given child piece's branch
// (in its worktree), without rewriting history. Aborts the merge on conflict.
func (h *Handler) mergeIntoChild(ctx context.Context, piecesDir, childName, targetBranch string) error {
	childWorktree := filepath.Join(piecesDir, childName)
	if err := h.git.Merge(ctx, childWorktree, targetBranch); err != nil {
		_ = h.git.MergeAbort(ctx, childWorktree)
		return fmt.Errorf("failed to merge %s into piece %q (resolve manually with `mp update` from that worktree): %w", targetBranch, childName, err)
	}
	return nil
}

// rebaseSubtree rebases childName's branch onto ontoRef (cutting at fromRef),
// then recursively rebases its descendants so the whole stack stays connected.
// A piece's branch name is the same as the piece name. Returns the names of all
// pieces whose branches were rewritten.
func (h *Handler) rebaseSubtree(ctx context.Context, repoRoot, piecesDir, childName, ontoRef, fromRef string) ([]string, error) {
	childWorktree := filepath.Join(piecesDir, childName)

	grandchildren, err := GetPieceChildren(childName, piecesDir, h.deps.FS)
	if err != nil {
		return nil, err
	}
	oldChildTip, err := h.git.GetBranchCommit(ctx, repoRoot, childName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve piece branch %q: %w", childName, err)
	}

	if err := h.git.RebaseOnto(ctx, childWorktree, ontoRef, fromRef, childName); err != nil {
		_ = h.git.RebaseAbort(ctx, childWorktree)
		return nil, fmt.Errorf("failed to rebase piece %q onto %s: %w", childName, ontoRef, err)
	}

	newChildTip, err := h.git.GetBranchCommit(ctx, repoRoot, childName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve rebased branch %q: %w", childName, err)
	}

	migrated := []string{childName}
	for _, gc := range grandchildren {
		sub, err := h.rebaseSubtree(ctx, repoRoot, piecesDir, gc, newChildTip, oldChildTip)
		if err != nil {
			return nil, err
		}
		migrated = append(migrated, sub...)
	}
	return migrated, nil
}

// EnsureSubtreeClean verifies every worktree in the subtree rooted at each of
// names has no uncommitted changes. Exported wrapper over ensureSubtreeClean for
// reuse by the stack package.
func (h *Handler) EnsureSubtreeClean(ctx context.Context, piecesDir string, names []string) error {
	return h.ensureSubtreeClean(ctx, piecesDir, names)
}

// MergeIntoChild merges targetBranch into the given child piece's branch (in its
// worktree), aborting the merge on conflict. Exported wrapper for the stack package.
func (h *Handler) MergeIntoChild(ctx context.Context, piecesDir, childName, targetBranch string) error {
	return h.mergeIntoChild(ctx, piecesDir, childName, targetBranch)
}

// RebaseSubtree rebases childName onto ontoRef (cutting at fromRef) and recursively
// re-points its descendants, aborting on conflict. Returns the names of all pieces
// whose branches were rewritten. Exported wrapper for the stack package.
func (h *Handler) RebaseSubtree(ctx context.Context, repoRoot, piecesDir, childName, ontoRef, fromRef string) ([]string, error) {
	return h.rebaseSubtree(ctx, repoRoot, piecesDir, childName, ontoRef, fromRef)
}

// projectName returns the project name for repoRoot from its monkeypuzzle
// config, falling back to the directory base name.
func (h *Handler) projectName(repoRoot string) string {
	if cfg, err := ReadConfig(repoRoot, h.deps.FS); err == nil && strings.TrimSpace(cfg.Project.Name) != "" {
		return cfg.Project.Name
	}
	return filepath.Base(repoRoot)
}

// pieceSessionName returns the tmux session name for a piece in a repo.
func (h *Handler) pieceSessionName(repoRoot, pieceName string) string {
	return session.Name(h.projectName(repoRoot), pieceName)
}

// mainSessionName returns the tmux session name for a repo's main worktree.
func (h *Handler) mainSessionName(repoRoot string) string {
	return session.MainName(h.projectName(repoRoot))
}

// ListPieces returns all available pieces in the pieces directory.
// Results are sorted by modification time (newest first).
// If repoRoot is empty, it will be detected from the current working directory.
func (h *Handler) ListPieces(ctx context.Context, repoRoot string) ([]PieceListItem, error) {
	// If repoRoot not provided, detect from current working directory
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		// Use GetMainRepoRoot to handle worktrees correctly
		detectedRoot, err := h.git.GetMainRepoRoot(ctx, wd)
		if err != nil {
			// Fallback to RepoRoot if not in a worktree
			detectedRoot, err = h.git.RepoRoot(ctx, wd)
			if err != nil {
				// If not in a git repo, use empty repoRoot (global directory)
				repoRoot = ""
			} else {
				repoRoot = detectedRoot
			}
		} else {
			repoRoot = detectedRoot
		}
	}

	piecesDir, err := getPiecesDir(repoRoot)
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
		sessionName := h.pieceSessionName(repoRoot, name)

		// Get modification time
		info, err := entry.Info()
		var modTime time.Time
		if err == nil {
			modTime = info.ModTime()
		}

		// Check if session exists
		hasSession := h.mux.Exists(ctx, sessionName)

		// Read piece metadata for parent info
		parent := "main"
		if metadata, err := ReadPieceMetadata(worktreePath, h.deps.FS); err == nil {
			parent = metadata.Parent
		}

		pieces = append(pieces, PieceListItem{
			Name:         name,
			WorktreePath: worktreePath,
			SessionName:  sessionName,
			HasSession:   hasSession,
			ModTime:      modTime,
			Parent:       parent,
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(pieces, func(i, j int) bool {
		return pieces[i].ModTime.After(pieces[j].ModTime)
	})

	return pieces, nil
}

// TreeNode represents a node in the piece tree
type TreeNode struct {
	Piece    *PieceListItem // nil for root/main node
	Children []*TreeNode
	IsOrphan bool // true if parent piece doesn't exist
}

// BuildPieceTree builds a tree structure from a list of pieces.
// Returns the root node (representing "main") with children attached.
// Orphaned pieces (parent doesn't exist) are grouped under a special orphan node.
func BuildPieceTree(pieces []PieceListItem) *TreeNode {
	// Build a map of piece names to pieces for quick lookup
	pieceMap := make(map[string]*PieceListItem)
	for i := range pieces {
		pieceMap[pieces[i].Name] = &pieces[i]
	}

	// Create root node
	root := &TreeNode{Piece: nil, Children: []*TreeNode{}}

	// Track which pieces have been added as children
	added := make(map[string]bool)

	// First pass: add all pieces with existing parents
	for i := range pieces {
		piece := &pieces[i]
		if piece.Parent == "main" {
			node := &TreeNode{Piece: piece, Children: []*TreeNode{}}
			root.Children = append(root.Children, node)
			added[piece.Name] = true
		} else if parent, exists := pieceMap[piece.Parent]; exists {
			// Parent exists, find or create parent node and add as child
			parentNode := findOrCreateNode(root, parent.Name, pieceMap)
			node := &TreeNode{Piece: piece, Children: []*TreeNode{}}
			parentNode.Children = append(parentNode.Children, node)
			added[piece.Name] = true
		}
	}

	// Second pass: handle orphaned pieces (parent doesn't exist)
	var orphanNode *TreeNode
	for i := range pieces {
		piece := &pieces[i]
		if !added[piece.Name] && piece.Parent != "main" {
			// This piece's parent doesn't exist - it's orphaned
			if orphanNode == nil {
				orphanNode = &TreeNode{Piece: nil, Children: []*TreeNode{}, IsOrphan: true}
				root.Children = append(root.Children, orphanNode)
			}
			node := &TreeNode{Piece: piece, Children: []*TreeNode{}, IsOrphan: false}
			orphanNode.Children = append(orphanNode.Children, node)
		}
	}

	return root
}

// findOrCreateNode finds a node by piece name in the tree, or creates it if not found.
// This is a helper for BuildPieceTree to handle pieces that need to be parents.
func findOrCreateNode(root *TreeNode, pieceName string, pieceMap map[string]*PieceListItem) *TreeNode {
	// Search recursively for the node
	var findNode func(*TreeNode) *TreeNode
	findNode = func(node *TreeNode) *TreeNode {
		if node.Piece != nil && node.Piece.Name == pieceName {
			return node
		}
		for _, child := range node.Children {
			if result := findNode(child); result != nil {
				return result
			}
		}
		return nil
	}

	if node := findNode(root); node != nil {
		return node
	}

	// Node doesn't exist, need to create it
	piece, exists := pieceMap[pieceName]
	if !exists {
		// This shouldn't happen if BuildPieceTree logic is correct
		return root
	}

	// Create the node and add it to root if parent is main, otherwise recursively add
	node := &TreeNode{Piece: piece, Children: []*TreeNode{}}
	if piece.Parent == "main" {
		root.Children = append(root.Children, node)
		return node
	}

	// Parent is not main, need to add to parent node
	parentNode := findOrCreateNode(root, piece.Parent, pieceMap)
	parentNode.Children = append(parentNode.Children, node)
	return node
}

// SwitchPiece switches to a piece by name.
// Uses multiplexer to switch/attach, falls back to printing path.
// Special case: "main" or "master" switches to the main repo session.
func (h *Handler) SwitchPiece(ctx context.Context, name string) (SwitchResult, error) {
	// Detect repo root from current working directory or piece worktree
	wd, err := os.Getwd()
	if err != nil {
		return SwitchResult{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Get main repo root (handles being inside a worktree)
	mainRepoRoot, err := h.git.GetMainRepoRoot(ctx, wd)
	if err != nil {
		// Fallback to regular repo root
		mainRepoRoot, err = h.git.RepoRoot(ctx, wd)
		if err != nil {
			return SwitchResult{}, fmt.Errorf("not in a git repository: %w", err)
		}
	}

	// Special handling for "main" or "master" - switch to main repo
	if name == "main" || name == "master" {
		return h.switchToMain(ctx, mainRepoRoot, name)
	}

	pieces, err := h.ListPieces(ctx, mainRepoRoot)
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

	// Check if multiplexer is noop (no session management)
	if adapters.IsNoopMultiplexer(h.mux) {
		result.Method = "path"
		fmt.Println(target.WorktreePath)
		return result, nil
	}

	// Try to switch using multiplexer (creates session if needed)
	if err := h.mux.SwitchTo(ctx, target.SessionName, target.WorktreePath); err == nil {
		result.Method = "multiplexer"
		result.Piece.HasSession = true
		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: fmt.Sprintf("Switched to piece: %s", name),
			Data:    result,
		})
		return result, nil
	}

	// Fallback: print path for cd $(mp switch ...)
	result.Method = "path"
	fmt.Println(target.WorktreePath)

	return result, nil
}

// switchToMain switches to the main repo session.
func (h *Handler) switchToMain(ctx context.Context, mainRepoRoot, name string) (SwitchResult, error) {
	sessionName := h.mainSessionName(mainRepoRoot)

	// Build result with main repo info
	result := SwitchResult{
		Piece: PieceListItem{
			Name:         name,
			WorktreePath: mainRepoRoot,
			SessionName:  sessionName,
			HasSession:   h.mux.Exists(ctx, sessionName),
		},
	}

	// Check if multiplexer is noop (no session management)
	if adapters.IsNoopMultiplexer(h.mux) {
		result.Method = "path"
		fmt.Println(mainRepoRoot)
		return result, nil
	}

	// Try to switch using multiplexer (creates session if needed)
	if err := h.mux.SwitchTo(ctx, sessionName, mainRepoRoot); err == nil {
		result.Method = "multiplexer"
		result.Piece.HasSession = true
		h.deps.Output.Write(core.Message{
			Type:    core.MsgSuccess,
			Content: fmt.Sprintf("Switched to %s", name),
			Data:    result,
		})
		return result, nil
	}

	// Fallback: print path
	result.Method = "path"
	fmt.Println(mainRepoRoot)

	return result, nil
}
