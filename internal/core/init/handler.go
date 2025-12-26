package init

import (
	"encoding/json"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
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

// Run executes the init command with validated input
func (h *Handler) Run(input Input) error {
	// Sanitize project name (remove invalid filesystem characters)
	input.Name = SanitizeProjectName(input.Name)
	
	// Validate input
	if err := Validate(input); err != nil {
		return err
	}

	// Create directories
	if err := h.deps.FS.MkdirAll(DirName, DefaultDirPerm); err != nil {
		return err
	}

	issuesDir := "issues"
	if input.IssueProvider == "markdown" {
		if err := h.deps.FS.MkdirAll(issuesDir, DefaultDirPerm); err != nil {
			return err
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

	if input.IssueProvider == "markdown" {
		cfg.Issues.Config["directory"] = issuesDir
	}

	// Write config
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(DirName, ConfigFile)
	if err := h.deps.FS.WriteFile(configPath, data, DefaultFilePerm); err != nil {
		return err
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Created " + configPath,
		Data:    cfg,
	})

	return nil
}
