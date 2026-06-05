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
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/workflow"
)

// DefaultPlaneBaseURL is the API base URL for Plane Cloud.
const DefaultPlaneBaseURL = "https://api.plane.so"

// PlaneProvider implements Provider using the Plane REST API.
//
// State translation is workflow-driven: the project's workflow declares a
// provider_map.plane.<state> entry per workflow state, naming either a
// Plane state UUID (state_id) or a Plane state group (group). At read time
// the provider walks the workflow's states in order and picks the first
// whose provider_map entry matches the issue's current Plane state. At
// write time it looks up the workflow state and writes the configured
// Plane state UUID directly, or the first state in the configured group.
type PlaneProvider struct {
	http          core.HTTPClient
	baseURL       string // no trailing slash
	apiKey        string
	workspaceSlug string
	projectID     string
	wf            workflow.Workflow

	// lazily-loaded caches
	stateGroups map[string]string // state uuid -> group
	groupState  map[string]string // group -> a representative state uuid
	identifier  string            // project identifier prefix (e.g. "PROJ")
}

// NewPlaneProvider creates a provider backed by a Plane project. The
// workflow tells the provider how to translate Plane states ↔ mp's
// workflow state names; pass workflow.Default() for the historical
// behavior.
func NewPlaneProvider(httpClient core.HTTPClient, baseURL, apiKey, workspaceSlug, projectID string, wf workflow.Workflow) *PlaneProvider {
	if baseURL == "" {
		baseURL = DefaultPlaneBaseURL
	}
	return &PlaneProvider{
		http:          httpClient,
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		workspaceSlug: workspaceSlug,
		projectID:     projectID,
		wf:            wf,
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

// List returns issues from the Plane project, optionally filtered by status.
func (p *PlaneProvider) List(statusFilter []string) ([]Issue, error) {
	raw, err := p.fetchAllIssues(0)
	if err != nil {
		return nil, err
	}
	return p.toIssues(raw, statusFilter, "", 0)
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
	return p.toIssues(raw, input.Status, input.Query, limit)
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

// UpdateStatus moves an issue to a Plane state matching the given workflow
// state. The mapping comes from the workflow's provider_map.plane entry for
// status, which may name a Plane state UUID (state_id) or a Plane state
// group (group).
func (p *PlaneProvider) UpdateStatus(id string, status string) error {
	if err := p.ensureStates(); err != nil {
		return err
	}
	stateID, err := p.resolvePlaneState(status)
	if err != nil {
		return err
	}
	if _, err := p.do("PATCH", p.issuesPath(id), map[string]string{"state": stateID}); err != nil {
		return fmt.Errorf("failed to update issue status: %w", err)
	}
	return nil
}

// resolvePlaneState turns a workflow state name into a Plane state UUID. It
// prefers state_id when present; otherwise falls back to the configured
// group's representative state.
func (p *PlaneProvider) resolvePlaneState(workflowState string) (string, error) {
	entry, ok := p.wf.ProviderEntry("plane", workflowState)
	if !ok {
		return "", fmt.Errorf("workflow has no provider_map.plane entry for %q", workflowState)
	}
	if entry.StateID != "" {
		return entry.StateID, nil
	}
	if entry.Group != "" {
		if stateID, ok := p.groupState[strings.ToLower(entry.Group)]; ok {
			return stateID, nil
		}
		return "", fmt.Errorf("Plane has no state in group %q (for workflow state %q)", entry.Group, workflowState)
	}
	return "", fmt.Errorf("provider_map.plane.%s names neither state_id nor group", workflowState)
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

// toIssues converts Plane issues, resolving state groups, applying an optional
// case-insensitive title query and status filter, and capping at limit.
func (p *PlaneProvider) toIssues(raw []planeIssue, statusFilter []string, query string, limit int) ([]Issue, error) {
	if err := p.ensureStates(); err != nil {
		return nil, err
	}
	queryLower := strings.ToLower(query)
	var issues []Issue
	for _, pi := range raw {
		if query != "" && !strings.Contains(strings.ToLower(pi.Name), queryLower) {
			continue
		}
		status := p.statusForState(pi.State)
		if len(statusFilter) > 0 && !containsStatus(statusFilter, status) {
			continue
		}
		issues = append(issues, p.issueFrom(pi, status))
		if limit > 0 && len(issues) >= limit {
			break
		}
	}
	return issues, nil
}

func (p *PlaneProvider) toIssue(pi planeIssue) (Issue, error) {
	if err := p.ensureStates(); err != nil {
		return Issue{}, err
	}
	return p.issueFrom(pi, p.statusForState(pi.State)), nil
}

func (p *PlaneProvider) issueFrom(pi planeIssue, status string) Issue {
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
		Status:      status,
		Description: pi.DescriptionStripped,
	}
}

func (p *PlaneProvider) statusForState(stateID string) string {
	// Prefer an exact state_id mapping if the workflow declared one.
	for _, ws := range p.wf.AllStates() {
		entry, ok := p.wf.ProviderEntry("plane", ws)
		if !ok {
			continue
		}
		if entry.StateID != "" && entry.StateID == stateID {
			return ws
		}
	}
	// Otherwise fall back to a group match.
	if group, ok := p.stateGroups[stateID]; ok {
		gLower := strings.ToLower(group)
		for _, ws := range p.wf.AllStates() {
			entry, ok := p.wf.ProviderEntry("plane", ws)
			if !ok {
				continue
			}
			if entry.Group != "" && strings.ToLower(entry.Group) == gLower {
				return ws
			}
		}
	}
	return p.wf.Initial
}

// PlaneState is the minimal projection of a Plane workflow state returned by
// ListStates. Useful for `mp issue states --provider plane` so users can
// populate workflow.provider_map.plane without curl.
type PlaneState struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// ListStates fetches every workflow state defined on this Plane project.
func (p *PlaneProvider) ListStates() ([]PlaneState, error) {
	var out []PlaneState
	cursor := ""
	for {
		path := p.statesPath()
		if cursor != "" {
			path += "?cursor=" + url.QueryEscape(cursor)
		}
		respBody, err := p.do("GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list states: %w", err)
		}
		var env planeListResponse
		if err := json.Unmarshal(respBody, &env); err != nil {
			return nil, fmt.Errorf("failed to parse states response: %w", err)
		}
		var page []PlaneState
		if len(env.Results) > 0 {
			if err := json.Unmarshal(env.Results, &page); err != nil {
				return nil, fmt.Errorf("failed to parse states page: %w", err)
			}
		}
		out = append(out, page...)
		if !env.NextPageResults || env.NextCursor == "" {
			return out, nil
		}
		cursor = env.NextCursor
	}
}

// ensureStates loads the project's workflow states and identifier once.
func (p *PlaneProvider) ensureStates() error {
	if p.stateGroups != nil {
		return nil
	}
	// Project identifier (best-effort: a failure here is non-fatal).
	if projBody, err := p.do("GET", p.projectPath(), nil); err == nil {
		var proj struct {
			Identifier string `json:"identifier"`
		}
		if json.Unmarshal(projBody, &proj) == nil {
			p.identifier = proj.Identifier
		}
	}

	groups := map[string]string{}
	groupState := map[string]string{}
	cursor := ""
	for {
		path := p.statesPath()
		if cursor != "" {
			path += "?cursor=" + url.QueryEscape(cursor)
		}
		respBody, err := p.do("GET", path, nil)
		if err != nil {
			return fmt.Errorf("failed to list states: %w", err)
		}
		var env planeListResponse
		if err := json.Unmarshal(respBody, &env); err != nil {
			return fmt.Errorf("failed to parse states response: %w", err)
		}
		var page []struct {
			ID    string `json:"id"`
			Group string `json:"group"`
		}
		if len(env.Results) > 0 {
			if err := json.Unmarshal(env.Results, &page); err != nil {
				return fmt.Errorf("failed to parse states page: %w", err)
			}
		}
		for _, s := range page {
			groups[s.ID] = s.Group
			key := strings.ToLower(s.Group)
			if _, ok := groupState[key]; !ok {
				groupState[key] = s.ID
			}
		}
		if !env.NextPageResults || env.NextCursor == "" {
			break
		}
		cursor = env.NextCursor
	}
	p.stateGroups = groups
	p.groupState = groupState
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
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Plane API %s %s returned status %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

