package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// DefaultPlaneBaseURL is the API base URL for Plane Cloud.
const DefaultPlaneBaseURL = "https://api.plane.so"

// PlaneImporter is a read-only import source backed by the Plane REST API. It
// fetches remote issues so they can be materialised as local markdown; it never
// creates or mutates Plane state.
type PlaneImporter struct {
	http          core.HTTPClient
	baseURL       string // no trailing slash
	apiKey        string
	workspaceSlug string
	projectID     string

	// lazily-loaded caches
	identifierLoaded bool
	identifier       string // project identifier prefix (e.g. "PROJ")
}

// NewPlaneImporter creates a read-only Plane import source.
func NewPlaneImporter(httpClient core.HTTPClient, baseURL, apiKey, workspaceSlug, projectID string) *PlaneImporter {
	if baseURL == "" {
		baseURL = DefaultPlaneBaseURL
	}
	return &PlaneImporter{
		http:          httpClient,
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		workspaceSlug: workspaceSlug,
		projectID:     projectID,
	}
}

// planeIssue is the subset of Plane's issue payload we use.
type planeIssue struct {
	ID                  string `json:"id"`
	SequenceID          int    `json:"sequence_id"`
	Name                string `json:"name"`
	DescriptionStripped string `json:"description_stripped"`
}

// planeListResponse is Plane's cursor-paginated list envelope.
type planeListResponse struct {
	Results         json.RawMessage `json:"results"`
	NextCursor      string          `json:"next_cursor"`
	NextPageResults bool            `json:"next_page_results"`
}

// Search returns remote issues matching query (empty = all issues), capped at
// limit (0 = DefaultSearchLimit). Plane has no documented title-search query
// parameter, so the list is fetched and filtered client-side.
func (p *PlaneImporter) Search(_ context.Context, query string, limit int) ([]RemoteIssue, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	raw, err := p.fetchAllIssues(limit)
	if err != nil {
		return nil, err
	}
	queryLower := strings.ToLower(query)
	issues := make([]RemoteIssue, 0, len(raw))
	for _, pi := range raw {
		if query != "" && !strings.Contains(strings.ToLower(pi.Name), queryLower) {
			continue
		}
		issues = append(issues, p.remoteFrom(pi))
		if len(issues) >= limit {
			break
		}
	}
	return issues, nil
}

// Fetch returns a single remote issue by its Plane UUID.
func (p *PlaneImporter) Fetch(_ context.Context, id string) (RemoteIssue, error) {
	respBody, err := p.do("GET", p.issuesPath(id), nil)
	if err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to fetch issue: %w", err)
	}
	var pi planeIssue
	if err := json.Unmarshal(respBody, &pi); err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to parse issue response: %w", err)
	}
	if pi.ID == "" && pi.Name == "" {
		return RemoteIssue{}, fmt.Errorf("plane issue not found: %s", id)
	}
	return p.remoteFrom(pi), nil
}

// fetchAllIssues walks the cursor-paginated issues endpoint, stopping early
// once limit issues have been collected (limit <= 0 means no limit).
func (p *PlaneImporter) fetchAllIssues(limit int) ([]planeIssue, error) {
	var all []planeIssue
	cursor := ""
	for {
		q := url.Values{}
		q.Set("per_page", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		respBody, err := p.do("GET", p.issuesPath("")+"?"+q.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list issues: %w", err)
		}
		var env planeListResponse
		if err := json.Unmarshal(respBody, &env); err != nil {
			return nil, fmt.Errorf("failed to parse issues response: %w", err)
		}
		var page []planeIssue
		if len(env.Results) > 0 {
			if err := json.Unmarshal(env.Results, &page); err != nil {
				return nil, fmt.Errorf("failed to parse issues page: %w", err)
			}
		}
		all = append(all, page...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		if !env.NextPageResults || env.NextCursor == "" {
			return all, nil
		}
		cursor = env.NextCursor
	}
}

// remoteFrom converts a Plane issue into a RemoteIssue, loading the project
// identifier prefix lazily for a human-readable ID.
func (p *PlaneImporter) remoteFrom(pi planeIssue) RemoteIssue {
	_ = p.ensureIdentifier()
	id := pi.ID
	if pi.SequenceID > 0 && p.identifier != "" {
		id = fmt.Sprintf("%s-%d", p.identifier, pi.SequenceID)
	}
	return RemoteIssue{
		ID:    id,
		Title: pi.Name,
		Body:  pi.DescriptionStripped,
		URL:   p.issueURL(pi.ID),
	}
}

func (p *PlaneImporter) issueURL(uuid string) string {
	if uuid == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/projects/%s/issues/%s", p.baseURL, p.workspaceSlug, p.projectID, uuid)
}

// ensureIdentifier loads the project's identifier prefix once (best-effort).
func (p *PlaneImporter) ensureIdentifier() error {
	if p.identifierLoaded {
		return nil
	}
	p.identifierLoaded = true
	if projBody, err := p.do("GET", p.projectPath(), nil); err == nil {
		var proj struct {
			Identifier string `json:"identifier"`
		}
		if json.Unmarshal(projBody, &proj) == nil {
			p.identifier = proj.Identifier
		}
	}
	return nil
}

func (p *PlaneImporter) projectPath() string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/", p.workspaceSlug, p.projectID)
}

// issuesPath builds the issues collection path, or a single-issue path when id is non-empty.
func (p *PlaneImporter) issuesPath(id string) string {
	base := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", p.workspaceSlug, p.projectID)
	if id == "" {
		return base
	}
	return base + id + "/"
}

// do performs an HTTP GET against the Plane API and returns the response body.
// Only read methods are used; the importer never mutates remote state.
func (p *PlaneImporter) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, p.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plane API %s %s returned status %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
