package main

import (
	"testing"

	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
)

func TestRemoteBranchShortName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"simple remote ref", "origin/foo", "foo"},
		{"nested branch keeps slashes", "origin/feature/x", "feature/x"},
		{"other remote", "upstream/bar", "bar"},
		{"no slash", "origin", ""},
		{"trailing slash", "origin/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remoteBranchShortName(tt.ref); got != tt.want {
				t.Errorf("remoteBranchShortName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestMatchCurrent(t *testing.T) {
	projects := []dashProject{
		{
			Info: projectcmd.Info{Name: "alpha", Path: "/repos/alpha"},
			Pieces: []dashPiece{
				{Name: "fix-login", WorktreePath: "/wt/alpha/fix-login"},
				// A piece worktree nested under the project root: the piece
				// must win over the project for paths inside it.
				{Name: "nested", WorktreePath: "/repos/alpha/.monkeypuzzle/pieces/nested"},
			},
		},
		{Info: projectcmd.Info{Name: "beta", Path: "/repos/beta"}},
		{
			Info:   projectcmd.Info{Name: "remote", Path: "/repos/remote", Host: "devbox"},
			Pieces: []dashPiece{{Name: "far", WorktreePath: "/repos/remote/far"}},
		},
	}

	tests := []struct {
		name        string
		wd          string
		wantProject string
		wantPiece   string
	}{
		{"piece worktree root", "/wt/alpha/fix-login", "alpha", "fix-login"},
		{"subdir of piece worktree", "/wt/alpha/fix-login/src/deep", "alpha", "fix-login"},
		{"project root", "/repos/beta", "beta", ""},
		{"subdir of project root", "/repos/beta/apps/web", "beta", ""},
		{"nested piece beats its project", "/repos/alpha/.monkeypuzzle/pieces/nested/src", "alpha", "nested"},
		{"outside everything", "/somewhere/else", "", ""},
		{"prefix but not a path boundary", "/repos/alphaville", "", ""},
		{"remote projects never match", "/repos/remote/far", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchCurrent(tt.wd, projects)
			if tt.wantProject == "" {
				if got != nil {
					t.Fatalf("matchCurrent(%q) = %+v, want nil", tt.wd, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("matchCurrent(%q) = nil, want %s/%s", tt.wd, tt.wantProject, tt.wantPiece)
			}
			if got.Project != tt.wantProject || got.Piece != tt.wantPiece {
				t.Errorf("matchCurrent(%q) = %s/%s, want %s/%s", tt.wd, got.Project, got.Piece, tt.wantProject, tt.wantPiece)
			}
		})
	}
}
