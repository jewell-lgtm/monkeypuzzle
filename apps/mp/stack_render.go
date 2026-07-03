package main

import (
	"fmt"
	"strings"

	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// renderStackStatus renders a StackStatusResult as the human tree shown on a
// terminal. JSON stays the contract for pipes/agents; this is presentation only.
//
//	main
//	├── api-rate-limit  PR #6 OPEN → main
//	│   └── api-rate-limit-cli  PR #7 OPEN → api-rate-limit
//	└── fix-flaky-retry  PR #9 OPEN → main
func renderStackStatus(result stackcmd.StackStatusResult) string {
	var b strings.Builder
	b.WriteString(result.MainBranch + "\n")
	if result.Tree != nil {
		renderStatusNodes(&b, result.Tree.Children, "")
	}
	if !result.ForgeChecked {
		b.WriteString("\n! forge unreachable — PR/MR state not checked\n")
	}
	if len(result.Reconstructed) > 0 {
		b.WriteString(fmt.Sprintf("\nreconstructed from forge: %s\n", strings.Join(result.Reconstructed, ", ")))
	}
	if len(result.Applied) > 0 {
		b.WriteString(fmt.Sprintf("bases updated on forge: %s\n", strings.Join(result.Applied, ", ")))
	}
	return b.String()
}

func renderStatusNodes(b *strings.Builder, nodes []*stackcmd.StackNode, prefix string) {
	for i, node := range nodes {
		connector, childPrefix := treeBranch(prefix, i == len(nodes)-1)
		b.WriteString(prefix + connector + statusNodeLabel(node) + "\n")
		for _, d := range node.Drift {
			b.WriteString(childPrefix + "! " + d + "\n")
		}
		renderStatusNodes(b, node.Children, childPrefix)
	}
}

func statusNodeLabel(node *stackcmd.StackNode) string {
	if node.PR == nil {
		return node.Piece + "  (no PR)"
	}
	return fmt.Sprintf("%s  PR #%d %s → %s", node.Piece, node.PR.Number, node.PR.State, node.PR.Base)
}

// renderStackGraph renders a GraphResult (the no-clone forge forest) for a
// terminal, one tree per stack:
//
//	jewell-lgtm/mp-demo  (main, github)
//	└── #6 feat: add rate limiter  [api-rate-limit]
//	    └── #7 feat: wire rate limit into CLI  [api-rate-limit-cli]
func renderStackGraph(result stackcmd.GraphResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s  (%s, %s)\n", result.Repo, result.DefaultBranch, result.Provider))
	if len(result.Stacks) == 0 {
		b.WriteString("no open stacked PRs\n")
		return b.String()
	}
	for _, s := range result.Stacks {
		if s.Root == nil {
			continue
		}
		renderGraphNodes(&b, []*stackgraph.StackNode{s.Root}, "")
	}
	return b.String()
}

func renderGraphNodes(b *strings.Builder, nodes []*stackgraph.StackNode, prefix string) {
	for i, node := range nodes {
		connector, childPrefix := treeBranch(prefix, i == len(nodes)-1)
		draft := ""
		if node.PR.Draft {
			draft = " (draft)"
		}
		b.WriteString(fmt.Sprintf("%s%s#%d %s%s  [%s]\n", prefix, connector, node.PR.Number, node.PR.Title, draft, node.PR.HeadRef))
		renderGraphNodes(b, node.Children, childPrefix)
	}
}

// treeBranch returns the connector for a node and the prefix its children
// inherit, matching the box-drawing style of `mp list`.
func treeBranch(prefix string, last bool) (connector, childPrefix string) {
	if last {
		return "└── ", prefix + "    "
	}
	return "├── ", prefix + "│   "
}
