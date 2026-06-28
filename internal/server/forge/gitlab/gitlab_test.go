package gitlab

import (
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

func TestMapState(t *testing.T) {
	cases := map[string]string{
		"opened":  stackgraph.StateOpen,
		"merged":  "MERGED",
		"closed":  "CLOSED",
		"locked":  "CLOSED",
		"unknown": stackgraph.StateOpen, // forward-compatible default
	}
	for in, want := range cases {
		if got := mapState(in); got != want {
			t.Errorf("mapState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapMR(t *testing.T) {
	mr := &gitlab.BasicMergeRequest{
		IID:          7,
		SourceBranch: "feat-b",
		TargetBranch: "feat-a",
		Title:        "Add b",
		State:        "opened",
		WebURL:       "https://gitlab.com/o/r/-/merge_requests/7",
		Draft:        true,
		Author:       &gitlab.BasicUser{Username: "zazzy"},
	}
	got := mapMR(mr)
	want := stackgraph.PRRef{
		Number:  7,
		HeadRef: "feat-b",
		BaseRef: "feat-a",
		Title:   "Add b",
		State:   stackgraph.StateOpen,
		URL:     "https://gitlab.com/o/r/-/merge_requests/7",
		Author:  "zazzy",
		Draft:   true,
	}
	if got != want {
		t.Fatalf("mapMR mismatch:\n got=%+v\nwant=%+v", got, want)
	}

	// Nil author is tolerated (returns empty author, no panic).
	if a := mapMR(&gitlab.BasicMergeRequest{IID: 1}).Author; a != "" {
		t.Fatalf("nil author should yield empty string, got %q", a)
	}
}

func TestMapProject_Visibility(t *testing.T) {
	pub := mapProject(&gitlab.Project{
		ID: 1, Path: "r", Visibility: gitlab.PublicVisibility,
		Namespace: &gitlab.ProjectNamespace{Path: "o"},
	})
	if pub.Private {
		t.Errorf("public project mapped as private")
	}
	if pub.Owner != "o" || pub.Name != "r" || pub.ForgeRepoID != 1 {
		t.Errorf("unexpected mapping: %+v", pub)
	}

	priv := mapProject(&gitlab.Project{ID: 2, Path: "r", Visibility: gitlab.PrivateVisibility})
	if !priv.Private {
		t.Errorf("private project mapped as public")
	}
}
