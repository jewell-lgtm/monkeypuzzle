package piece_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// flattenTree renders a tree as "name(parentPath)" lines for easy assertion.
func flattenTree(node *piece.TreeNode, path string, out *[]string) {
	name := "main"
	if node.IsOrphan {
		name = "orphan"
	} else if node.Piece != nil {
		name = node.Piece.Name
	}
	if path != "" {
		*out = append(*out, fmt.Sprintf("%s/%s", path, name))
	} else {
		*out = append(*out, name)
	}
	childPath := name
	if path != "" {
		childPath = path + "/" + name
	}
	for _, child := range node.Children {
		flattenTree(child, childPath, out)
	}
}

func treeLines(root *piece.TreeNode) []string {
	var out []string
	flattenTree(root, "", &out)
	return out
}

func assertTree(t *testing.T, got []string, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tree mismatch\ngot:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// Regression: pieces arrive sorted newest-first (ListPieces mod-time sort), so
// a stacked child is usually processed BEFORE its parent. The old
// findOrCreateNode-based builder created the parent chain on demand and then
// appended fresh duplicate nodes when the loop reached those pieces itself.
func TestBuildPieceTree_ChildBeforeParent_NoDuplicates(t *testing.T) {
	// Newest-first: docs -> cli -> base (a 3-deep stack), exactly the order
	// ListPieces yields right after `mp create` + 2x `mp stack append`.
	pieces := []piece.PieceListItem{
		{Name: "api-rate-limit-docs", Parent: "api-rate-limit-cli"},
		{Name: "api-rate-limit-cli", Parent: "api-rate-limit"},
		{Name: "api-rate-limit", Parent: "main"},
	}

	got := treeLines(piece.BuildPieceTree(pieces))
	want := []string{
		"main",
		"main/api-rate-limit",
		"main/api-rate-limit/api-rate-limit-cli",
		"main/api-rate-limit/api-rate-limit-cli/api-rate-limit-docs",
	}
	assertTree(t, got, want)
}

func TestBuildPieceTree_MixedStackAndIndependents(t *testing.T) {
	// The demo-repo shape: one 3-deep stack + 2 independents, newest-first.
	pieces := []piece.PieceListItem{
		{Name: "refactor-config", Parent: "main"},
		{Name: "fix-flaky-retry", Parent: "main"},
		{Name: "api-rate-limit-docs", Parent: "api-rate-limit-cli"},
		{Name: "api-rate-limit-cli", Parent: "api-rate-limit"},
		{Name: "api-rate-limit", Parent: "main"},
	}

	got := treeLines(piece.BuildPieceTree(pieces))
	want := []string{
		"main",
		"main/refactor-config",
		"main/fix-flaky-retry",
		"main/api-rate-limit",
		"main/api-rate-limit/api-rate-limit-cli",
		"main/api-rate-limit/api-rate-limit-cli/api-rate-limit-docs",
	}
	assertTree(t, got, want)
}

func TestBuildPieceTree_OrphansGrouped(t *testing.T) {
	pieces := []piece.PieceListItem{
		{Name: "kid", Parent: "gone-parent"},
		{Name: "base", Parent: "main"},
	}

	got := treeLines(piece.BuildPieceTree(pieces))
	want := []string{
		"main",
		"main/orphan",
		"main/orphan/kid",
		"main/base",
	}
	assertTree(t, got, want)
}

func TestBuildPieceTree_Empty(t *testing.T) {
	root := piece.BuildPieceTree(nil)
	if len(root.Children) != 0 {
		t.Errorf("expected empty tree, got %d children", len(root.Children))
	}
}
