package main

import (
	"testing"

	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
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
					Children: []*stackcmd.StackNode{
						{
							Piece: "api-rate-limit-cli",
							PR:    &stackcmd.StackNodePR{Number: 7, State: "OPEN", Base: "api-rate-limit"},
							Drift: []string{"PR #7 base on forge is main, local parent is api-rate-limit"},
						},
					},
				},
				{Piece: "fix-flaky-retry", PR: &stackcmd.StackNodePR{Number: 9, State: "OPEN", Base: "main"}},
				{Piece: "no-pr-yet"},
			},
		},
	}

	want := `main
├── api-rate-limit  PR #6 OPEN → main
│   └── api-rate-limit-cli  PR #7 OPEN → api-rate-limit
│       ! PR #7 base on forge is main, local parent is api-rate-limit
├── fix-flaky-retry  PR #9 OPEN → main
└── no-pr-yet  (no PR)
`
	if got := renderStackStatus(result); got != want {
		t.Errorf("renderStackStatus mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderStackStatus_ForgeUnreachable(t *testing.T) {
	result := stackcmd.StackStatusResult{
		MainBranch: "main",
		Tree:       &stackcmd.StackNode{},
	}
	got := renderStackStatus(result)
	want := "main\n\n! forge unreachable — PR/MR state not checked\n"
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
