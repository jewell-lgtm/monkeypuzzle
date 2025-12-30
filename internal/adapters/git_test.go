package adapters

import (
	"testing"
)

func TestGit_WorktreeAdd(t *testing.T) {
	tests := []struct {
		name         string
		repoRoot     string
		worktreePath string
		mockOutput   []byte
		mockErr      error
		wantErr      bool
	}{
		{
			name:         "success",
			repoRoot:     "/repo",
			worktreePath: "/worktree",
			mockOutput:   []byte(""),
			mockErr:      nil,
			wantErr:      false,
		},
		{
			name:         "failure",
			repoRoot:     "/repo",
			worktreePath: "/worktree",
			mockOutput:   []byte("fatal: already exists"),
			mockErr:      MockError("exit status 1"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"worktree", "add", tt.worktreePath}, tt.mockOutput, tt.mockErr)

			git := NewGit(exec)
			err := git.WorktreeAdd(tt.repoRoot, tt.worktreePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("WorktreeAdd() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGit_CurrentBranch(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput []byte
		mockErr    error
		want       string
		wantErr    bool
	}{
		{
			name:       "returns branch name",
			mockOutput: []byte("main\n"),
			mockErr:    nil,
			want:       "main",
			wantErr:    false,
		},
		{
			name:       "trims whitespace",
			mockOutput: []byte("  feature-branch  \n"),
			mockErr:    nil,
			want:       "feature-branch",
			wantErr:    false,
		},
		{
			name:       "handles error",
			mockOutput: []byte(""),
			mockErr:    MockError("not a git repo"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, tt.mockOutput, tt.mockErr)

			git := NewGit(exec)
			got, err := git.CurrentBranch("/repo")

			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentBranch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CurrentBranch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGit_IsMainAhead(t *testing.T) {
	tests := []struct {
		name        string
		mainBranch  string
		pieceBranch string
		mergeBase   string
		commitCount string
		want        bool
		wantErr     bool
	}{
		{
			name:        "main not ahead",
			mainBranch:  "main",
			pieceBranch: "feature",
			mergeBase:   "abc123",
			commitCount: "0",
			want:        false,
		},
		{
			name:        "main ahead by 1",
			mainBranch:  "main",
			pieceBranch: "feature",
			mergeBase:   "abc123",
			commitCount: "1",
			want:        true,
		},
		{
			name:        "main ahead by 5",
			mainBranch:  "main",
			pieceBranch: "feature",
			mergeBase:   "abc123",
			commitCount: "5",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"merge-base", tt.mainBranch, tt.pieceBranch}, []byte(tt.mergeBase+"\n"), nil)
			exec.AddResponse("git", []string{"rev-list", "--count", tt.mergeBase + ".." + tt.mainBranch}, []byte(tt.commitCount+"\n"), nil)

			git := NewGit(exec)
			got, err := git.IsMainAhead("/repo", tt.mainBranch, tt.pieceBranch)

			if (err != nil) != tt.wantErr {
				t.Errorf("IsMainAhead() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsMainAhead() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGit_IsBranchMerged(t *testing.T) {
	tests := []struct {
		name       string
		mainBranch string
		branchName string
		mockOutput []byte
		want       bool
	}{
		{
			name:       "branch is merged",
			mainBranch: "main",
			branchName: "feature",
			mockOutput: []byte("  feature\n* main\n  other\n"),
			want:       true,
		},
		{
			name:       "branch not merged",
			mainBranch: "main",
			branchName: "feature",
			mockOutput: []byte("* main\n  other\n"),
			want:       false,
		},
		{
			name:       "handles current branch marker",
			mainBranch: "main",
			branchName: "feature",
			mockOutput: []byte("* feature\n  main\n"),
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"branch", "--merged", tt.mainBranch}, tt.mockOutput, nil)

			git := NewGit(exec)
			got, err := git.IsBranchMerged("/repo", tt.mainBranch, tt.branchName)

			if err != nil {
				t.Errorf("IsBranchMerged() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("IsBranchMerged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGit_GetCommitMessages(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput []byte
		want       []string
	}{
		{
			name:       "multiple commits",
			mockOutput: []byte("feat: add feature\nfix: bug fix\nchore: cleanup\n"),
			want:       []string{"feat: add feature", "fix: bug fix", "chore: cleanup"},
		},
		{
			name:       "single commit",
			mockOutput: []byte("feat: add feature\n"),
			want:       []string{"feat: add feature"},
		},
		{
			name:       "empty",
			mockOutput: []byte(""),
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"log", "--format=%s", "main..feature"}, tt.mockOutput, nil)

			git := NewGit(exec)
			got, err := git.GetCommitMessages("/repo", "main", "feature")

			if err != nil {
				t.Errorf("GetCommitMessages() error = %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("GetCommitMessages() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetCommitMessages()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGit_IsWorktree(t *testing.T) {
	tests := []struct {
		name   string
		gitDir string
		want   bool
	}{
		{
			name:   "worktree path",
			gitDir: "/repo/.git/worktrees/my-piece",
			want:   true,
		},
		{
			name:   "main repo",
			gitDir: "/repo/.git",
			want:   false,
		},
		{
			name:   "contains worktrees",
			gitDir: "/some/path/with/worktrees/in/it",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := NewGit(NewMockExec())
			got := git.IsWorktree(tt.gitDir)
			if got != tt.want {
				t.Errorf("IsWorktree(%q) = %v, want %v", tt.gitDir, got, tt.want)
			}
		})
	}
}

func TestGit_BranchExistsOnRemote(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		mockOutput []byte
		want       bool
	}{
		{
			name:       "exists",
			branch:     "feature",
			mockOutput: []byte("abc123\trefs/heads/feature\n"),
			want:       true,
		},
		{
			name:       "not exists",
			branch:     "feature",
			mockOutput: []byte(""),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"ls-remote", "--heads", "origin", tt.branch}, tt.mockOutput, nil)

			git := NewGit(exec)
			got, err := git.BranchExistsOnRemote("/repo", tt.branch)

			if err != nil {
				t.Errorf("BranchExistsOnRemote() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("BranchExistsOnRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}
