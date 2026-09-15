package stack

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
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
		if exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200") {
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
		exec.AddResponse("gh", []string{"pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200"},
			[]byte(`[]`), nil)

		provider, err := newStackHandler(fs, exec).providerForRepo("/repo")
		if err != nil {
			t.Fatalf("providerForRepo: %v", err)
		}
		if _, err := provider.ListPRs(context.Background(), "/repo"); err != nil {
			t.Fatalf("ListPRs: %v", err)
		}
		if !exec.WasCalled("gh", "pr", "list", "--state", "all", "--json", "number,headRefName,baseRefName,state,url,isDraft", "--limit", "200") {
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

// TestPushSynced_SkipsMergedPiece proves `mp stack sync --push` never pushes a
// piece that is already merged: re-creating its branch on the forge resurrects
// a base that was deleted on merge and breaks the retarget of child PRs.
func TestPushSynced_SkipsMergedPiece(t *testing.T) {
	const mainBr = "main"
	tests := []struct {
		name       string
		merged     bool
		force      bool
		wantPush   []string
		wantMerged bool
	}{
		{name: "merged piece is not pushed", merged: true, wantMerged: true},
		{name: "merged piece is not force-pushed", merged: true, force: true, wantMerged: true},
		{name: "unmerged piece is pushed", merged: false, wantPush: []string{"push", "-u", "origin", "HEAD"}},
		{name: "unmerged rebased piece is force-pushed", merged: false, force: true, wantPush: []string{"push", "--force-with-lease", "origin", "HEAD"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := adapters.NewMemoryFS()
			exec := adapters.NewMockExec()
			h := newStackHandler(fs, exec)

			// Non-git temp dir: projectdir.WorktreeDir falls back to <wt>/.monkeypuzzle.
			piecesDir := t.TempDir()
			wt := filepath.Join(piecesDir, "a")
			if err := piece.WritePieceMetadata(wt, piece.PieceMetadata{Parent: "main", Merged: tt.merged}, fs); err != nil {
				t.Fatalf("WritePieceMetadata: %v", err)
			}
			// Git fallbacks so the unmerged case reports "not merged" cleanly.
			exec.AddResponse("git", []string{"ls-remote", "--heads", "origin", "a"}, []byte(""), nil)
			exec.AddResponse("git", []string{"rev-list", "--left-right", "--count", "main...a"}, []byte("0\t1\n"), nil)
			exec.AddResponse("git", []string{"branch", "--merged", mainBr}, []byte("* main\n"), nil)
			exec.AddResponse("git", []string{"cherry", mainBr, "a"}, []byte("+ deadbeef\n"), nil)
			exec.AddResponse("git", []string{"rev-parse", "a"}, []byte("deadbeef\n"), nil)
			exec.AddResponse("git", []string{"merge-base", "--is-ancestor", "deadbeef", mainBr}, nil, fmt.Errorf("exit status 1"))
			exec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, nil, nil)
			exec.AddResponse("git", []string{"push", "--force-with-lease", "origin", "HEAD"}, nil, nil)

			var result SyncResult
			h.pushSynced(context.Background(), piecesDir, "a", mainBr, tt.force, &result)

			pushed := false
			for _, c := range exec.GetCalls() {
				if c.Name == "git" && len(c.Args) > 0 && c.Args[0] == "push" {
					pushed = true
					if tt.wantPush == nil {
						t.Errorf("unexpected push: git %v", c.Args)
					} else if strings.Join(c.Args, " ") != strings.Join(tt.wantPush, " ") {
						t.Errorf("push args = %v, want %v", c.Args, tt.wantPush)
					}
				}
			}
			if tt.wantPush != nil && !pushed {
				t.Errorf("expected git %v, no push ran", tt.wantPush)
			}
			if got := len(result.Merged) == 1 && result.Merged[0] == "a"; got != tt.wantMerged {
				t.Errorf("result.Merged = %v, want listed=%v", result.Merged, tt.wantMerged)
			}
			if wantPushed := tt.wantPush != nil; (len(result.Pushed) == 1) != wantPushed {
				t.Errorf("result.Pushed = %v, want listed=%v", result.Pushed, wantPushed)
			}
		})
	}
}
