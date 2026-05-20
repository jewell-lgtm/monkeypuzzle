package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// GitLabProvider resolves issues by shelling out to the glab CLI.
// Reuses whatever GitLab auth glab is already configured with — no API key
// handling here.
type GitLabProvider struct {
	exec    core.Exec
	project string // optional override; empty means "use the cwd repo"
}

// NewGitLabProvider creates a provider backed by glab.
func NewGitLabProvider(exec core.Exec, project string) *GitLabProvider {
	return &GitLabProvider{exec: exec, project: project}
}

// Get returns an issue by iid (the numeric "#123" id, as a string).
func (p *GitLabProvider) Get(id string) (Issue, error) {
	args := append([]string{"issue", "view", id, "-F", "json"}, p.repoArgs()...)
	output, err := p.exec.Run(context.Background(), "glab", args...)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to view issue %s: %s", id, strings.TrimSpace(string(output)))
	}
	var raw struct {
		IID    int    `json:"iid"`
		Title  string `json:"title"`
		State  string `json:"state"`
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return Issue{}, fmt.Errorf("failed to parse issue %s: %w", id, err)
	}
	return Issue{
		ID:     fmt.Sprintf("%d", raw.IID),
		Number: fmt.Sprintf("#%d", raw.IID),
		Title:  raw.Title,
		URL:    raw.WebURL,
		Open:   strings.ToLower(raw.State) == "opened" || strings.ToLower(raw.State) == "open",
	}, nil
}

func (p *GitLabProvider) repoArgs() []string {
	if p.project == "" {
		return nil
	}
	return []string{"-R", p.project}
}
