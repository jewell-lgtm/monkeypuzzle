package stack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// writeProviderConfig writes a monkeypuzzle.json selecting the given PR provider.
func writeProviderConfig(t *testing.T, fs *adapters.MemoryFS, repoRoot, provider string) {
	t.Helper()
	cfg := `{"version":"1","project":{"name":"test"},"pr":{"provider":"` + provider + `","config":{}}}`
	path := projectdir.ConfigFilePath(repoRoot)
	_ = fs.MkdirAll(filepath.Dir(path), 0755)
	if err := fs.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func newStackHandler(fs *adapters.MemoryFS, exec *adapters.MockExec) *Handler {
	return NewHandler(core.Deps{FS: fs, Exec: exec, Output: adapters.NewBufferOutput()})
}

// TestProviderForRepo_RoutesToConfiguredForge proves mp stack resolves the PR/MR
// provider from monkeypuzzle.json and dispatches to that forge's CLI — so
// stack base-reconciliation works on GitLab, not just GitHub.
func TestProviderForRepo_RoutesToConfiguredForge(t *testing.T) {
	t.Run("gitlab repo hits glab, not gh", func(t *testing.T) {
		fs := adapters.NewMemoryFS()
		exec := adapters.NewMockExec()
		writeProviderConfig(t, fs, "/repo", "gitlab")
		exec.AddResponse("glab", []string{"mr", "list", "--all", "--per-page", "100", "-F", "json"},
			[]byte(`[{"iid":1,"source_branch":"a","target_branch":"main","state":"opened","web_url":"https://gl/x/-/merge_requests/1"}]`), nil)

		provider, err := newStackHandler(fs, exec).providerForRepo("/repo")
		if err != nil {
			t.Fatalf("providerForRepo: %v", err)
		}
		prs, err := provider.ListPRs(context.Background(), "/repo")
		if err != nil {
			t.Fatalf("ListPRs: %v", err)
		}
		if !exec.WasCalled("glab", "mr", "list", "--all", "--per-page", "100", "-F", "json") {
			t.Error("expected glab to be invoked for a gitlab repo")
		}
		if exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url", "--limit", "200") {
			t.Error("gh must not be invoked for a gitlab repo")
		}
		if len(prs) != 1 || prs[0].State != "OPEN" {
			t.Errorf("unexpected PRs: %+v", prs)
		}
	})

	t.Run("missing provider defaults to github", func(t *testing.T) {
		fs := adapters.NewMemoryFS()
		exec := adapters.NewMockExec()
		writeProviderConfig(t, fs, "/repo", "")
		exec.AddResponse("gh", []string{"pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url", "--limit", "200"},
			[]byte(`[]`), nil)

		provider, err := newStackHandler(fs, exec).providerForRepo("/repo")
		if err != nil {
			t.Fatalf("providerForRepo: %v", err)
		}
		if _, err := provider.ListPRs(context.Background(), "/repo"); err != nil {
			t.Fatalf("ListPRs: %v", err)
		}
		if !exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url", "--limit", "200") {
			t.Error("expected gh to be invoked when provider is unset (github default)")
		}
	})
}

func TestSplitRemoteRef(t *testing.T) {
	tests := []struct {
		from       string
		wantRemote string
		wantRef    string
	}{
		{"origin/main", "origin", "main"},
		{"upstream/dev", "upstream", "dev"},
		{"origin/feature/x", "origin", "feature/x"}, // branch keeps its slashes
		{"main", "", "main"},                        // bare ref, no remote
		{"", "", ""},
	}
	for _, tt := range tests {
		remote, ref := splitRemoteRef(tt.from)
		if remote != tt.wantRemote || ref != tt.wantRef {
			t.Errorf("splitRemoteRef(%q) = (%q, %q), want (%q, %q)", tt.from, remote, ref, tt.wantRemote, tt.wantRef)
		}
	}
}
