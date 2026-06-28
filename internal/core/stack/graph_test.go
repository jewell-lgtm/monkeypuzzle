package stack

import (
	"context"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// graphPRsJSON: #1 roots on main, #2 stacks on #1, #3 is MERGED (drops out).
const graphPRsJSON = `[
  {"number":1,"headRefName":"feat-a","baseRefName":"main","state":"OPEN","url":"https://x/1","title":"A","author":{"login":"octo"},"isDraft":false},
  {"number":2,"headRefName":"feat-b","baseRefName":"feat-a","state":"OPEN","url":"https://x/2","title":"B","author":{"login":"octo"},"isDraft":true},
  {"number":3,"headRefName":"feat-c","baseRefName":"main","state":"MERGED","url":"https://x/3","title":"C","author":{"login":"octo"},"isDraft":false}
]`

func graphMock(prs string) *adapters.MockExec {
	m := adapters.NewMockExec()
	m.AddResponse("gh", []string{
		"pr", "list", "--repo", "o/n", "--state", "all",
		"--json", "number,headRefName,baseRefName,state,url,title,author,isDraft",
		"--limit", "200",
	}, []byte(prs), nil)
	return m
}

func TestGraphBuildsForestFromForge(t *testing.T) {
	h := NewHandler(core.Deps{Exec: graphMock(graphPRsJSON), Output: adapters.NewBufferOutput()})

	got, err := h.Graph(context.Background(), GraphInput{Repo: "o/n", DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if got.DefaultBranch != "main" || got.Provider != ProviderGitHub {
		t.Fatalf("unexpected meta: %+v", got)
	}
	if len(got.Stacks) != 1 {
		t.Fatalf("want 1 stack (merged #3 dropped), got %d", len(got.Stacks))
	}
	root := got.Stacks[0].Root
	if root.PR.Number != 1 || root.PR.Author != "octo" || root.PR.Title != "A" {
		t.Fatalf("want root #1 'A' by octo, got %+v", root.PR)
	}
	if len(root.Children) != 1 || root.Children[0].PR.Number != 2 {
		t.Fatalf("want child #2, got %+v", root.Children)
	}
	if !root.Children[0].PR.Draft {
		t.Errorf("want #2 draft=true")
	}
}

func TestGraphAutoDetectsDefaultBranch(t *testing.T) {
	m := graphMock("[]")
	m.AddResponse("gh", []string{"repo", "view", "o/n", "--json", "defaultBranchRef"},
		[]byte(`{"defaultBranchRef":{"name":"trunk"}}`), nil)
	h := NewHandler(core.Deps{Exec: m, Output: adapters.NewBufferOutput()})

	got, err := h.Graph(context.Background(), GraphInput{Repo: "o/n"})
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if got.DefaultBranch != "trunk" {
		t.Fatalf("want default branch trunk, got %q", got.DefaultBranch)
	}
}

func TestGraphRequiresRepo(t *testing.T) {
	h := NewHandler(core.Deps{Exec: adapters.NewMockExec(), Output: adapters.NewBufferOutput()})
	if _, err := h.Graph(context.Background(), GraphInput{}); err == nil {
		t.Fatal("want error when repo is empty")
	}
}

func TestGraphRejectsUnsupportedProvider(t *testing.T) {
	h := NewHandler(core.Deps{Exec: adapters.NewMockExec(), Output: adapters.NewBufferOutput()})
	if _, err := h.Graph(context.Background(), GraphInput{Repo: "o/n", Provider: ProviderGitLab}); err == nil {
		t.Fatal("want error for unsupported provider")
	}
}

// TestGraphSurfacesGhError: when `gh pr list` fails, Graph must surface gh's real
// diagnostic (the 404) and the repo slug — not collapse it to "set GH_TOKEN".
func TestGraphSurfacesGhError(t *testing.T) {
	m := adapters.NewMockExec()
	m.AddResponse("gh", []string{
		"pr", "list", "--repo", "o/n", "--state", "all",
		"--json", "number,headRefName,baseRefName,state,url,title,author,isDraft",
		"--limit", "200",
	}, []byte("HTTP 404: Not Found (https://api.github.com/repos/o/n/pulls)"), adapters.MockError("exit status 1"))
	h := NewHandler(core.Deps{Exec: m, Output: adapters.NewBufferOutput()})

	_, err := h.Graph(context.Background(), GraphInput{Repo: "o/n", DefaultBranch: "main"})
	if err == nil {
		t.Fatal("want error when gh pr list fails")
	}
	msg := err.Error()
	if !strings.Contains(msg, "o/n") {
		t.Errorf("want repo slug in error, got %q", msg)
	}
	if !strings.Contains(msg, "404") || !strings.Contains(msg, "Not Found") {
		t.Errorf("want gh's real diagnostic surfaced, got %q", msg)
	}
	if strings.Contains(msg, "set GH_TOKEN") {
		t.Errorf("must not collapse gh error to 'set GH_TOKEN', got %q", msg)
	}
}
