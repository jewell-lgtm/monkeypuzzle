package pr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

func newProvider(t *testing.T, kind string, exec *adapters.MockExec) pr.Provider {
	t.Helper()
	p, err := pr.NewProvider(pr.ProviderConfig{
		ProviderType: kind,
		Deps:         pr.ProviderDeps{Exec: exec},
	})
	if err != nil {
		t.Fatalf("NewProvider(%q): %v", kind, err)
	}
	return p
}

// TestProvider_ListPRs_RoutesToForgeCLI proves the configured provider type
// dispatches ListPRs to the right forge CLI and returns canonical PRInfo.
func TestProvider_ListPRs_RoutesToForgeCLI(t *testing.T) {
	t.Run("gitlab uses glab", func(t *testing.T) {
		exec := adapters.NewMockExec()
		exec.AddResponse("glab", []string{"mr", "list", "--all", "--per-page", "100", "-F", "json"},
			[]byte(`[{"iid":3,"source_branch":"feat","target_branch":"main","state":"opened","web_url":"https://gl/x/-/merge_requests/3"}]`), nil)

		prs, err := newProvider(t, "gitlab", exec).ListPRs(context.Background(), "/repo")
		if err != nil {
			t.Fatalf("ListPRs: %v", err)
		}
		if !exec.WasCalled("glab", "mr", "list", "--all", "--per-page", "100", "-F", "json") {
			t.Error("expected glab mr list to be called")
		}
		if exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200") {
			t.Error("gh must not be called for a gitlab provider")
		}
		if len(prs) != 1 || prs[0].Number != 3 || prs[0].State != "OPEN" || prs[0].BaseRefName != "main" {
			t.Errorf("unexpected PRInfo: %+v", prs)
		}
	})

	t.Run("github uses gh", func(t *testing.T) {
		exec := adapters.NewMockExec()
		exec.AddResponse("gh", []string{"pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200"},
			[]byte(`[{"number":5,"headRefName":"feat","baseRefName":"main","state":"OPEN","url":"https://gh/x/pull/5"}]`), nil)

		prs, err := newProvider(t, "github", exec).ListPRs(context.Background(), "/repo")
		if err != nil {
			t.Fatalf("ListPRs: %v", err)
		}
		if !exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200") {
			t.Error("expected gh pr list to be called")
		}
		if len(prs) != 1 || prs[0].Number != 5 {
			t.Errorf("unexpected PRInfo: %+v", prs)
		}
	})
}

// TestProvider_ListPRs_Unavailable proves each provider maps its adapter's
// forge-CLI-unavailable error to the neutral pr.ErrProviderUnavailable sentinel.
func TestProvider_ListPRs_Unavailable(t *testing.T) {
	for _, kind := range []string{"github", "gitlab"} {
		t.Run(kind, func(t *testing.T) {
			exec := adapters.NewMockExec() // no responses -> every call errors
			_, err := newProvider(t, kind, exec).ListPRs(context.Background(), "/repo")
			if !errors.Is(err, pr.ErrProviderUnavailable) {
				t.Errorf("%s ListPRs error = %v, want ErrProviderUnavailable", kind, err)
			}
		})
	}
}

// TestProvider_SetPRBase_RoutesToForgeCLI proves base re-pointing dispatches to
// the right forge CLI per provider.
func TestProvider_SetPRBase_RoutesToForgeCLI(t *testing.T) {
	t.Run("gitlab", func(t *testing.T) {
		exec := adapters.NewMockExec()
		exec.AddResponse("glab", []string{"mr", "update", "7", "--target-branch", "main"}, nil, nil)
		if err := newProvider(t, "gitlab", exec).SetPRBase(context.Background(), "/repo", 7, "main"); err != nil {
			t.Fatalf("SetPRBase: %v", err)
		}
		if !exec.WasCalled("glab", "mr", "update", "7", "--target-branch", "main") {
			t.Error("expected glab mr update --target-branch")
		}
	})

	t.Run("github", func(t *testing.T) {
		exec := adapters.NewMockExec()
		exec.AddResponse("gh", []string{"pr", "edit", "7", "--base", "main"}, nil, nil)
		if err := newProvider(t, "github", exec).SetPRBase(context.Background(), "/repo", 7, "main"); err != nil {
			t.Fatalf("SetPRBase: %v", err)
		}
		if !exec.WasCalled("gh", "pr", "edit", "7", "--base", "main") {
			t.Error("expected gh pr edit --base")
		}
	})
}
