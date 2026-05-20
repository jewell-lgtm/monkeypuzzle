package pr

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
)

// GitLabProvider implements Provider by delegating to adapters.GitLab (which shells out to glab).
type GitLabProvider struct {
	gl *adapters.GitLab
}

// NewGitLabProvider wraps an existing GitLab adapter as a pr.Provider.
func NewGitLabProvider(gl *adapters.GitLab) *GitLabProvider {
	return &GitLabProvider{gl: gl}
}

func (p *GitLabProvider) Push(ctx context.Context, workDir string) error {
	return p.gl.Push(ctx, workDir)
}

func (p *GitLabProvider) Create(ctx context.Context, workDir string, input CreateInput) (*CreateResult, error) {
	res, err := p.gl.CreatePR(ctx, workDir, adapters.PRCreateInput{
		Title: input.Title,
		Body:  input.Body,
		Base:  input.Base,
		Draft: input.Draft,
	})
	if err != nil {
		return nil, err
	}
	return &CreateResult{Number: res.Number, URL: res.URL}, nil
}

func (p *GitLabProvider) MarkReady(ctx context.Context, workDir string, number int) error {
	return p.gl.MarkPRReady(ctx, workDir, number)
}

func (p *GitLabProvider) GetStatus(ctx context.Context, workDir string, number int) (string, error) {
	return p.gl.GetPRStatus(ctx, workDir, number)
}

func (p *GitLabProvider) IsMerged(ctx context.Context, workDir string, number int) (bool, error) {
	return p.gl.IsPRMerged(ctx, workDir, number)
}

func (p *GitLabProvider) FindMergedByBranch(ctx context.Context, workDir, branchName string) (bool, int, error) {
	return p.gl.FindMergedPRByBranch(ctx, workDir, branchName)
}
