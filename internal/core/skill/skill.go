// Package skill materialises the agent skill documents mp ships. Skills are a
// portable format, so the canonical location is .agents/skills/<name>/SKILL.md
// with a .claude/skills/<name> symlink for agents that only read their own
// directory — the same split the repo already uses for AGENTS.md and CLAUDE.md.
package skill

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

//go:embed assets/*.md
var assets embed.FS

const (
	// AgentsDir is the canonical, vendor-neutral skill location.
	AgentsDir = ".agents/skills"
	// ClaudeDir holds a symlink per skill; Claude Code does not read AgentsDir.
	ClaudeDir = ".claude/skills"
	// SkillFile is the document every skill directory contains.
	SkillFile = "SKILL.md"

	DefaultDirPerm  = 0755
	DefaultFilePerm = 0644
)

// Status describes what an install did to the target.
const (
	StatusCreated   = "created"
	StatusUpdated   = "updated"
	StatusUnchanged = "unchanged"
)

// Skill is one document mp can write out.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        []byte `json:"-"`
}

// ErrUnknownSkill is returned for a name mp does not ship.
var ErrUnknownSkill = errors.New("unknown skill")

// Catalog lists every skill mp ships, by name.
func Catalog() []Skill {
	entries, err := assets.ReadDir("assets")
	if err != nil {
		return nil
	}
	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		body, err := assets.ReadFile(filepath.Join("assets", entry.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		skills = append(skills, Skill{Name: name, Description: frontmatterDescription(body), Body: body})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills
}

// Get returns one shipped skill by name.
func Get(name string) (Skill, error) {
	for _, s := range Catalog() {
		if s.Name == name {
			return s, nil
		}
	}
	return Skill{}, fmt.Errorf("%w %q; run 'mp skill list' to see what mp ships", ErrUnknownSkill, name)
}

// frontmatterDescription pulls the description field out of the YAML header so
// 'mp skill list' can show it without a YAML dependency.
func frontmatterDescription(body []byte) string {
	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if rest, ok := strings.CutPrefix(line, "description:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// Handler writes skills to disk.
type Handler struct {
	deps core.Deps
}

func NewHandler(deps core.Deps) *Handler {
	return &Handler{deps: deps}
}

// Install writes one skill under root, which is a repo root for project scope
// or the user's home directory for user scope.
func (h *Handler) Install(root string, in Input) (Result, error) {
	in = WithDefaults(in)
	target, err := Get(in.Name)
	if err != nil {
		return Result{}, err
	}

	skillDir := filepath.Join(root, AgentsDir, target.Name)
	skillPath := filepath.Join(skillDir, SkillFile)

	status := StatusCreated
	switch existing, readErr := h.deps.FS.ReadFile(skillPath); {
	case readErr == nil && bytes.Equal(existing, target.Body):
		status = StatusUnchanged
	case readErr == nil:
		status = StatusUpdated
	case !errors.Is(readErr, os.ErrNotExist):
		return Result{}, readErr
	}

	if status != StatusUnchanged {
		if err := h.deps.FS.MkdirAll(skillDir, DefaultDirPerm); err != nil {
			return Result{}, err
		}
		if err := h.deps.FS.WriteFile(skillPath, target.Body, DefaultFilePerm); err != nil {
			return Result{}, err
		}
	}

	// The link is a convenience for one agent; the document is the deliverable.
	// Failing the whole call would report nothing for a file already written.
	link, err := h.linkForClaude(root, target.Name)
	if err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("wrote the skill but could not link %s: %v", filepath.Join(ClaudeDir, target.Name), err),
		})
	}

	scope := ScopeProject
	if in.User {
		scope = ScopeUser
	}
	result := Result{
		Name:   target.Name,
		Path:   filepath.Join(AgentsDir, target.Name, SkillFile),
		Link:   link,
		Scope:  scope,
		Status: status,
	}
	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("%s %s (%s scope)", status, result.Path, scope),
	})
	return result, nil
}

// warnOccupied reports a .claude/skills path mp will not touch. Leaving a
// skill someone wrote by hand alone matters more than the convenience link.
func (h *Handler) warnOccupied(name, why string) {
	h.deps.Output.Write(core.Message{
		Type:    core.MsgWarning,
		Content: fmt.Sprintf("%s %s; leaving it alone", filepath.Join(ClaudeDir, name), why),
	})
}

// linkForClaude points .claude/skills/<name> at the canonical .agents copy,
// because Claude Code does not read .agents/skills. The link is relative so a
// checked-in repo stays portable. Anything real already sitting at that path is
// left alone and reported — it may be a skill the user wrote by hand.
func (h *Handler) linkForClaude(root, name string) (string, error) {
	linkDir := filepath.Join(root, ClaudeDir)
	linkPath := filepath.Join(linkDir, name)
	rel := filepath.Join("..", "..", AgentsDir, name)

	switch target, err := h.deps.FS.Readlink(linkPath); {
	case err == nil && target == rel:
		return filepath.Join(ClaudeDir, name), nil
	case err == nil:
		// A link somewhere else is someone's deliberate choice, not stale state
		// of ours to repoint.
		h.warnOccupied(name, "is a link to "+target)
		return "", nil
	case errors.Is(err, os.ErrInvalid):
		// Upgrade path: mp wrote a real directory here before skills moved to
		// .agents. Say what to do rather than deleting someone's files.
		h.warnOccupied(name, "already exists and is not a link (remove it and re-run to link the canonical copy)")
		return "", nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}

	if err := h.deps.FS.MkdirAll(linkDir, DefaultDirPerm); err != nil {
		return "", err
	}
	if err := h.deps.FS.Symlink(rel, linkPath); err != nil {
		return "", err
	}
	return filepath.Join(ClaudeDir, name), nil
}
