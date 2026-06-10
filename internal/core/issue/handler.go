package issue

import (
	"fmt"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// Handler resolves issue identifiers via the configured provider.
type Handler struct {
	deps    core.Deps
	workDir string
}

// NewHandler creates a new issue handler with dependencies.
func NewHandler(deps core.Deps, workDir string) *Handler {
	return &Handler{deps: deps, workDir: workDir}
}

// Get resolves the given identifier via the configured provider.
func (h *Handler) Get(id string) (Issue, error) {
	provider, providerType, err := h.getProvider()
	if err != nil {
		return Issue{}, err
	}
	// Markdown ids are file paths; anchor relative paths at the project root so
	// resolution works regardless of the caller's cwd (e.g. `mp switch --issue`
	// run from outside the repo).
	if providerType == "markdown" && !filepath.IsAbs(id) {
		id = filepath.Join(h.workDir, id)
	}
	return provider.Get(id)
}

// getProvider builds the configured provider from .monkeypuzzle/monkeypuzzle.json.
func (h *Handler) getProvider() (Provider, string, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read config (run mp init first): %w", err)
	}
	provider, err := NewProvider(ProviderConfig{
		ProviderType: cfg.Issues.Provider,
		Config:       cfg.Issues.Config,
		Deps: ProviderDeps{
			FS:   h.deps.FS,
			HTTP: h.deps.HTTP,
			Exec: h.deps.Exec,
		},
	})
	if err != nil {
		return nil, "", err
	}
	return provider, cfg.Issues.Provider, nil
}
