package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const linearAPIURL = "https://api.linear.app/graphql"

// LinearImporter is a read-only import source backed by the Linear GraphQL API.
// It fetches remote issues so they can be materialised as local markdown; it
// never creates or mutates Linear state.
type LinearImporter struct {
	http    core.HTTPClient
	apiKey  string
	teamKey string
}

// NewLinearImporter creates a read-only Linear import source.
func NewLinearImporter(http core.HTTPClient, apiKey, teamKey string) *LinearImporter {
	return &LinearImporter{
		http:    http,
		apiKey:  apiKey,
		teamKey: teamKey,
	}
}

// linearNode is the subset of a Linear issue payload we consume.
type linearNode struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

func (n linearNode) toRemote() RemoteIssue {
	return RemoteIssue{
		ID:    n.Identifier,
		Title: n.Title,
		Body:  n.Description,
		URL:   n.URL,
	}
}

// Search returns remote issues matching query (empty = recent issues), capped
// at limit (0 = DefaultSearchLimit).
func (p *LinearImporter) Search(_ context.Context, query string, limit int) ([]RemoteIssue, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	var gql string
	variables := map[string]interface{}{
		"teamKey": p.teamKey,
		"first":   limit,
	}
	if query != "" {
		gql = `query issues($teamKey: String!, $first: Int!, $query: String!) {
			issues(
				filter: {
					team: {key: {eq: $teamKey}}
					title: {containsIgnoreCase: $query}
				}
				first: $first
				orderBy: updatedAt
			) {
				nodes { id identifier title description url }
			}
		}`
		variables["query"] = query
	} else {
		gql = `query issues($teamKey: String!, $first: Int!) {
			issues(
				filter: {team: {key: {eq: $teamKey}}}
				first: $first
				orderBy: updatedAt
			) {
				nodes { id identifier title description url }
			}
		}`
	}

	resp, err := p.doGraphQL(gql, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to search issues: %w", err)
	}

	var result struct {
		Data struct {
			Issues struct {
				Nodes []linearNode `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	issues := make([]RemoteIssue, 0, len(result.Data.Issues.Nodes))
	for _, node := range result.Data.Issues.Nodes {
		issues = append(issues, node.toRemote())
	}
	return issues, nil
}

// Fetch returns a single remote issue by its Linear identifier (UUID or human
// key like "ENG-123").
func (p *LinearImporter) Fetch(_ context.Context, id string) (RemoteIssue, error) {
	query := `query Issue($id: String!) {
		issue(id: $id) {
			id
			identifier
			title
			description
			url
		}
	}`
	variables := map[string]interface{}{"id": id}

	resp, err := p.doGraphQL(query, variables)
	if err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to fetch issue: %w", err)
	}

	var result struct {
		Data struct {
			Issue linearNode `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Data.Issue.ID == "" && result.Data.Issue.Title == "" {
		return RemoteIssue{}, fmt.Errorf("linear issue not found: %s", id)
	}
	return result.Data.Issue.toRemote(), nil
}

// doGraphQL executes a GraphQL request.
func (p *LinearImporter) doGraphQL(query string, variables map[string]interface{}) ([]byte, error) {
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
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
