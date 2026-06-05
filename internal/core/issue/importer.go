package issue

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// RemoteIssue is a read-only issue fetched from an external tracker (Linear,
// Plane, ...). It is the minimal shape needed to materialise a local markdown
// issue. Trackers are no longer a two-way store; they are one-shot import
// sources.
type RemoteIssue struct {
	ID    string `json:"id"`    // Provider-specific identifier (UUID / human key)
	Title string `json:"title"` // Issue title
	Body  string `json:"body"`  // Issue description / body
	URL   string `json:"url"`   // Canonical URL of the remote issue (best-effort)
}

// Importer is a read-only one-shot import source. Implementations fetch remote
// issues so that `mp issue import` can write them as local markdown. They never
// create or mutate remote state.
type Importer interface {
	// Search returns remote issues matching query (empty = recent issues),
	// capped at limit (0 = default).
	Search(ctx context.Context, query string, limit int) ([]RemoteIssue, error)
	// Fetch returns a single remote issue by its identifier.
	Fetch(ctx context.Context, id string) (RemoteIssue, error)
}

// ImporterDeps holds dependencies available to importers.
type ImporterDeps struct {
	HTTP core.HTTPClient
}

// ImporterConfig holds configuration for creating an importer.
type ImporterConfig struct {
	Source string            // import source name (e.g. "linear", "plane")
	Config map[string]string // source-specific config (api_key, team, ...)
	Deps   ImporterDeps
}

// ImporterFactory creates an importer from configuration.
type ImporterFactory func(cfg ImporterConfig) (Importer, error)

// importerRegistry holds registered importer factories keyed by source name.
var importerRegistry = map[string]ImporterFactory{}

// RegisterImporter registers an importer factory under a source name.
func RegisterImporter(name string, factory ImporterFactory) {
	importerRegistry[name] = factory
}

// NewImporter creates an importer for the named source from configuration.
func NewImporter(cfg ImporterConfig) (Importer, error) {
	factory, ok := importerRegistry[cfg.Source]
	if !ok {
		return nil, fmt.Errorf("unknown import source: %s (registered: %s)", cfg.Source, strings.Join(RegisteredImporters(), ", "))
	}
	return factory(cfg)
}

// RegisteredImporters returns the sorted list of registered import-source names.
func RegisteredImporters() []string {
	names := make([]string, 0, len(importerRegistry))
	for name := range importerRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func init() {
	RegisterImporter("linear", func(cfg ImporterConfig) (Importer, error) {
		apiKey := os.Getenv("LINEAR_API_KEY")
		if apiKey == "" {
			apiKey = cfg.Config["api_key"]
		}
		if apiKey == "" {
			return nil, fmt.Errorf("LINEAR_API_KEY env var or api_key config required")
		}
		teamKey := cfg.Config["team"]
		if teamKey == "" {
			return nil, fmt.Errorf("linear import source requires 'team' config")
		}
		if cfg.Deps.HTTP == nil {
			return nil, fmt.Errorf("linear import source requires HTTP client")
		}
		return NewLinearImporter(cfg.Deps.HTTP, apiKey, teamKey), nil
	})

	RegisterImporter("plane", func(cfg ImporterConfig) (Importer, error) {
		apiKey := os.Getenv("PLANE_API_KEY")
		if apiKey == "" {
			apiKey = cfg.Config["api_key"]
		}
		if apiKey == "" {
			return nil, fmt.Errorf("PLANE_API_KEY env var or api_key config required")
		}
		workspace := cfg.Config["workspace"]
		if workspace == "" {
			return nil, fmt.Errorf("plane import source requires 'workspace' config")
		}
		project := cfg.Config["project"]
		if project == "" {
			return nil, fmt.Errorf("plane import source requires 'project' config")
		}
		baseURL := cfg.Config["base_url"]
		if baseURL == "" {
			baseURL = DefaultPlaneBaseURL
		}
		baseURL = strings.TrimRight(baseURL, "/")
		if cfg.Deps.HTTP == nil {
			return nil, fmt.Errorf("plane import source requires HTTP client")
		}
		return NewPlaneImporter(cfg.Deps.HTTP, baseURL, apiKey, workspace, project), nil
	})
}
