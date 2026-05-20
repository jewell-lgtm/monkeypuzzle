package issue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const linearAPIURL = "https://api.linear.app/graphql"

// closedLinearStates are Linear workflow state names that map to Open=false.
// Anything else is considered Open. Workflow modelling beyond that belongs in hooks.
var closedLinearStates = map[string]bool{
	"Done":      true,
	"Canceled":  true,
	"Cancelled": true,
}

// LinearProvider resolves Linear issues via the Linear GraphQL API.
type LinearProvider struct {
	http    core.HTTPClient
	apiKey  string
	teamKey string
}

// NewLinearProvider creates a provider for Linear-based issues.
func NewLinearProvider(http core.HTTPClient, apiKey, teamKey string) *LinearProvider {
	return &LinearProvider{http: http, apiKey: apiKey, teamKey: teamKey}
}

// Get returns an issue by ID or identifier (e.g. "ABC-123").
func (p *LinearProvider) Get(id string) (Issue, error) {
	query := `query Issue($id: String!) {
		issue(id: $id) {
			id
			identifier
			title
			url
			state { name }
		}
	}`
	resp, err := p.doGraphQL(query, map[string]interface{}{"id": id})
	if err != nil {
		return Issue{}, fmt.Errorf("failed to get issue: %w", err)
	}

	var result struct {
		Data struct {
			Issue struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				URL        string `json:"url"`
				State      struct {
					Name string `json:"name"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return Issue{}, fmt.Errorf("failed to parse response: %w", err)
	}

	i := result.Data.Issue
	return Issue{
		ID:     i.ID,
		Number: i.Identifier,
		Title:  i.Title,
		URL:    i.URL,
		Open:   !closedLinearStates[i.State.Name],
	}, nil
}

// doGraphQL executes a GraphQL request against the Linear API.
func (p *LinearProvider) doGraphQL(query string, variables map[string]interface{}) ([]byte, error) {
	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", linearAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear API returned %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}
