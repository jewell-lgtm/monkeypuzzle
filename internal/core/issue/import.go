package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// ImportInput holds input for the issue import command.
type ImportInput struct {
	From  string `json:"from"`  // import source name (e.g. "linear", "plane")
	ID    string `json:"id"`    // remote issue identifier (unique match by ID)
	Query string `json:"query"` // text search to resolve a remote issue
}

// ImportSchema returns the JSON-stdin schema for `mp issue import`.
func ImportSchema() ([]byte, error) {
	schema := map[string]any{
		"from":  "",
		"id":    "",
		"query": "",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseImportJSON parses JSON input into ImportInput.
func ParseImportJSON(data []byte) (ImportInput, error) {
	var in ImportInput
	if err := json.Unmarshal(data, &in); err != nil {
		return ImportInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// ImportSource describes a configured import source.
type ImportSource struct {
	Name   string            // source name (linear, plane)
	Config map[string]string // source-specific config (api_key, team, ...)
}

// ConfiguredImportSources returns the import sources available from config.
//
// Backward compatibility: a config carrying issue_provider: "linear" (or
// "plane") plus its creds is surfaced as an import source. The markdown
// provider is the local store and is never an import source.
func (h *Handler) ConfiguredImportSources() ([]ImportSource, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return nil, fmt.Errorf("failed to read config (run mp init first): %w", err)
	}

	var sources []ImportSource
	switch cfg.Issues.Provider {
	case "linear", "plane", "gitlab":
		sources = append(sources, ImportSource{
			Name:   cfg.Issues.Provider,
			Config: cfg.Issues.Config,
		})
	}

	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, nil
}

// ResolveImportSource picks the import source to use given an optional explicit
// name. With no explicit name and exactly one configured source it returns
// that; with multiple it fails loudly (ambiguity). An explicit name must match
// a configured source.
func (h *Handler) ResolveImportSource(from string) (ImportSource, error) {
	sources, err := h.ConfiguredImportSources()
	if err != nil {
		return ImportSource{}, err
	}
	return resolveImportSource(sources, from)
}

// resolveImportSource is the pure selection logic: pick from `sources` given an
// optional explicit name, failing loudly on genuine ambiguity.
func resolveImportSource(sources []ImportSource, from string) (ImportSource, error) {
	if len(sources) == 0 {
		return ImportSource{}, fmt.Errorf("no import source configured; run `mp init` and configure linear, plane, or gitlab")
	}

	if from != "" {
		for _, s := range sources {
			if s.Name == from {
				return s, nil
			}
		}
		return ImportSource{}, fmt.Errorf("import source %q is not configured (configured: %v)", from, sourceNames(sources))
	}

	if len(sources) > 1 {
		return ImportSource{}, fmt.Errorf("multiple import sources configured (%v); specify one with --from", sourceNames(sources))
	}
	return sources[0], nil
}

func sourceNames(sources []ImportSource) []string {
	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = s.Name
	}
	return names
}

// newImporter constructs an Importer for the given source.
func (h *Handler) newImporter(src ImportSource) (Importer, error) {
	return NewImporter(ImporterConfig{
		Source: src.Name,
		Config: src.Config,
		Deps:   ImporterDeps{HTTP: h.deps.HTTP, Exec: h.deps.Exec},
	})
}

// SearchRemote returns remote issues from the resolved import source matching
// query. Used by the interactive picker.
func (h *Handler) SearchRemote(ctx context.Context, src ImportSource, query string, limit int) ([]RemoteIssue, error) {
	imp, err := h.newImporter(src)
	if err != nil {
		return nil, err
	}
	h.pubLoading(true, "Searching "+src.Name+"...")
	defer h.pubLoading(false, "")
	return imp.Search(ctx, query, limit)
}

// ResolveRemoteIssue resolves a single remote issue from the source using the
// input's ID or Query. It fails loudly on ambiguity: a query matching more than
// one remote issue (with no ID) errors and names the choices.
func (h *Handler) ResolveRemoteIssue(ctx context.Context, src ImportSource, in ImportInput) (RemoteIssue, error) {
	imp, err := h.newImporter(src)
	if err != nil {
		return RemoteIssue{}, err
	}

	if in.ID != "" {
		h.pubLoading(true, "Fetching "+in.ID+"...")
		defer h.pubLoading(false, "")
		return imp.Fetch(ctx, in.ID)
	}

	if in.Query == "" {
		return RemoteIssue{}, fmt.Errorf("provide --id or --query to select a remote issue")
	}

	h.pubLoading(true, "Searching "+src.Name+"...")
	results, err := imp.Search(ctx, in.Query, DefaultSearchLimit)
	h.pubLoading(false, "")
	if err != nil {
		return RemoteIssue{}, err
	}
	switch len(results) {
	case 0:
		return RemoteIssue{}, fmt.Errorf("no %s issue matched query %q", src.Name, in.Query)
	case 1:
		return results[0], nil
	default:
		var choices []string
		for _, r := range results {
			label := r.ID
			if r.Title != "" {
				label = fmt.Sprintf("%s (%s)", r.ID, r.Title)
			}
			choices = append(choices, label)
		}
		return RemoteIssue{}, fmt.Errorf("query %q matched %d %s issues; disambiguate with --id. Matches: %v", in.Query, len(results), src.Name, choices)
	}
}

// WriteImported materialises a remote issue as a local markdown issue and
// returns the created file info.
func (h *Handler) WriteImported(remote RemoteIssue) (IssueFile, error) {
	pinfo, err := h.getProvider()
	if err != nil {
		return IssueFile{}, err
	}

	issue, err := pinfo.provider.Create(CreateInput{
		Title:       remote.Title,
		Description: importBody(remote),
	})
	if err != nil {
		return IssueFile{}, err
	}

	filename := filepath.Base(issue.ID)
	result := IssueFile{
		Path:     filepath.Join(pinfo.issuesDir, filename),
		Title:    issue.Title,
		Filename: filename,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: fmt.Sprintf("Imported %s as %s", remote.ID, result.Path),
		Data:    result,
	})

	return result, nil
}

// importBody composes the markdown body for an imported issue, appending the
// source URL as a reference line when present.
func importBody(remote RemoteIssue) string {
	body := remote.Body
	if remote.URL != "" {
		if body != "" {
			body += "\n\n"
		}
		body += "Imported from: " + remote.URL
	}
	return body
}

// Import is the non-interactive entry point: resolve source, resolve the remote
// issue, and write it locally.
func (h *Handler) Import(ctx context.Context, in ImportInput) (IssueFile, error) {
	src, err := h.ResolveImportSource(in.From)
	if err != nil {
		return IssueFile{}, err
	}
	remote, err := h.ResolveRemoteIssue(ctx, src, in)
	if err != nil {
		return IssueFile{}, err
	}
	return h.WriteImported(remote)
}
