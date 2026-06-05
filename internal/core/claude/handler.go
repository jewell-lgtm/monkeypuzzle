package claude

import (
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const (
	SkillDir  = ".claude/skills/managing-monkeypuzzle"
	SkillFile = "SKILL.md"

	DefaultDirPerm  = 0755
	DefaultFilePerm = 0644
)

// Result is the output of skill creation
type Result struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// Handler manages Claude Code skill creation
type Handler struct {
	deps core.Deps
}

// NewHandler creates a new claude handler
func NewHandler(deps core.Deps) *Handler {
	return &Handler{deps: deps}
}

// CreateSkill creates the Claude Code skill file
func (h *Handler) CreateSkill(workDir string) (Result, error) {
	skillDir := filepath.Join(workDir, SkillDir)
	skillPath := filepath.Join(skillDir, SkillFile)

	if err := h.deps.FS.MkdirAll(skillDir, DefaultDirPerm); err != nil {
		return Result{}, err
	}

	if err := h.deps.FS.WriteFile(skillPath, []byte(skillContent), DefaultFilePerm); err != nil {
		return Result{}, err
	}

	relPath := filepath.Join(SkillDir, SkillFile)
	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Created " + relPath,
	})

	return Result{
		Path:    relPath,
		Created: true,
	}, nil
}

// skillContent is the embedded SKILL.md content
const skillContent = `---
name: managing-monkeypuzzle
description: Manages development workflow with mp CLI. Creates pieces (git worktrees), tracks issues, creates PRs. Use when working with .monkeypuzzle projects, mp commands, or piece-based development.
---

# mp CLI

All commands support JSON stdin (` + "`echo '{...}' | mp <cmd>`" + `) with JSON output to stdout. Use ` + "`mp <cmd> --schema`" + ` for input schema.

## Issues

` + "```bash" + `
# List issues (all or filtered by status)
echo '{}' | mp issue list
echo '{"status":["todo"]}' | mp issue list
echo '{"status":["todo","in-progress"]}' | mp issue list

# Create issue
echo '{"title":"Feature X","description":"Details..."}' | mp issue create

# Search issues (fuzzy match)
echo '{"query":"auth","status":["todo"]}' | mp issue search
` + "```" + `

## Pieces (worktrees)

` + "```bash" + `
# Show current piece status
mp piece

# List all pieces (tree view or flat)
mp piece list
echo '{"flat":true}' | mp piece list

# Create new piece from issue (via --issue flag)
mp piece create --issue issues/feat.md --skip-switch

# Create new piece by name
echo '{"name":"my-feature","skip_switch":true}' | mp piece create

# Create piece from a prompt (name auto-generated from the prompt)
mp piece create --prompt "add dark mode"

# Create stacked piece (child of another piece)
echo '{"name":"child-feat","parent":"parent-piece"}' | mp piece create

# Switch to existing piece
echo '{"name":"my-feature"}' | mp piece switch

# Update piece with latest from main
echo '{}' | mp piece update
echo '{"main_branch":"develop"}' | mp piece update

# Merge piece back to main (requires no unmerged children)
echo '{}' | mp piece merge
echo '{"force":true}' | mp piece merge  # force even with children

# Create PR for current piece
echo '{}' | mp piece pr create
echo '{"title":"Add X","body":"Description"}' | mp piece pr create

# Cleanup after merge
echo '{}' | mp piece done

# Cleanup all merged pieces
echo '{"force":true}' | mp piece cleanup
echo '{"dry_run":true}' | mp piece cleanup

# Abandon unmerged piece
echo '{"force":true}' | mp piece abandon
echo '{"name":"piece-name","delete_branch":true}' | mp piece abandon

# Flatten: remove ALL piece worktrees (regardless of merge status)
echo '{"force":true}' | mp flatten
echo '{"force":true,"delete_branches":true}' | mp flatten
echo '{"dry_run":true}' | mp flatten   # preview only

# Convert existing branch to piece
echo '{}' | mp piece adopt
echo '{"name":"custom-name","parent":"main"}' | mp piece adopt

# Cross-project picker: pieces + open todo issues + unadopted branches.
# Selecting an issue creates a piece from it; selecting a branch adopts it.
mp switch --project app --piece my-feature                           # attach existing piece
mp switch --project app --issue issues/foo.md                        # create + attach
mp switch --project app --branch spike-feature                       # adopt + attach
echo '{"project":"app","issue":"issues/foo.md"}' | mp switch
echo '{"project":"app","branch":"spike-feature"}' | mp switch
` + "```" + `

## Stacks (git-town-style whole-stack ops)

Pieces stacked via ` + "`--parent`" + ` form a stack. ` + "`mp stack`" + ` operates over the whole
stack. All operations are non-interactive: anything risky aborts cleanly and
prints plain-English next steps (e.g. which PR base to change on GitHub).

` + "```bash" + `
# Show the stack tree, PR state, and drift vs the GitHub PR list
mp stack status

# Propagate main and each parent down through the stack
mp stack sync
mp stack sync --strategy rebase   # rebase instead of merge

# Resume a conflicted rebase started by 'mp stack sync --strategy rebase'
mp stack continue

# Create a new piece as a child of the current piece
mp stack append --name child-feat

# Insert a new piece between the current piece and its parent
mp stack prepend --name base-feat
` + "```" + `

## Projects & Dashboard

A "project" is any git repo initialised with ` + "`mp init`" + `. Registering projects lets
mp list pieces and issues across all of them and jump between their sessions.

` + "```bash" + `
# Register / list / unregister projects (current dir by default)
mp project add
mp project list
mp project remove <name>

# Cross-project dashboard of projects + piece worktrees (bare 'mp' is the same)
mp dash
mp dash --json
` + "```" + `

## Init & Config

` + "```bash" + `
# Init monkeypuzzle in repo
echo '{"name":"project","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Config (uses args, not JSON stdin)
mp config get multiplexer
mp config set multiplexer tmux  # tmux, zellij, or none

# Relocate the .monkeypuzzle state dir (e.g. into a gitignored path)
mp move .DONOTCOMMIT/monkeypuzzle
mp move .monkeypuzzle                # move back to the default
` + "```" + `

## Workflow

1. ` + "`mp issue create`" + ` or find existing issue
2. ` + "`mp piece create --issue issues/foo.md`" + ` creates worktree
3. Work in worktree, commit changes
4. For dependent work, stack with ` + "`mp stack append`" + ` and keep it in sync via ` + "`mp stack sync`" + `
5. ` + "`mp piece pr create`" + ` pushes and creates PR
6. After PR merged: ` + "`mp piece done`" + ` or ` + "`mp piece cleanup`" + `
`
