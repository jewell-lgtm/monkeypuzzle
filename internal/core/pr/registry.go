package pr

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
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
	piece.SetMergeCheckerFactory(NewMergeChecker)
}

// NewMergeChecker resolves the PR provider used to detect whether a piece branch
// was merged via a PR/MR. It satisfies piece.MergeCheckerFactory and is the seam
// that wires forge-based (squash-aware) merge detection into cleanup/done.
//
// repoRoot is often a piece worktree path — cleanup/done resolve merge status
// before removing the worktree and pass it here. monkeypuzzle.json lives in the
// MAIN working tree's state dir (.monkeypuzzle/), which a linked worktree's
// checkout does not contain (it's gitignored at the main root). Reading config
// straight from repoRoot therefore fails for every piece, which used to make the
// factory return nil and silently skip PR-based detection — the only method that
// catches multi-commit squash-merges — leaving cleanup with git-ancestry only.
// So resolve the main repo root before reading config.
func NewMergeChecker(repoRoot string, deps core.Deps) piece.MergeChecker {
	configRoot := repoRoot
	if mainRoot, err := projectdir.MainRepoRoot(repoRoot); err == nil {
		configRoot = mainRoot
	}

	// Default to the github provider when config is absent/unreadable rather than
	// disabling detection: a missing forge declaration should degrade to the
	// default, not turn cleanup into ancestry-only. A wrong guess is harmless —
	// the forge CLI errors out and detection falls through to git ancestry.
	providerType := "github"
	var providerConfig map[string]string
	if cfg, err := piece.ReadConfig(configRoot, deps.FS); err == nil {
		if cfg.PR.Provider != "" {
			providerType = cfg.PR.Provider
		}
		providerConfig = cfg.PR.Config
	}

	p, err := NewProvider(ProviderConfig{
		ProviderType: providerType,
		Config:       providerConfig,
		Deps:         ProviderDeps{Exec: deps.Exec},
	})
	if err != nil {
		return nil
	}
	return p
}
