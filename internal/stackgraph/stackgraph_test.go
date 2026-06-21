package stackgraph

import (
	"reflect"
	"testing"
)

// pr is a terse constructor for an OPEN PRRef (the common case in tests).
func pr(number int, head, base string) PRRef {
	return PRRef{Number: number, HeadRef: head, BaseRef: base, State: StateOpen}
}

// forest flattens BuildStacks output into a comparable shape: the ordered list
// of root PR numbers, and, for every node, its ordered child PR numbers.
type forest struct {
	roots    []int
	children map[int][]int
}

func collect(stacks []Stack) forest {
	f := forest{children: map[int][]int{}}
	var walk func(n *StackNode)
	walk = func(n *StackNode) {
		kids := []int{}
		for _, c := range n.Children {
			kids = append(kids, c.PR.Number)
			walk(c)
		}
		f.children[n.PR.Number] = kids
	}
	for _, s := range stacks {
		f.roots = append(f.roots, s.Root.PR.Number)
		walk(s.Root)
	}
	return f
}

func TestBuildStacks(t *testing.T) {
	const main = "main"

	tests := []struct {
		name     string
		prs      []PRRef
		def      string
		wantRoot []int
		wantKids map[int][]int
	}{
		{
			name:     "empty",
			prs:      nil,
			def:      main,
			wantRoot: nil,
			wantKids: map[int][]int{},
		},
		{
			name: "single root linear chain",
			prs: []PRRef{
				pr(1, "feat-a", "main"),
				pr(2, "feat-b", "feat-a"),
				pr(3, "feat-c", "feat-b"),
			},
			def:      main,
			wantRoot: []int{1},
			wantKids: map[int][]int{1: {2}, 2: {3}, 3: {}},
		},
		{
			name: "single root branching",
			prs: []PRRef{
				pr(1, "feat-a", "main"),
				pr(2, "feat-b", "feat-a"),
				pr(3, "feat-c", "feat-a"),
			},
			def:      main,
			wantRoot: []int{1},
			wantKids: map[int][]int{1: {2, 3}, 2: {}, 3: {}},
		},
		{
			name: "multiple independent roots",
			prs: []PRRef{
				pr(1, "feat-a", "main"),
				pr(2, "feat-b", "main"),
			},
			def:      main,
			wantRoot: []int{1, 2},
			wantKids: map[int][]int{1: {}, 2: {}},
		},
		{
			name: "orphan whose base is absent surfaces as root",
			prs: []PRRef{
				pr(1, "feat-a", "long-lived-branch"), // base not default, not an open head
				pr(2, "feat-b", "feat-a"),
			},
			def:      main,
			wantRoot: []int{1},
			wantKids: map[int][]int{1: {2}, 2: {}},
		},
		{
			name: "merged and closed PRs are filtered out",
			prs: []PRRef{
				pr(1, "feat-a", "main"),
				{Number: 2, HeadRef: "feat-b", BaseRef: "feat-a", State: "MERGED"},
				{Number: 3, HeadRef: "feat-c", BaseRef: "feat-a", State: "CLOSED"},
			},
			def:      main,
			wantRoot: []int{1},
			wantKids: map[int][]int{1: {}},
		},
		{
			name: "child of a merged base becomes an orphan root",
			prs: []PRRef{
				// #1 merged & filtered out; #2 based on its head -> orphan root.
				{Number: 1, HeadRef: "feat-a", BaseRef: "main", State: "MERGED"},
				pr(2, "feat-b", "feat-a"),
			},
			def:      main,
			wantRoot: []int{2},
			wantKids: map[int][]int{2: {}},
		},
		{
			name: "duplicate head branch keeps lowest number",
			prs: []PRRef{
				pr(5, "feat", "main"),
				pr(9, "feat", "main"), // same head -> dropped
				pr(7, "child", "feat"),
			},
			def:      main,
			wantRoot: []int{5},
			wantKids: map[int][]int{5: {7}, 7: {}},
		},
		{
			name: "two-node cycle terminates and surfaces both",
			prs: []PRRef{
				pr(1, "a", "b"), // a based on b
				pr(2, "b", "a"), // b based on a -> cycle, no default anchor
			},
			def:      main,
			wantRoot: []int{1},
			wantKids: map[int][]int{1: {2}, 2: {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collect(BuildStacks(tt.prs, tt.def))
			if !reflect.DeepEqual(got.roots, tt.wantRoot) {
				t.Errorf("roots = %v, want %v", got.roots, tt.wantRoot)
			}
			if !reflect.DeepEqual(got.children, tt.wantKids) {
				t.Errorf("children = %v, want %v", got.children, tt.wantKids)
			}
		})
	}
}

// Input order must not affect output: a shuffled chain yields the same forest.
func TestBuildStacks_DeterministicRegardlessOfInputOrder(t *testing.T) {
	ordered := []PRRef{
		pr(1, "feat-a", "main"),
		pr(2, "feat-b", "feat-a"),
		pr(3, "feat-c", "feat-b"),
		pr(4, "feat-d", "main"),
	}
	shuffled := []PRRef{ordered[3], ordered[1], ordered[0], ordered[2]}

	want := collect(BuildStacks(ordered, "main"))
	got := collect(BuildStacks(shuffled, "main"))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shuffled input changed output:\n got=%+v\nwant=%+v", got, want)
	}
}

// BuildStacks must not mutate the caller's slice.
func TestBuildStacks_DoesNotMutateInput(t *testing.T) {
	in := []PRRef{
		pr(3, "feat-c", "feat-b"),
		pr(1, "feat-a", "main"),
		pr(2, "feat-b", "feat-a"),
	}
	snapshot := append([]PRRef(nil), in...)
	_ = BuildStacks(in, "main")
	if !reflect.DeepEqual(in, snapshot) {
		t.Errorf("input was mutated:\n got=%+v\nwant=%+v", in, snapshot)
	}
}
