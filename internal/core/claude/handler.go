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

All commands: ` + "`echo '{...}' | mp <cmd>`" + ` with JSON output to stdout. Use ` + "`mp <cmd> --schema`" + ` for input schema.

## Commands

` + "```" + `
echo '{}' | mp issue list
echo '{"status":["todo"]}' | mp issue list
echo '{"title":"Feature","description":"..."}' | mp issue create
echo '{"issue_path":"issues/feat.md","skip_switch":true}' | mp piece new
echo '{"name":"my-feature","skip_switch":true}' | mp piece new
echo '{"flat":true}' | mp piece list
echo '{}' | mp piece update
echo '{}' | mp piece merge
echo '{"title":"Add feature","body":"..."}' | mp piece pr create
echo '{}' | mp piece done
echo '{"force":true}' | mp piece cleanup
echo '{"force":true}' | mp piece abandon
echo '{"name":"project","issue_provider":"markdown","pr_provider":"github"}' | mp init
` + "```" + `
`
