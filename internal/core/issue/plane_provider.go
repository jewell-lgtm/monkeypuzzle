package issue

import (
	"bytes"
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

// PlaneProvider implements Provider using the Plane REST API.
type PlaneProvider struct {
	http          core.HTTPClient
	baseURL       string // no trailing slash
	apiKey        string
	workspaceSlug string
	projectID     string

	// lazily-loaded caches
	identifierLoaded bool
	identifier       string // project identifier prefix (e.g. "PROJ")
}

// NewPlaneProvider creates a provider backed by a Plane project.
func NewPlaneProvider(httpClient core.HTTPClient, baseURL, apiKey, workspaceSlug, projectID string) *PlaneProvider {
	if baseURL == "" {
		baseURL = DefaultPlaneBaseURL
	}
	return &PlaneProvider{
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
	State               string `json:"state"`
}

// planeListResponse is Plane's cursor-paginated list envelope.
type planeListResponse struct {
	Results         json.RawMessage `json:"results"`
	NextCursor      string          `json:"next_cursor"`
	NextPageResults bool            `json:"next_page_results"`
}

// Create creates a new issue in the Plane project.
func (p *PlaneProvider) Create(input CreateInput) (Issue, error) {
	body := map[string]string{
		"name":             input.Title,
		"description_html": input.Description,
	}
	respBody, err := p.do("POST", p.issuesPath(""), body)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to create issue: %w", err)
	}
	var pi planeIssue
	if err := json.Unmarshal(respBody, &pi); err != nil {
		return Issue{}, fmt.Errorf("failed to parse create response: %w", err)
	}
	return p.toIssue(pi)
}

// List returns all issues from the Plane project.
func (p *PlaneProvider) List() ([]Issue, error) {
	raw, err := p.fetchAllIssues(0)
	if err != nil {
		return nil, err
	}
	return p.toIssues(raw, "", 0)
}

// SearchIssues returns issues matching the search criteria.
// Plane has no documented title-search query parameter, so the list is fetched
// and filtered client-side (same approach as the markdown provider).
func (p *PlaneProvider) SearchIssues(input SearchInput) ([]Issue, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	raw, err := p.fetchAllIssues(limit)
	if err != nil {
		return nil, err
	}
	return p.toIssues(raw, input.Query, limit)
}

// Get returns a single issue by its Plane UUID.
func (p *PlaneProvider) Get(id string) (Issue, error) {
	respBody, err := p.do("GET", p.issuesPath(id), nil)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to get issue: %w", err)
	}
	var pi planeIssue
	if err := json.Unmarshal(respBody, &pi); err != nil {
		return Issue{}, fmt.Errorf("failed to parse issue response: %w", err)
	}
	return p.toIssue(pi)
}

// fetchAllIssues walks the cursor-paginated issues endpoint, stopping early
// once limit issues have been collected (limit <= 0 means no limit).
func (p *PlaneProvider) fetchAllIssues(limit int) ([]planeIssue, error) {
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

// toIssues converts Plane issues, applying an optional case-insensitive title
// query and capping at limit.
func (p *PlaneProvider) toIssues(raw []planeIssue, query string, limit int) ([]Issue, error) {
	if err := p.ensureIdentifier(); err != nil {
		return nil, err
	}
	queryLower := strings.ToLower(query)
	var issues []Issue
	for _, pi := range raw {
		if query != "" && !strings.Contains(strings.ToLower(pi.Name), queryLower) {
			continue
		}
		issues = append(issues, p.issueFrom(pi))
		if limit > 0 && len(issues) >= limit {
			break
		}
	}
	return issues, nil
}

func (p *PlaneProvider) toIssue(pi planeIssue) (Issue, error) {
	if err := p.ensureIdentifier(); err != nil {
		return Issue{}, err
	}
	return p.issueFrom(pi), nil
}

func (p *PlaneProvider) issueFrom(pi planeIssue) Issue {
	number := ""
	if pi.SequenceID > 0 {
		if p.identifier != "" {
			number = fmt.Sprintf("%s-%d", p.identifier, pi.SequenceID)
		} else {
			number = fmt.Sprintf("%d", pi.SequenceID)
		}
	}
	return Issue{
		ID:          pi.ID,
		Number:      number,
		Title:       pi.Name,
		Description: pi.DescriptionStripped,
	}
}

// ensureIdentifier loads the project's identifier prefix once (best-effort).
func (p *PlaneProvider) ensureIdentifier() error {
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

func (p *PlaneProvider) projectPath() string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/", p.workspaceSlug, p.projectID)
}

// issuesPath builds the issues collection path, or a single-issue path when id is non-empty.
func (p *PlaneProvider) issuesPath(id string) string {
	base := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", p.workspaceSlug, p.projectID)
	if id == "" {
		return base
	}
	return base + id + "/"
}

// do performs an HTTP request against the Plane API and returns the response body.
func (p *PlaneProvider) do(method, path string, body interface{}) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, p.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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
