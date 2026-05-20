package pr

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
)

// GitHubProvider implements Provider by delegating to adapters.GitHub (which shells out to gh).
type GitHubProvider struct {
	gh *adapters.GitHub
}

// NewGitHubProvider wraps an existing GitHub adapter as a pr.Provider.
func NewGitHubProvider(gh *adapters.GitHub) *GitHubProvider {
	return &GitHubProvider{gh: gh}
}

func (p *GitHubProvider) Push(ctx context.Context, workDir string) error {
	return p.gh.Push(ctx, workDir)
}

func (p *GitHubProvider) Create(ctx context.Context, workDir string, input CreateInput) (*CreateResult, error) {
	res, err := p.gh.CreatePR(ctx, workDir, adapters.PRCreateInput{
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

func (p *GitHubProvider) MarkReady(ctx context.Context, workDir string, number int) error {
	return p.gh.MarkPRReady(ctx, workDir, number)
}

func (p *GitHubProvider) GetStatus(ctx context.Context, workDir string, number int) (string, error) {
	return p.gh.GetPRStatus(ctx, workDir, number)
}

func (p *GitHubProvider) IsMerged(ctx context.Context, workDir string, number int) (bool, error) {
	return p.gh.IsPRMerged(ctx, workDir, number)
}

func (p *GitHubProvider) FindMergedByBranch(ctx context.Context, workDir, branchName string) (bool, int, error) {
	return p.gh.FindMergedPRByBranch(ctx, workDir, branchName)
}
