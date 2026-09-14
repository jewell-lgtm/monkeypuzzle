package skill_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/skill"
)

func newTestHandler() (*skill.Handler, *adapters.MemoryFS) {
	fs := adapters.NewMemoryFS()
	h := skill.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()})
	return h, fs
}

func TestCatalogShipsManagingSkill(t *testing.T) {
	var found bool
	for _, s := range skill.Catalog() {
		if s.Name == skill.DefaultSkill {
			found = true
			if s.Description == "" {
				t.Error("description not parsed from frontmatter")
			}
			if !strings.HasPrefix(string(s.Body), "---\n") {
				t.Error("body should start with YAML frontmatter")
			}
		}
	}
	if !found {
		t.Fatalf("catalog is missing %q", skill.DefaultSkill)
	}
}

func TestGetUnknownSkill(t *testing.T) {
	if _, err := skill.Get("no-such-skill"); !errors.Is(err, skill.ErrUnknownSkill) {
		t.Fatalf("want ErrUnknownSkill, got %v", err)
	}
}

func TestInstallWritesCanonicalPathAndLink(t *testing.T) {
	h, fs := newTestHandler()

	result, err := h.Install("/repo", skill.Input{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result.Status != skill.StatusCreated {
		t.Errorf("status = %q, want %q", result.Status, skill.StatusCreated)
	}
	if result.Scope != skill.ScopeProject {
		t.Errorf("scope = %q, want %q", result.Scope, skill.ScopeProject)
	}

	canonical := filepath.Join("/repo", skill.AgentsDir, skill.DefaultSkill, skill.SkillFile)
	body, err := fs.ReadFile(canonical)
	if err != nil {
		t.Fatalf("canonical skill not written: %v", err)
	}
	if !strings.Contains(string(body), "name: "+skill.DefaultSkill) {
		t.Error("written body is not the skill document")
	}

	// The link must be relative, or a checked-in repo breaks when cloned
	// somewhere else.
	link := filepath.Join("/repo", skill.ClaudeDir, skill.DefaultSkill)
	target, err := fs.Readlink(link)
	if err != nil {
		t.Fatalf("claude link not created: %v", err)
	}
	if want := filepath.Join("..", "..", skill.AgentsDir, skill.DefaultSkill); target != want {
		t.Errorf("link target = %q, want %q", target, want)
	}
}

func TestInstallReportsUnchangedAndUpdated(t *testing.T) {
	h, fs := newTestHandler()

	if _, err := h.Install("/repo", skill.Input{}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	again, err := h.Install("/repo", skill.Input{})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if again.Status != skill.StatusUnchanged {
		t.Errorf("status = %q, want %q", again.Status, skill.StatusUnchanged)
	}

	canonical := filepath.Join("/repo", skill.AgentsDir, skill.DefaultSkill, skill.SkillFile)
	if err := fs.WriteFile(canonical, []byte("drifted"), skill.DefaultFilePerm); err != nil {
		t.Fatalf("seed drift: %v", err)
	}
	repaired, err := h.Install("/repo", skill.Input{})
	if err != nil {
		t.Fatalf("third install: %v", err)
	}
	if repaired.Status != skill.StatusUpdated {
		t.Errorf("status = %q, want %q", repaired.Status, skill.StatusUpdated)
	}
	body, _ := fs.ReadFile(canonical)
	if string(body) == "drifted" {
		t.Error("drifted body was not rewritten")
	}
}

func TestInstallUserScope(t *testing.T) {
	h, _ := newTestHandler()

	result, err := h.Install("/home/someone", skill.Input{User: true})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result.Scope != skill.ScopeUser {
		t.Errorf("scope = %q, want %q", result.Scope, skill.ScopeUser)
	}
}

func TestInstallUnknownSkillFails(t *testing.T) {
	h, _ := newTestHandler()

	if _, err := h.Install("/repo", skill.Input{Name: "nope"}); !errors.Is(err, skill.ErrUnknownSkill) {
		t.Fatalf("want ErrUnknownSkill, got %v", err)
	}
}

func TestSchemaKeepsBooleanFields(t *testing.T) {
	schema, err := skill.Schema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	// omitempty on Input.User would erase this from a struct-built example.
	if !strings.Contains(string(schema), `"user"`) {
		t.Errorf("schema is missing the user field: %s", schema)
	}
}

// The upgrade path: mp wrote a real directory at .claude/skills/<name> before
// skills moved to .agents. It must not fail, and must not delete anything.
func TestInstallLeavesRealDirectoryAlone(t *testing.T) {
	h, fs := newTestHandler()

	occupied := filepath.Join("/repo", skill.ClaudeDir, skill.DefaultSkill)
	if err := fs.MkdirAll(occupied, skill.DefaultDirPerm); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := h.Install("/repo", skill.Input{})
	if err != nil {
		t.Fatalf("install must not fail when the link path is occupied: %v", err)
	}
	if result.Status != skill.StatusCreated {
		t.Errorf("status = %q, want %q", result.Status, skill.StatusCreated)
	}
	if result.Link != "" {
		t.Errorf("link = %q, want empty when mp declined to touch the path", result.Link)
	}
	// The canonical document is still the deliverable.
	canonical := filepath.Join("/repo", skill.AgentsDir, skill.DefaultSkill, skill.SkillFile)
	if _, err := fs.ReadFile(canonical); err != nil {
		t.Errorf("canonical skill not written: %v", err)
	}
}

// A link someone else made is their choice, not stale state to repoint.
func TestInstallLeavesForeignSymlinkAlone(t *testing.T) {
	h, fs := newTestHandler()

	linkPath := filepath.Join("/repo", skill.ClaudeDir, skill.DefaultSkill)
	if err := fs.MkdirAll(filepath.Join("/repo", skill.ClaudeDir), skill.DefaultDirPerm); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := fs.Symlink("../../mine", linkPath); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	result, err := h.Install("/repo", skill.Input{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result.Link != "" {
		t.Errorf("link = %q, want empty", result.Link)
	}
	target, err := fs.Readlink(linkPath)
	if err != nil {
		t.Fatalf("the user's link was removed: %v", err)
	}
	if target != "../../mine" {
		t.Errorf("link was repointed to %q", target)
	}
}
