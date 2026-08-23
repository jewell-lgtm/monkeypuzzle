package pr

import (
	"context"
	"errors"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
)

// toPRInfos converts adapter PRInfo rows (which already carry canonical State)
// into the provider-neutral pr.PRInfo type.
func toPRInfos(rows []adapters.PRInfo) []PRInfo {
	out := make([]PRInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, PRInfo{
			Number:      r.Number,
			HeadRefName: r.HeadRefName,
			BaseRefName: r.BaseRefName,
			State:       r.State,
			URL:         r.URL,
			Draft:       r.IsDraft,
		})
	}
	return out
}

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

func (p *GitHubProvider) ListPRs(ctx context.Context, workDir string) ([]PRInfo, error) {
	rows, err := p.gh.ListPRs(ctx, workDir)
	if errors.Is(err, adapters.ErrGHUnavailable) {
		return nil, ErrProviderUnavailable
	}
	if err != nil {
		return nil, err
	}
	return toPRInfos(rows), nil
}

func (p *GitHubProvider) SetPRBase(ctx context.Context, workDir string, number int, base string) error {
	return p.gh.SetPRBase(ctx, workDir, number, base)
}
