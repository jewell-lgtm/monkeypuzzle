package stack

import (
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

func prMap(prs ...pr.PRInfo) map[string]pr.PRInfo {
	m := make(map[string]pr.PRInfo, len(prs))
	for _, p := range prs {
		m[p.HeadRefName] = p
	}
	return m
}

func TestReconstructParents_RebuildsFromPRBase(t *testing.T) {
	items := []piece.PieceListItem{
		{Name: "a", Parent: "main"},
		{Name: "b", Parent: "main"}, // local lineage lost; PR says it sits on a
	}
	prs := prMap(
		pr.PRInfo{Number: 1, HeadRefName: "a", BaseRefName: "main", State: "OPEN"},
		pr.PRInfo{Number: 2, HeadRefName: "b", BaseRefName: "a", State: "OPEN"},
	)

	rewrites := reconstructParents(items, prs, "main")
	if len(rewrites) != 1 {
		t.Fatalf("expected 1 rewrite, got %d: %+v", len(rewrites), rewrites)
	}
	if rewrites[0].Piece != "b" || rewrites[0].NewParent != "a" {
		t.Errorf("expected b->a, got %s->%s", rewrites[0].Piece, rewrites[0].NewParent)
	}
}

func TestReconstructParents_SkipsPhantomParent(t *testing.T) {
	items := []piece.PieceListItem{{Name: "b", Parent: "main"}}
	// PR base points at a branch that has no local piece.
	prs := prMap(pr.PRInfo{Number: 2, HeadRefName: "b", BaseRefName: "ghost", State: "OPEN"})

	if rewrites := reconstructParents(items, prs, "main"); len(rewrites) != 0 {
		t.Errorf("expected no rewrites for phantom parent, got %+v", rewrites)
	}
}

func TestReconstructParents_IgnoresClosedAndMerged(t *testing.T) {
	items := []piece.PieceListItem{{Name: "b", Parent: "main"}}
	prs := prMap(pr.PRInfo{Number: 2, HeadRefName: "b", BaseRefName: "a", State: "MERGED"})
	if rewrites := reconstructParents(items, prs, "main"); len(rewrites) != 0 {
		t.Errorf("expected no rewrites for non-open PR, got %+v", rewrites)
	}
}

func TestBuildStackTree_FlagsBaseDrift(t *testing.T) {
	items := []piece.PieceListItem{{Name: "b", Parent: "main"}}
	prs := prMap(pr.PRInfo{Number: 7, HeadRefName: "b", BaseRefName: "a", State: "OPEN", URL: "http://x/7"})

	_, drift := buildStackTree(items, prs, "main")
	if len(drift) != 1 {
		t.Fatalf("expected 1 drift message, got %d: %v", len(drift), drift)
	}
	if !strings.Contains(drift[0], "#7") || !strings.Contains(drift[0], "--apply-bases") {
		t.Errorf("drift message missing expected content: %q", drift[0])
	}
}

func TestBuildStackTree_NoDriftWhenAligned(t *testing.T) {
	items := []piece.PieceListItem{
		{Name: "a", Parent: "main"},
		{Name: "b", Parent: "a"},
	}
	prs := prMap(
		pr.PRInfo{Number: 1, HeadRefName: "a", BaseRefName: "main", State: "OPEN"},
		pr.PRInfo{Number: 2, HeadRefName: "b", BaseRefName: "a", State: "OPEN"},
	)
	tree, drift := buildStackTree(items, prs, "main")
	if len(drift) != 0 {
		t.Errorf("expected no drift, got %v", drift)
	}
	// a hangs off root; b hangs off a.
	if len(tree.Children) != 1 || tree.Children[0].Piece != "a" {
		t.Fatalf("expected root->a, got %+v", tree.Children)
	}
	if len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].Piece != "b" {
		t.Errorf("expected a->b, got %+v", tree.Children[0].Children)
	}
}

func TestBuildStackTree_MergedPRNote(t *testing.T) {
	items := []piece.PieceListItem{{Name: "a", Parent: "main"}}
	prs := prMap(pr.PRInfo{Number: 9, HeadRefName: "a", BaseRefName: "main", State: "MERGED"})
	_, drift := buildStackTree(items, prs, "main")
	if len(drift) != 1 || !strings.Contains(drift[0], "merged") {
		t.Errorf("expected merged note, got %v", drift)
	}
}

func TestBuildStackTree_OrphanParent(t *testing.T) {
	items := []piece.PieceListItem{{Name: "b", Parent: "gone"}}
	tree, drift := buildStackTree(items, prMap(), "main")
	if len(drift) != 1 || !strings.Contains(drift[0], "not found locally") {
		t.Errorf("expected orphan drift, got %v", drift)
	}
	// Orphan still attaches under root so it's visible.
	if len(tree.Children) != 1 || tree.Children[0].Piece != "b" {
		t.Errorf("expected orphan b under root, got %+v", tree.Children)
	}
}

func TestComputeBaseFixes_FindsMismatch(t *testing.T) {
	items := []piece.PieceListItem{{Name: "b", Parent: "a"}}
	prs := prMap(pr.PRInfo{Number: 5, HeadRefName: "b", BaseRefName: "main", State: "OPEN"})

	fixes := computeBaseFixes(items, prs, "main")
	if len(fixes) != 1 || fixes[0].PRNumber != 5 || fixes[0].Base != "a" {
		t.Errorf("expected fix PR#5 base=a, got %+v", fixes)
	}
}

func TestComputeBaseFixes_NoneWhenAligned(t *testing.T) {
	items := []piece.PieceListItem{{Name: "a", Parent: "main"}}
	prs := prMap(pr.PRInfo{Number: 1, HeadRefName: "a", BaseRefName: "main", State: "OPEN"})
	if fixes := computeBaseFixes(items, prs, "main"); len(fixes) != 0 {
		t.Errorf("expected no fixes, got %+v", fixes)
	}
}
