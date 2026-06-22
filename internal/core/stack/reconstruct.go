package stack

import (
	"fmt"
	"sort"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

// StackNodePR is the GitHub PR associated with a piece, if any.
type StackNodePR struct {
	Number int    `json:"number"`
	State  string `json:"state"` // OPEN, MERGED, CLOSED
	URL    string `json:"url"`
	Base   string `json:"base"` // PR base branch as seen on GitHub
}

// StackNode is one piece in the rendered stack tree.
type StackNode struct {
	Piece    string       `json:"piece"`
	Parent   string       `json:"parent"`
	PR       *StackNodePR `json:"pr,omitempty"`
	Drift    []string     `json:"drift,omitempty"`
	Children []*StackNode `json:"children,omitempty"`
}

// StackStatusResult is the output of `mp stack status`.
type StackStatusResult struct {
	MainBranch    string     `json:"main_branch"`
	Tree          *StackNode `json:"tree"`
	GitHubChecked bool       `json:"github_checked"`
	Reconstructed []string   `json:"reconstructed,omitempty"` // pieces whose parent was rewritten by --from-github
	Applied       []string   `json:"applied,omitempty"`       // PRs whose base was edited by --apply-bases
	Drift         []string   `json:"drift,omitempty"`         // flat list of all drift messages
}

// SyncResult is the output of `mp stack sync`.
type SyncResult struct {
	MainBranch string   `json:"main_branch"`
	Strategy   string   `json:"strategy"`
	Updated    []string `json:"updated"`
	Pushed     []string `json:"pushed,omitempty"`
	Skipped    []string `json:"skipped,omitempty"`
	Status     string   `json:"status"` // synced | dry-run | aborted | blocked
}

// localParentBranch maps a piece's stored parent ("main" sentinel or a piece
// name) to the actual git branch it should sit on top of.
func localParentBranch(parent, mainBranch string) string {
	if parent == "main" || parent == "" {
		return mainBranch
	}
	return parent
}

// driftBaseMessage is fallback #6: a PR base on GitHub disagrees with local lineage.
func driftBaseMessage(prNumber int, prURL, ghBase, localParent string) string {
	return fmt.Sprintf(
		"PR #%d base on GitHub is %q but this piece's local parent is %q. "+
			"Fix on GitHub: open %s, click Edit by the title, set base to %q — "+
			"or run 'mp stack status --apply-bases'. To accept GitHub's lineage locally instead, run 'mp stack status --from-github'.",
		prNumber, ghBase, localParent, prURL, localParent)
}

// driftMergedMessage notes a merged PR whose piece still exists locally.
func driftMergedMessage(prNumber int, pieceName string) string {
	return fmt.Sprintf("PR #%d is merged — run 'mp done' to clean up %q.", prNumber, pieceName)
}

// driftOrphanMessage notes a piece whose parent doesn't exist locally.
func driftOrphanMessage(parent string) string {
	return fmt.Sprintf("parent piece %q not found locally (run 'mp stack status --from-github' if it lives on GitHub)", parent)
}

// buildStackTree builds the rendered tree from local pieces, attaching PR info and
// drift annotations. Returns the synthetic root (representing main) and a flat list
// of every drift message found.
func buildStackTree(items []piece.PieceListItem, prByHead map[string]pr.PRInfo, mainBranch string) (*StackNode, []string) {
	// Stable order for deterministic output.
	sorted := append([]piece.PieceListItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	nodes := make(map[string]*StackNode, len(sorted))
	for i := range sorted {
		it := sorted[i]
		n := &StackNode{Piece: it.Name, Parent: it.Parent}
		if info, ok := prByHead[it.Name]; ok {
			n.PR = &StackNodePR{Number: info.Number, State: info.State, URL: info.URL, Base: info.BaseRefName}
		}
		nodes[it.Name] = n
	}

	root := &StackNode{Piece: "", Parent: ""}
	var allDrift []string

	for i := range sorted {
		it := sorted[i]
		n := nodes[it.Name]

		// Drift detection against the PR.
		if n.PR != nil {
			parentBranch := localParentBranch(it.Parent, mainBranch)
			switch n.PR.State {
			case "OPEN":
				if n.PR.Base != parentBranch {
					msg := driftBaseMessage(n.PR.Number, n.PR.URL, n.PR.Base, parentBranch)
					n.Drift = append(n.Drift, msg)
					allDrift = append(allDrift, msg)
				}
			case "MERGED":
				msg := driftMergedMessage(n.PR.Number, it.Name)
				n.Drift = append(n.Drift, msg)
				allDrift = append(allDrift, msg)
			}
		}

		// Attach to parent.
		switch {
		case it.Parent == "main" || it.Parent == "":
			root.Children = append(root.Children, n)
		case nodes[it.Parent] != nil:
			nodes[it.Parent].Children = append(nodes[it.Parent].Children, n)
		default:
			msg := driftOrphanMessage(it.Parent)
			n.Drift = append(n.Drift, msg)
			allDrift = append(allDrift, msg)
			root.Children = append(root.Children, n)
		}
	}

	return root, allDrift
}

// parentRewrite is a single lineage change reconstructed from GitHub.
type parentRewrite struct {
	Piece     string
	NewParent string // "main" sentinel or a parent piece name
}

// reconstructParents computes the local parent rewrites implied by open PR bases.
// For each piece with an OPEN PR whose base differs from its current local parent,
// it proposes setting the parent to the PR base (mapped to the "main" sentinel when
// the base is the main branch). Pure: callers persist the rewrites.
func reconstructParents(items []piece.PieceListItem, prByHead map[string]pr.PRInfo, mainBranch string) []parentRewrite {
	existing := make(map[string]bool, len(items))
	for _, it := range items {
		existing[it.Name] = true
	}

	var rewrites []parentRewrite
	for _, it := range items {
		info, ok := prByHead[it.Name]
		if !ok || info.State != "OPEN" {
			continue
		}
		newParent := info.BaseRefName
		if newParent == mainBranch {
			newParent = "main"
		}
		// Only adopt a parent that exists locally (or the main sentinel); otherwise
		// the lineage would point at a phantom piece.
		if newParent != "main" && !existing[newParent] {
			continue
		}
		if newParent != it.Parent {
			rewrites = append(rewrites, parentRewrite{Piece: it.Name, NewParent: newParent})
		}
	}
	sort.Slice(rewrites, func(i, j int) bool { return rewrites[i].Piece < rewrites[j].Piece })
	return rewrites
}

// baseFix is a single PR base edit implied by local lineage (the --apply-bases path).
type baseFix struct {
	PRNumber int
	Base     string
}

// computeBaseFixes finds open PRs whose base on GitHub disagrees with local lineage.
func computeBaseFixes(items []piece.PieceListItem, prByHead map[string]pr.PRInfo, mainBranch string) []baseFix {
	var fixes []baseFix
	for _, it := range items {
		info, ok := prByHead[it.Name]
		if !ok || info.State != "OPEN" {
			continue
		}
		want := localParentBranch(it.Parent, mainBranch)
		if info.BaseRefName != want {
			fixes = append(fixes, baseFix{PRNumber: info.Number, Base: want})
		}
	}
	sort.Slice(fixes, func(i, j int) bool { return fixes[i].PRNumber < fixes[j].PRNumber })
	return fixes
}
