package issue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// DefaultPlaneBaseURL is the API base URL for Plane Cloud.
const DefaultPlaneBaseURL = "https://api.plane.so"

// closedPlaneGroups are Plane state groups that map to Open=false.
var closedPlaneGroups = map[string]bool{
	"completed": true,
	"cancelled": true,
	"canceled":  true,
}

// PlaneProvider resolves issues via the Plane REST API.
type PlaneProvider struct {
	http          core.HTTPClient
	baseURL       string // no trailing slash
	apiKey        string
	workspaceSlug string
	projectID     string

	// lazily-loaded caches
	stateGroups map[string]string // state uuid -> group
	identifier  string            // project identifier prefix (e.g. "PROJ")
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
	ID         string `json:"id"`
	SequenceID int    `json:"sequence_id"`
	Name       string `json:"name"`
	State      string `json:"state"`
}

// Get returns an issue by its Plane UUID.
func (p *PlaneProvider) Get(id string) (Issue, error) {
	respBody, err := p.do("GET", p.issuesPath(id), nil)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to get issue: %w", err)
	}
	var pi planeIssue
	if err := json.Unmarshal(respBody, &pi); err != nil {
		return Issue{}, fmt.Errorf("failed to parse issue response: %w", err)
	}
	if err := p.ensureStates(); err != nil {
		return Issue{}, err
	}
	number := ""
	if pi.SequenceID > 0 {
		if p.identifier != "" {
			number = fmt.Sprintf("%s-%d", p.identifier, pi.SequenceID)
		} else {
			number = fmt.Sprintf("%d", pi.SequenceID)
		}
	}
	group := strings.ToLower(p.stateGroups[pi.State])
	return Issue{
		ID:     pi.ID,
		Number: number,
		Title:  pi.Name,
		URL:    p.issueWebURL(pi.ID),
		Open:   !closedPlaneGroups[group],
	}, nil
}

// ensureStates loads the project's workflow states and identifier once.
func (p *PlaneProvider) ensureStates() error {
	if p.stateGroups != nil {
		return nil
	}
	if projBody, err := p.do("GET", p.projectPath(), nil); err == nil {
		var proj struct {
			Identifier string `json:"identifier"`
		}
		if err := json.Unmarshal(projBody, &proj); err == nil {
			p.identifier = proj.Identifier
		}
	}
	statesBody, err := p.do("GET", p.statesPath(), nil)
	if err != nil {
		return fmt.Errorf("failed to load Plane states: %w", err)
	}
	var states []struct {
		ID    string `json:"id"`
		Group string `json:"group"`
	}
	// Plane returns either a bare array or a {results: [...]} envelope depending on endpoint.
	if err := json.Unmarshal(statesBody, &states); err != nil {
		var env struct {
			Results json.RawMessage `json:"results"`
		}
		if err2 := json.Unmarshal(statesBody, &env); err2 == nil && len(env.Results) > 0 {
			if err := json.Unmarshal(env.Results, &states); err != nil {
				return fmt.Errorf("failed to parse Plane states: %w", err)
			}
		} else {
			return fmt.Errorf("failed to parse Plane states: %w", err)
		}
	}
	p.stateGroups = make(map[string]string, len(states))
	for _, s := range states {
		p.stateGroups[s.ID] = s.Group
	}
	return nil
}

func (p *PlaneProvider) projectPath() string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/", p.workspaceSlug, p.projectID)
}

func (p *PlaneProvider) statesPath() string {
	return fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", p.workspaceSlug, p.projectID)
}

// issuesPath builds the issues collection path, or a single-issue path when id is non-empty.
func (p *PlaneProvider) issuesPath(id string) string {
	base := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", p.workspaceSlug, p.projectID)
	if id == "" {
		return base
	}
	return base + id + "/"
}

// issueWebURL returns a best-effort browser URL for a Plane issue.
func (p *PlaneProvider) issueWebURL(id string) string {
	// Plane web URL convention: <baseHost>/<workspace>/projects/<projectID>/issues/<issueID>
	// baseURL is the API host; not always the same as the web host. Best effort: replace api. prefix.
	host := strings.Replace(p.baseURL, "://api.", "://", 1)
	return fmt.Sprintf("%s/%s/projects/%s/issues/%s", host, p.workspaceSlug, p.projectID, id)
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
