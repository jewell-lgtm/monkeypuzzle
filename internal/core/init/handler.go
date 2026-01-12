package init

import (
	"encoding/json"
	"path/filepath"

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
	Issues  IssueConfig   `json:"issues"`
	PR      PRConfig      `json:"pr"`
}

type ProjectConfig struct {
	Name string `json:"name"`
}

type IssueConfig struct {
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config"`
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

// ConfigExists checks if a config already exists
func (h *Handler) ConfigExists() bool {
	_, err := h.deps.FS.Stat(filepath.Join(DirName, ConfigFile))
	return err == nil
}

// Run executes the init command with validated input.
// Expects input to be pre-validated via WithDefaults() and Validate().
// Returns the created Config for JSON output.
func (h *Handler) Run(input Input, workDir string) (Config, error) {
	// Create directories
	if err := h.deps.FS.MkdirAll(DirName, DefaultDirPerm); err != nil {
		return Config{}, err
	}

	issuesDir := "issues"
	if input.IssueProvider == "markdown" {
		if err := h.deps.FS.MkdirAll(issuesDir, DefaultDirPerm); err != nil {
			return Config{}, err
		}
	}

	// Build config
	cfg := Config{
		Version: "1",
		Project: ProjectConfig{Name: input.Name},
		Issues: IssueConfig{
			Provider: input.IssueProvider,
			Config:   make(map[string]string),
		},
		PR: PRConfig{
			Provider: input.PRProvider,
			Config:   make(map[string]string),
		},
	}

	// Copy issue config from input
	if input.IssueConfig != nil {
		for k, v := range input.IssueConfig {
			cfg.Issues.Config[k] = v
		}
	}

	if input.IssueProvider == "markdown" {
		cfg.Issues.Config["directory"] = issuesDir
	}

	// Write config
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}

	configPath := filepath.Join(DirName, ConfigFile)
	if err := h.deps.FS.WriteFile(configPath, data, DefaultFilePerm); err != nil {
		return Config{}, err
	}

	// Ensure .gitignore has correct entries
	if err := h.EnsureGitignore(); err != nil {
		return Config{}, err
	}

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

// EnsureGitignore creates .monkeypuzzle/.gitignore for piece-specific state.
func (h *Handler) EnsureGitignore() error {
	gitignorePath := filepath.Join(DirName, ".gitignore")
	content := `# Piece worktree state (not tracked)
current-issue.json
piece-metadata.json
pr-metadata.json
`
	return h.deps.FS.WriteFile(gitignorePath, []byte(content), DefaultFilePerm)
}
