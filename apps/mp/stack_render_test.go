package main

import (
	"testing"

	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

func TestRenderStackStatus_TreeWithPRsAndDrift(t *testing.T) {
	result := stackcmd.StackStatusResult{
		MainBranch:   "main",
		ForgeChecked: true,
		Tree: &stackcmd.StackNode{
			Children: []*stackcmd.StackNode{
				{
					Piece: "api-rate-limit",
					PR:    &stackcmd.StackNodePR{Number: 6, State: "OPEN", Base: "main"},
					Diff:  &stackcmd.Diffstat{Additions: 210, Deletions: 18},
					Children: []*stackcmd.StackNode{
						{
							Piece: "api-rate-limit-cli",
							PR:    &stackcmd.StackNodePR{Number: 7, State: "OPEN", Base: "api-rate-limit", Draft: true},
							Diff:  &stackcmd.Diffstat{Additions: 62, Deletions: 4},
							Drift: []string{"PR #7 base on forge is main, local parent is api-rate-limit"},
						},
					},
				},
				{Piece: "fix-flaky-retry", PR: &stackcmd.StackNodePR{Number: 9, State: "MERGED", Base: "main"}},
				{Piece: "no-pr-yet", Diff: &stackcmd.Diffstat{Additions: 5, Deletions: 1}},
			},
		},
	}

	want := `main
├── ◆ api-rate-limit          in review  #6  +210 -18
│   └── ◆ api-rate-limit-cli  draft      #7  +62  -4
│       ⚠ PR #7 base on forge is main, local parent is api-rate-limit
├── ● fix-flaky-retry         merged ✓   #9
└── ◇ no-pr-yet               no PR          +5   -1
`
	if got := renderStackStatus(result, cli.Painter{}); got != want {
		t.Errorf("renderStackStatus mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderStackStatus_ForgeUnreachable(t *testing.T) {
	result := stackcmd.StackStatusResult{
		MainBranch: "main",
		Tree:       &stackcmd.StackNode{},
	}
	got := renderStackStatus(result, cli.Painter{})
	want := "main\n\n⚠ forge unreachable — PR/MR state not checked\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderStackGraph_Forest(t *testing.T) {
	result := stackcmd.GraphResult{
		Repo:          "jewell-lgtm/mp-demo",
		DefaultBranch: "main",
		Provider:      "github",
		Stacks: []stackgraph.Stack{
			{Root: &stackgraph.StackNode{
				PR: stackgraph.PRRef{Number: 6, Title: "feat: add rate limiter", HeadRef: "api-rate-limit"},
				Children: []*stackgraph.StackNode{
					{PR: stackgraph.PRRef{Number: 7, Title: "feat: wire rate limit into CLI", HeadRef: "api-rate-limit-cli", Draft: true}},
				},
			}},
			{Root: &stackgraph.StackNode{
				PR: stackgraph.PRRef{Number: 9, Title: "fix: deflake retry test", HeadRef: "fix-flaky-retry"},
			}},
		},
	}

	want := `jewell-lgtm/mp-demo  (main, github)
└── #6 feat: add rate limiter  [api-rate-limit]
    └── #7 feat: wire rate limit into CLI (draft)  [api-rate-limit-cli]
└── #9 fix: deflake retry test  [fix-flaky-retry]
`
	if got := renderStackGraph(result); got != want {
		t.Errorf("renderStackGraph mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderStackGraph_Empty(t *testing.T) {
	result := stackcmd.GraphResult{Repo: "o/r", DefaultBranch: "main", Provider: "github"}
	want := "o/r  (main, github)\nno open stacked PRs\n"
	if got := renderStackGraph(result); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
