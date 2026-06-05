package issue

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// ProviderDeps holds dependencies available to providers
type ProviderDeps struct {
	FS   core.FS
	HTTP core.HTTPClient
}

// ProviderConfig holds configuration for creating a provider
type ProviderConfig struct {
	ProviderType string
	Config       map[string]string
	Deps         ProviderDeps
}

// ProviderFactory creates a provider from configuration
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

// registry holds registered provider factories
var registry = map[string]ProviderFactory{}

// RegisterProvider registers a provider factory
func RegisterProvider(name string, factory ProviderFactory) {
	registry[name] = factory
}

// NewProvider creates a provider from configuration.
//
// The local issue store is unconditionally markdown: trackers (linear, plane)
// are no longer two-way Providers, only one-shot import sources (see Importer).
func NewProvider(cfg ProviderConfig) (Provider, error) {
	factory, ok := registry[cfg.ProviderType]
	if !ok {
		return nil, fmt.Errorf("unsupported issue provider: %s", cfg.ProviderType)
	}
	return factory(cfg)
}

// RegisteredProviders returns list of registered provider names
func RegisteredProviders() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

func init() {
	// The markdown provider is the only local store.
	RegisterProvider("markdown", func(cfg ProviderConfig) (Provider, error) {
		dir, ok := cfg.Config["directory"]
		if !ok || dir == "" {
			dir = "issues"
		}
		return NewMarkdownProvider(cfg.Deps.FS, dir), nil
	})
}
