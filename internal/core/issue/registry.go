package issue

import (
	"fmt"
	"os"

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

// NewProvider creates a provider from configuration
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
	// Register markdown provider
	RegisterProvider("markdown", func(cfg ProviderConfig) (Provider, error) {
		dir, ok := cfg.Config["directory"]
		if !ok || dir == "" {
			dir = "issues"
		}
		return NewMarkdownProvider(cfg.Deps.FS, dir), nil
	})

	// Register linear provider
	RegisterProvider("linear", func(cfg ProviderConfig) (Provider, error) {
		apiKey := os.Getenv("LINEAR_API_KEY")
		if apiKey == "" {
			if key, ok := cfg.Config["api_key"]; ok {
				apiKey = key
			}
		}
		if apiKey == "" {
			return nil, fmt.Errorf("LINEAR_API_KEY env var or api_key config required")
		}

		teamKey := cfg.Config["team"]
		if teamKey == "" {
			return nil, fmt.Errorf("linear provider requires 'team' config")
		}

		if cfg.Deps.HTTP == nil {
			return nil, fmt.Errorf("linear provider requires HTTP client")
		}

		return NewLinearProvider(cfg.Deps.HTTP, apiKey, teamKey), nil
	})
}
