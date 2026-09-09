package init

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/claude"
)

const (
	DirName    = ".monkeypuzzle"
	ConfigFile = "monkeypuzzle.json"

	// DefaultDirPerm is the default permission for directories (0755 = rwxr-xr-x)
	DefaultDirPerm = 0755
	// DefaultFilePerm is the default permission for files (0644 = rw-r--r--)
	DefaultFilePerm = 0644
)

// Config is the output config structure written to monkeypuzzle.json
type Config struct {
	Version string        `json:"version"`
	Project ProjectConfig `json:"project"`
	PR      PRConfig      `json:"pr"`
}

type ProjectConfig struct {
	Name string `json:"name"`
}

type PRConfig struct {
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config"`
}

// Handler executes the init command
type Handler struct {
	deps core.Deps
}

// NewHandler creates a new init handler with dependencies
func NewHandler(deps core.Deps) *Handler {
	return &Handler{deps: deps}
}

// ConfigExists checks if a config already exists in the given monkeypuzzle dir
// (relative to the working directory). An empty dir means the default.
func (h *Handler) ConfigExists(dir string) bool {
	if dir == "" {
		dir = DirName
	}
	_, err := h.deps.FS.Stat(filepath.Join(dir, ConfigFile))
	return err == nil
}

// Run executes the init command with validated input.
// Expects input to be pre-validated via WithDefaults() and Validate().
// Returns the created Config for JSON output.
func (h *Handler) Run(input Input, workDir string) (Config, error) {
	mpDir := input.Dir
	if mpDir == "" {
		mpDir = DirName
	}

	// Create directories
	if err := h.deps.FS.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return Config{}, err
	}

	// Build config
	cfg := Config{
		Version: "1",
		Project: ProjectConfig{Name: input.Name},
		PR: PRConfig{
			Provider: input.PRProvider,
			Config:   make(map[string]string),
		},
	}

	// Write config
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}

	configPath := filepath.Join(mpDir, ConfigFile)
	if err := h.deps.FS.WriteFile(configPath, data, DefaultFilePerm); err != nil {
		return Config{}, err
	}

	// Ensure .gitignore has correct entries
	if err := h.EnsureGitignore(mpDir); err != nil {
		return Config{}, err
	}
	h.ensureExcludeWarn(workDir, mpDir)

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Created " + configPath,
		Data:    cfg,
	})

	// Create Claude Code skill if requested
	if input.CreateSkill != nil && *input.CreateSkill {
		claudeHandler := claude.NewHandler(h.deps)
		if _, err := claudeHandler.CreateSkill(workDir); err != nil {
			// Non-fatal: log warning but don't fail init
			h.deps.Output.Write(core.Message{
				Type:    core.MsgWarning,
				Content: "Failed to create Claude skill: " + err.Error(),
			})
		}
	}

	return cfg, nil
}

// Refresh re-runs the idempotent parts of init for an already-configured repo:
// re-writes <mpDir>/.gitignore (entries do drift between mp versions) and
// regenerates the Claude skill. monkeypuzzle.json is read but not modified —
// switching providers is an explicit reconfigure via Run.
//
// Returns the existing config so callers can emit it just like Run does.
func (h *Handler) Refresh(workDir, mpDir string) (Config, error) {
	if mpDir == "" {
		mpDir = DirName
	}

	configPath := filepath.Join(mpDir, ConfigFile)
	data, err := h.deps.FS.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("no existing config at %s: %w", configPath, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse existing config: %w", err)
	}

	if err := h.EnsureGitignore(mpDir); err != nil {
		return Config{}, err
	}
	h.ensureExcludeWarn(workDir, mpDir)

	claudeHandler := claude.NewHandler(h.deps)
	if _, err := claudeHandler.CreateSkill(workDir); err != nil {
		h.deps.Output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: "Failed to refresh Claude skill: " + err.Error(),
		})
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Refreshed " + mpDir + " (gitignore + Claude skill); monkeypuzzle.json left untouched",
		Data:    cfg,
	})

	return cfg, nil
}

// EnsureGitignore creates <mpDir>/.gitignore for piece-specific state.
// An empty mpDir means the default ".monkeypuzzle".
func (h *Handler) EnsureGitignore(mpDir string) error {
	if mpDir == "" {
		mpDir = DirName
	}
	gitignorePath := filepath.Join(mpDir, ".gitignore")
	content := `# Piece worktree state (not tracked)
piece-metadata.json
pr-metadata.json

# Piece worktrees
pieces/

# Hook logs (fire-and-forget hook output)
logs/
`
	return h.deps.FS.WriteFile(gitignorePath, []byte(content), DefaultFilePerm)
}

// excludeEntries are the piece-state paths mp writes inside every worktree,
// relative to the repo root. They mirror <mpDir>/.gitignore.
func excludeEntries(mpDir string) []string {
	rel := filepath.ToSlash(filepath.Clean(mpDir))
	return []string{
		rel + "/piece-metadata.json",
		rel + "/pr-metadata.json",
		rel + "/pieces/",
		rel + "/logs/",
	}
}

// EnsureExclude adds mp's piece-state paths to <git-common-dir>/info/exclude,
// which git honours in every linked worktree regardless of what the checked-out
// commit's .gitignore says. Without it a piece branched from a commit that
// predates the committed <mpDir>/.gitignore (the freshly-initialised repo is the
// common case) carries an untracked piece-metadata.json: `git worktree remove`
// refuses it and mp's clean checks call the piece dirty. Idempotent; a no-op
// when workDir is not inside a git repository or no exec is available.
func (h *Handler) EnsureExclude(ctx context.Context, workDir, mpDir string) error {
	if h.deps.Exec == nil {
		return nil
	}
	if mpDir == "" {
		mpDir = DirName
	}
	common, err := adapters.NewGit(h.deps.Exec).CommonDir(ctx, workDir)
	if err != nil {
		return nil // not a git repo (or git missing): nothing to exclude in
	}
	path := filepath.Join(common, "info", "exclude")
	existing, _ := h.deps.FS.ReadFile(path)
	have := map[string]bool{}
	for _, line := range strings.Split(string(existing), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, e := range excludeEntries(mpDir) {
		if !have[e] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := h.deps.FS.MkdirAll(filepath.Dir(path), DefaultDirPerm); err != nil {
		return err
	}
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "# monkeypuzzle piece state (written by mp init)\n" + strings.Join(missing, "\n") + "\n"
	return h.deps.FS.WriteFile(path, []byte(content), DefaultFilePerm)
}

// ensureExcludeWarn runs EnsureExclude and downgrades a failure to a warning:
// the committed .gitignore still covers the common case.
func (h *Handler) ensureExcludeWarn(workDir, mpDir string) {
	if err := h.EnsureExclude(context.Background(), workDir, mpDir); err != nil {
		h.deps.Output.Write(core.Message{Type: core.MsgWarning, Content: "Failed to update git info/exclude: " + err.Error()})
	}
}
