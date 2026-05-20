package pr

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// ProviderDeps holds dependencies available to PR providers.
type ProviderDeps struct {
	Exec core.Exec
}

// ProviderConfig holds configuration for creating a provider.
type ProviderConfig struct {
	ProviderType string
	Config       map[string]string
	Deps         ProviderDeps
}

// ProviderFactory creates a provider from configuration.
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

var registry = map[string]ProviderFactory{}

// RegisterProvider registers a provider factory under a name.
func RegisterProvider(name string, factory ProviderFactory) {
	registry[name] = factory
}

// NewProvider builds a provider from configuration.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	factory, ok := registry[cfg.ProviderType]
	if !ok {
		return nil, fmt.Errorf("unsupported pr provider: %s", cfg.ProviderType)
	}
	return factory(cfg)
}

// RegisteredProviders lists registered provider names.
func RegisteredProviders() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

func init() {
	RegisterProvider("github", func(cfg ProviderConfig) (Provider, error) {
		if cfg.Deps.Exec == nil {
			return nil, fmt.Errorf("github provider requires Exec dependency")
		}
		return NewGitHubProvider(adapters.NewGitHub(cfg.Deps.Exec)), nil
	})

	RegisterProvider("gitlab", func(cfg ProviderConfig) (Provider, error) {
		if cfg.Deps.Exec == nil {
			return nil, fmt.Errorf("gitlab provider requires Exec dependency")
		}
		return NewGitLabProvider(adapters.NewGitLab(cfg.Deps.Exec)), nil
	})

	// Wire piece.MergeChecker so piece.Handler can detect PR-based merges
	// without importing the pr package (which would be circular).
	piece.SetMergeCheckerFactory(func(repoRoot string, deps core.Deps) piece.MergeChecker {
		cfg, err := piece.ReadConfig(repoRoot, deps.FS)
		if err != nil {
			return nil
		}
		providerType := cfg.PR.Provider
		if providerType == "" {
			providerType = "github"
		}
		p, err := NewProvider(ProviderConfig{
			ProviderType: providerType,
			Config:       cfg.PR.Config,
			Deps:         ProviderDeps{Exec: deps.Exec},
		})
		if err != nil {
			return nil
		}
		return p
	})
}
