// Package stackgraph builds a forest of stacked pull requests purely from
// GitHub PR base-ref edges, with no dependency on local git worktrees.
//
// A "stack" is the tree formed when one PR's base branch is another PR's head
// branch. The root of a stack is a PR whose base is the repository's default
// branch. The only evidence of a stack is the PR base->head relationship, so
// this package reconstructs stacks from the set of open PRs alone — it is the
// shared, pure core used by both the server's read path and (later) the CLI.
package stackgraph

import "sort"

// PRRef is the minimal pull-request shape needed to reconstruct a stack. It
// mirrors internal/adapters.PRInfo (Number/HeadRef/BaseRef/State/URL) and
// extends it with Title/Author/Draft, which the GitHub API exposes and the UI
// wants. State is one of OPEN, MERGED, CLOSED.
type PRRef struct {
	Number  int    `json:"number"`
	HeadRef string `json:"head_ref"`
	BaseRef string `json:"base_ref"`
	Title   string `json:"title"`
	State   string `json:"state"`
	URL     string `json:"url"`
	Author  string `json:"author"`
	Draft   bool   `json:"draft"`
}

// StateOpen is the PRRef.State value for an open pull request. Only open PRs
// form live stack edges.
const StateOpen = "OPEN"

// StackNode is one PR in the forest together with the PRs stacked on top of it
// (those whose base branch is this PR's head branch).
type StackNode struct {
	PR       PRRef        `json:"pr"`
	Children []*StackNode `json:"children,omitempty"`
}

// Stack is a single root PR (based on the default branch, or an orphan) and its
// descendant forest.
type Stack struct {
	Root *StackNode `json:"root"`
}

// BuildStacks reconstructs the forest of stacked PRs from prs, treating
// defaultBranch as the trunk every stack roots on.
//
// Only OPEN PRs participate (merged/closed PRs no longer form live edges). A PR
// whose base is defaultBranch is a root; a PR whose base is another open PR's
// head is that PR's child; a PR whose base is neither (an orphan — e.g. its
// base PR was merged) surfaces as its own root rather than being dropped.
// Duplicate head branches are deduplicated keeping the lowest PR number. The
// result is deterministic: nodes are ordered by PR number throughout, and any
// PRs trapped in a base->head cycle still terminate and surface (the lowest
// number in the cycle becomes the root, the back-edge is broken).
func BuildStacks(prs []PRRef, defaultBranch string) []Stack {
	nodes := openDeduped(prs)
	n := len(nodes)
	if n == 0 {
		return nil
	}

	// Index each kept PR by its head branch so base refs can resolve to parents.
	byHead := make(map[string]int, n)
	for i, pr := range nodes {
		byHead[pr.HeadRef] = i
	}

	// Resolve each node's parent: -1 means a root (base is the default branch,
	// or an orphan whose base isn't an open PR head).
	parent := make([]int, n)
	children := make([][]int, n)
	for i, pr := range nodes {
		if pr.BaseRef == defaultBranch {
			parent[i] = -1
			continue
		}
		if j, ok := byHead[pr.BaseRef]; ok && j != i {
			parent[i] = j
			continue
		}
		parent[i] = -1 // orphan -> root
	}
	for i := range nodes {
		if p := parent[i]; p >= 0 {
			children[p] = append(children[p], i)
		}
	}

	visited := make([]bool, n)
	var build func(i int) *StackNode
	build = func(i int) *StackNode {
		visited[i] = true
		node := &StackNode{PR: nodes[i]}
		for _, c := range children[i] {
			if !visited[c] { // break back-edges defensively
				node.Children = append(node.Children, build(c))
			}
		}
		return node
	}

	var stacks []Stack
	// Real roots first (lowest number first, since nodes is number-sorted).
	for i := range nodes {
		if parent[i] == -1 && !visited[i] {
			stacks = append(stacks, Stack{Root: build(i)})
		}
	}
	// Anything still unvisited is trapped in a pure cycle (no default-branch
	// anchor); emit it so PRs are never silently dropped.
	for i := range nodes {
		if !visited[i] {
			stacks = append(stacks, Stack{Root: build(i)})
		}
	}
	return stacks
}

// openDeduped returns the OPEN PRs sorted by number ascending, with at most one
// PR per head branch (the lowest number wins). The input slice is not mutated.
func openDeduped(prs []PRRef) []PRRef {
	open := make([]PRRef, 0, len(prs))
	for _, pr := range prs {
		if pr.State == StateOpen {
			open = append(open, pr)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })

	seen := make(map[string]struct{}, len(open))
	deduped := open[:0]
	for _, pr := range open {
		if _, dup := seen[pr.HeadRef]; dup {
			continue
		}
		seen[pr.HeadRef] = struct{}{}
		deduped = append(deduped, pr)
	}
	return deduped
}
