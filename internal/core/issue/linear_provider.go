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

const linearAPIURL = "https://api.linear.app/graphql"

// linearToMpStatus maps Linear workflow state names to mp statuses
var linearToMpStatus = map[string]string{
	"Backlog":     "todo",
	"Todo":        "todo",
	"Triage":      "todo",
	"In Progress": "in-progress",
	"In Review":   "in-progress",
	"Done":        "done",
	"Canceled":    "done",
	"Cancelled":   "done",
}

// LinearProvider implements Provider using Linear API
type LinearProvider struct {
	http    core.HTTPClient
	apiKey  string
	teamKey string
}

// NewLinearProvider creates a provider for Linear-based issues
func NewLinearProvider(http core.HTTPClient, apiKey, teamKey string) *LinearProvider {
	return &LinearProvider{
		http:    http,
		apiKey:  apiKey,
		teamKey: teamKey,
	}
}

// Create creates a new issue in Linear
func (p *LinearProvider) Create(input CreateInput) (Issue, error) {
	query := `mutation CreateIssue($title: String!, $description: String, $teamId: String!) {
		issueCreate(input: {title: $title, description: $description, teamId: $teamId}) {
			issue {
				id
				identifier
				title
				description
				state {
					name
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"title":       input.Title,
		"description": input.Description,
		"teamId":      p.teamKey,
	}

	resp, err := p.doGraphQL(query, variables)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to create issue: %w", err)
	}

	var result struct {
		Data struct {
			IssueCreate struct {
				Issue struct {
					ID          string `json:"id"`
					Identifier  string `json:"identifier"`
					Title       string `json:"title"`
					Description string `json:"description"`
					State       struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return Issue{}, fmt.Errorf("failed to parse response: %w", err)
	}

	issue := result.Data.IssueCreate.Issue
	return Issue{
		ID:          issue.ID,
		Number:      issue.Identifier,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      mapLinearStatus(issue.State.Name),
	}, nil
}

// List returns issues from Linear, optionally filtered by status
func (p *LinearProvider) List(statusFilter []string) ([]Issue, error) {
	query := `query Issues($teamId: String!) {
		issues(filter: {team: {key: {eq: $teamId}}}) {
			nodes {
				id
				identifier
				title
				description
				state {
					name
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"teamId": p.teamKey,
	}

	resp, err := p.doGraphQL(query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	var result struct {
		Data struct {
			Issues struct {
				Nodes []struct {
					ID          string `json:"id"`
					Identifier  string `json:"identifier"`
					Title       string `json:"title"`
					Description string `json:"description"`
					State       struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var issues []Issue
	for _, node := range result.Data.Issues.Nodes {
		status := mapLinearStatus(node.State.Name)

		// Apply status filter
		if len(statusFilter) > 0 && !containsStatus(statusFilter, status) {
			continue
		}

		issues = append(issues, Issue{
			ID:          node.ID,
			Number:      node.Identifier,
			Title:       node.Title,
			Description: node.Description,
			Status:      status,
		})
	}

	return issues, nil
}

// SearchIssues returns issues matching search criteria from Linear
func (p *LinearProvider) SearchIssues(input SearchInput) ([]Issue, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	// Use server-side search when query provided
	if input.Query != "" {
		return p.searchIssuesWithQuery(input.Query, input.Status, limit)
	}

	// No query - use regular list with optional status filter
	return p.listIssuesWithLimit(input.Status, limit)
}

// searchIssuesWithQuery uses Linear's issues query with title filter for text search
func (p *LinearProvider) searchIssuesWithQuery(query string, statusFilter []string, limit int) ([]Issue, error) {
	gql := `query issues($teamKey: String!, $first: Int!, $query: String!) {
		issues(
			filter: {
				team: {key: {eq: $teamKey}}
				title: {containsIgnoreCase: $query}
			}
			first: $first
			orderBy: updatedAt
		) {
			nodes {
				id
				identifier
				title
				description
				state {
					name
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"teamKey": p.teamKey,
		"query":   query,
		"first":   limit,
	}

	resp, err := p.doGraphQL(gql, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to search issues: %w", err)
	}

	var result struct {
		Data struct {
			Issues struct {
				Nodes []struct {
					ID          string `json:"id"`
					Identifier  string `json:"identifier"`
					Title       string `json:"title"`
					Description string `json:"description"`
					State       struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var issues []Issue
	for _, node := range result.Data.Issues.Nodes {
		status := mapLinearStatus(node.State.Name)

		// Apply status filter client-side
		if len(statusFilter) > 0 && !containsStatus(statusFilter, status) {
			continue
		}

		issues = append(issues, Issue{
			ID:          node.ID,
			Number:      node.Identifier,
			Title:       node.Title,
			Description: node.Description,
			Status:      status,
		})
	}

	return issues, nil
}

// listIssuesWithLimit fetches issues without text search
func (p *LinearProvider) listIssuesWithLimit(statusFilter []string, limit int) ([]Issue, error) {
	gql := `query issues($teamKey: String!, $first: Int!) {
		issues(
			filter: {team: {key: {eq: $teamKey}}}
			first: $first
			orderBy: createdAt
		) {
			nodes {
				id
				identifier
				title
				description
				state {
					name
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"teamKey": p.teamKey,
		"first":   limit,
	}

	resp, err := p.doGraphQL(gql, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	var result struct {
		Data struct {
			Issues struct {
				Nodes []struct {
					ID          string `json:"id"`
					Identifier  string `json:"identifier"`
					Title       string `json:"title"`
					Description string `json:"description"`
					State       struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var issues []Issue
	for _, node := range result.Data.Issues.Nodes {
		status := mapLinearStatus(node.State.Name)

		// Apply status filter
		if len(statusFilter) > 0 && !containsStatus(statusFilter, status) {
			continue
		}

		issues = append(issues, Issue{
			ID:          node.ID,
			Number:      node.Identifier,
			Title:       node.Title,
			Description: node.Description,
			Status:      status,
		})
	}

	return issues, nil
}

// Get returns an issue by ID
func (p *LinearProvider) Get(id string) (Issue, error) {
	query := `query Issue($id: String!) {
		issue(id: $id) {
			id
			identifier
			title
			description
			state {
				name
			}
		}
	}`

	variables := map[string]interface{}{
		"id": id,
	}

	resp, err := p.doGraphQL(query, variables)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to get issue: %w", err)
	}

	var result struct {
		Data struct {
			Issue struct {
				ID          string `json:"id"`
				Identifier  string `json:"identifier"`
				Title       string `json:"title"`
				Description string `json:"description"`
				State       struct {
					Name string `json:"name"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return Issue{}, fmt.Errorf("failed to parse response: %w", err)
	}

	issue := result.Data.Issue
	return Issue{
		ID:          issue.ID,
		Number:      issue.Identifier,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      mapLinearStatus(issue.State.Name),
	}, nil
}

// UpdateStatus updates the status of an issue
func (p *LinearProvider) UpdateStatus(id string, status string) error {
	// First, we need to find the appropriate state ID for the target status
	// For now, we'll use a mutation that accepts state name
	query := `mutation UpdateIssue($id: String!, $stateId: String!) {
		issueUpdate(id: $id, input: {stateId: $stateId}) {
			issue {
				id
				state {
					name
				}
			}
		}
	}`

	// Map mp status to Linear state ID (simplified - real impl would query states)
	stateID := mapMpStatusToLinearState(status)

	variables := map[string]interface{}{
		"id":      id,
		"stateId": stateID,
	}

	_, err := p.doGraphQL(query, variables)
	if err != nil {
		return fmt.Errorf("failed to update issue status: %w", err)
	}

	return nil
}

// doGraphQL executes a GraphQL request
func (p *LinearProvider) doGraphQL(query string, variables map[string]interface{}) ([]byte, error) {
	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", linearAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// mapLinearStatus converts Linear state name to mp status
func mapLinearStatus(linearState string) string {
	if status, ok := linearToMpStatus[linearState]; ok {
		return status
	}
	// Default: unknown states map to todo
	if strings.Contains(strings.ToLower(linearState), "progress") ||
		strings.Contains(strings.ToLower(linearState), "review") {
		return "in-progress"
	}
	if strings.Contains(strings.ToLower(linearState), "done") ||
		strings.Contains(strings.ToLower(linearState), "complete") ||
		strings.Contains(strings.ToLower(linearState), "cancel") {
		return "done"
	}
	return "todo"
}

// mapMpStatusToLinearState converts mp status to Linear state ID placeholder
// In real usage, this would need to query the team's workflow states
func mapMpStatusToLinearState(status string) string {
	switch status {
	case "in-progress":
		return "started"
	case "done":
		return "completed"
	default:
		return "backlog"
	}
}
