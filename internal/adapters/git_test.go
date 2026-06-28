package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const submoduleRemoveErr = "fatal: working trees containing submodules cannot be moved or removed"

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
			err := git.WorktreeAdd(context.Background(), tt.repoRoot, tt.worktreePath)

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
			got, err := git.CurrentBranch(context.Background(), "/repo")

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
			got, err := git.IsMainAhead(context.Background(), "/repo", tt.mainBranch, tt.pieceBranch)

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
			got, err := git.IsBranchMerged(context.Background(), "/repo", tt.mainBranch, tt.branchName)

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

func TestGit_IsBranchSquashMerged(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput []byte
		want       bool
	}{
		{
			name:       "all commits applied by patch-id (squash-merged)",
			mockOutput: []byte("- abc123\n- def456\n"),
			want:       true,
		},
		{
			name:       "no unique commits (empty piece)",
			mockOutput: []byte(""),
			want:       true,
		},
		{
			name:       "has unique commit (not merged)",
			mockOutput: []byte("+ abc123\n"),
			want:       false,
		},
		{
			name:       "mix of applied and unique commits (not fully merged)",
			mockOutput: []byte("- abc123\n+ def456\n"),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"cherry", "main", "feature"}, tt.mockOutput, nil)

			git := NewGit(exec)
			got, err := git.IsBranchSquashMerged(context.Background(), "/repo", "main", "feature")

			if err != nil {
				t.Errorf("IsBranchSquashMerged() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("IsBranchSquashMerged() = %v, want %v", got, tt.want)
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
			got, err := git.GetCommitMessages(context.Background(), "/repo", "main", "feature")

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
		mockErr    error
		remoteOut  []byte // output of `git remote`, consulted only on ls-remote error
		want       bool
		wantErr    bool
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
		{
			// ls-remote fails but origin is configured -> a real error to surface.
			name:      "ls-remote error with origin",
			branch:    "feature",
			mockErr:   errors.New("exit status 128"),
			remoteOut: []byte("origin\n"),
			want:      false,
			wantErr:   true,
		},
		{
			// ls-remote fails because there is no origin remote -> benign.
			name:      "ls-remote error no origin",
			branch:    "feature",
			mockErr:   errors.New("exit status 128"),
			remoteOut: []byte(""),
			want:      false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"ls-remote", "--heads", "origin", tt.branch}, tt.mockOutput, tt.mockErr)
			exec.AddResponse("git", []string{"remote"}, tt.remoteOut, nil)

			git := NewGit(exec)
			got, err := git.BranchExistsOnRemote(context.Background(), "/repo", tt.branch)

			if (err != nil) != tt.wantErr {
				t.Errorf("BranchExistsOnRemote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BranchExistsOnRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}

// makeWorktreeDir creates a real on-disk directory to stand in for a worktree
// so os.RemoveAll has something concrete to delete in the fallback tests.
func makeWorktreeDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup worktree dir: %v", err)
	}
	return dir
}

func TestGit_WorktreeRemove_SubmoduleFallback(t *testing.T) {
	tests := []struct {
		name        string
		statusOut   []byte // git status --porcelain output (empty = clean)
		wantErr     bool
		wantRemoved bool // directory deleted via fallback
		wantPrune   bool // git worktree prune invoked
	}{
		{
			name:        "clean tree falls back to manual removal",
			statusOut:   []byte(""),
			wantErr:     false,
			wantRemoved: true,
			wantPrune:   true,
		},
		{
			// A submodule whose checkout differs from the recorded gitlink is
			// reported as modified by plain `git status`, but `--ignore-submodules=all`
			// filters it out, so a superproject-clean tree still falls back.
			name:        "submodule-only modification still falls back",
			statusOut:   []byte(""),
			wantErr:     false,
			wantRemoved: true,
			wantPrune:   true,
		},
		{
			name:        "dirty tree refuses removal",
			statusOut:   []byte(" M file.go"),
			wantErr:     true,
			wantRemoved: false,
			wantPrune:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktreePath := makeWorktreeDir(t)
			exec := NewMockExec()
			exec.AddResponse("git", []string{"worktree", "remove", worktreePath},
				[]byte(submoduleRemoveErr), MockError("exit status 128"))
			exec.AddResponse("git", []string{"status", "--porcelain", "--ignore-submodules=all"}, tt.statusOut, nil)
			exec.AddResponse("git", []string{"worktree", "prune"}, []byte(""), nil)

			git := NewGit(exec)
			err := git.WorktreeRemove(context.Background(), "/repo", worktreePath)

			if (err != nil) != tt.wantErr {
				t.Fatalf("WorktreeRemove() error = %v, wantErr %v", err, tt.wantErr)
			}
			_, statErr := os.Stat(worktreePath)
			removed := os.IsNotExist(statErr)
			if removed != tt.wantRemoved {
				t.Errorf("worktree removed = %v, want %v", removed, tt.wantRemoved)
			}
			if pruned := exec.WasCalled("git", "worktree", "prune"); pruned != tt.wantPrune {
				t.Errorf("prune called = %v, want %v", pruned, tt.wantPrune)
			}
		})
	}
}

func TestGit_WorktreeRemoveForce_SubmoduleFallback(t *testing.T) {
	worktreePath := makeWorktreeDir(t)
	exec := NewMockExec()
	exec.AddResponse("git", []string{"worktree", "remove", "--force", worktreePath},
		[]byte(submoduleRemoveErr), MockError("exit status 128"))
	exec.AddResponse("git", []string{"worktree", "prune"}, []byte(""), nil)

	git := NewGit(exec)
	if err := git.WorktreeRemoveForce(context.Background(), "/repo", worktreePath); err != nil {
		t.Fatalf("WorktreeRemoveForce() unexpected error = %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be removed, stat err = %v", err)
	}
	if !exec.WasCalled("git", "worktree", "prune") {
		t.Error("expected git worktree prune to be called")
	}
}

func TestGit_WorktreeRemoveForce_FallsBackOnAnyError(t *testing.T) {
	// --force means "always win": even a non-submodule refusal (e.g. a locked
	// worktree) must fall back to manual removal.
	worktreePath := makeWorktreeDir(t)
	exec := NewMockExec()
	exec.AddResponse("git", []string{"worktree", "remove", "--force", worktreePath},
		[]byte("fatal: '"+worktreePath+"' is locked"), MockError("exit status 128"))
	exec.AddResponse("git", []string{"worktree", "prune"}, []byte(""), nil)

	git := NewGit(exec)
	if err := git.WorktreeRemoveForce(context.Background(), "/repo", worktreePath); err != nil {
		t.Fatalf("WorktreeRemoveForce() unexpected error = %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be removed, stat err = %v", err)
	}
	if !exec.WasCalled("git", "worktree", "prune") {
		t.Error("expected git worktree prune to be called")
	}
}

func TestGit_WorktreeRemove_SurfacesGitDetail(t *testing.T) {
	worktreePath := makeWorktreeDir(t)
	exec := NewMockExec()
	exec.AddResponse("git", []string{"worktree", "remove", worktreePath},
		[]byte("fatal: some other git failure"), MockError("exit status 1"))

	git := NewGit(exec)
	err := git.WorktreeRemove(context.Background(), "/repo", worktreePath)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "some other git failure") {
		t.Errorf("error should surface git output, got: %v", err)
	}
}
