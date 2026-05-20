package issue

import (
	"fmt"

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
	provider, err := h.getProvider()
	if err != nil {
		return Issue{}, err
	}
	return provider.Get(id)
}

// getProvider builds the configured provider from .monkeypuzzle/monkeypuzzle.json.
func (h *Handler) getProvider() (Provider, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return nil, fmt.Errorf("failed to read config (run mp init first): %w", err)
	}
	return NewProvider(ProviderConfig{
		ProviderType: cfg.Issues.Provider,
		Config:       cfg.Issues.Config,
		Deps: ProviderDeps{
			FS:   h.deps.FS,
			HTTP: h.deps.HTTP,
			Exec: h.deps.Exec,
		},
	})
}
