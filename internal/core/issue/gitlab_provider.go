package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// GitLabImporter is a one-shot import source backed by the glab CLI.
// Reuses whatever GitLab auth glab is already configured with — no API key
// handling here. Like all importers it is read-only: it never creates or
// mutates remote state.
type GitLabImporter struct {
	exec    core.Exec
	project string // optional override; empty means "use the cwd repo"
}

// NewGitLabImporter creates an importer backed by glab.
func NewGitLabImporter(exec core.Exec, project string) *GitLabImporter {
	return &GitLabImporter{exec: exec, project: project}
}

type gitlabIssue struct {
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
}

func (i gitlabIssue) toRemoteIssue() RemoteIssue {
	return RemoteIssue{
		ID:    fmt.Sprintf("%d", i.IID),
		Title: i.Title,
		Body:  i.Description,
		URL:   i.WebURL,
	}
}

func (p *GitLabImporter) repoArgs() []string {
	if p.project == "" {
		return nil
	}
	return []string{"-R", p.project}
}

// Search returns open remote issues matching query (empty = recent issues).
func (p *GitLabImporter) Search(ctx context.Context, query string, limit int) ([]RemoteIssue, error) {
	args := append([]string{"issue", "list", "-F", "json"}, p.repoArgs()...)
	if query != "" {
		args = append(args, "--search", query)
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	args = append(args, "--per-page", fmt.Sprintf("%d", limit))

	output, err := p.exec.Run(ctx, "glab", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %s", strings.TrimSpace(string(output)))
	}

	var raw []gitlabIssue
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse issue list: %w", err)
	}

	issues := make([]RemoteIssue, 0, len(raw))
	for _, r := range raw {
		issues = append(issues, r.toRemoteIssue())
	}
	return issues, nil
}

// Fetch returns a single remote issue by iid (the string form of glab's iid).
func (p *GitLabImporter) Fetch(ctx context.Context, id string) (RemoteIssue, error) {
	args := append([]string{"issue", "view", id, "-F", "json"}, p.repoArgs()...)
	output, err := p.exec.Run(ctx, "glab", args...)
	if err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to view issue %s: %s", id, strings.TrimSpace(string(output)))
	}
	var raw gitlabIssue
	if err := json.Unmarshal(output, &raw); err != nil {
		return RemoteIssue{}, fmt.Errorf("failed to parse issue %s: %w", id, err)
	}
	return raw.toRemoteIssue(), nil
}
