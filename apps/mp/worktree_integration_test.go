//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_WorktreeAtomListsCandidatesAndDeletesManagedPieces(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("worktree-test")

	rawPath := filepath.Join(env.tmpDir, "raw-worktree")
	env.gitInDir(env.tmpDir, "worktree", "add", "-b", "raw-branch", rawPath)
	stdout, stderr, err := env.run("create", "--name", "managed-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("create managed piece: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	stdout, stderr, err = env.run("worktrees")
	if err != nil {
		t.Fatalf("bare non-TTY list: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var listed struct {
		Worktrees []struct {
			Path      string `json:"path"`
			Branch    string `json:"branch"`
			Managed   bool   `json:"managed"`
			Piece     string `json:"piece"`
			Main      bool   `json:"main"`
			SizeBytes int64  `json:"size_bytes"`
			Inbox     bool   `json:"in_inbox"`
			Lifecycle string `json:"lifecycle"`
		} `json:"worktrees"`
	}
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("list JSON: %v\n%s", err, stdout)
	}
	var sawMain, sawRaw, sawManaged bool
	for _, row := range listed.Worktrees {
		sawMain = sawMain || row.Main
		sawRaw = sawRaw || row.Branch == "raw-branch" && !row.Managed
		sawManaged = sawManaged || row.Piece == "managed-piece" && row.Managed && row.Inbox && row.SizeBytes > 0 && row.Lifecycle == "idle"
	}
	if !sawMain || !sawRaw || !sawManaged {
		t.Fatalf("classification main=%t raw=%t managed=%t: %+v", sawMain, sawRaw, sawManaged, listed.Worktrees)
	}

	if stdout, stderr, err = env.run("worktree", "delete", "raw-branch"); err == nil || !strings.Contains(stderr, "not mp-managed") {
		t.Fatalf("delete raw should be refused: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if _, err := os.Stat(rawPath); err != nil {
		t.Fatalf("raw worktree should remain: %v", err)
	}
	if branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)"); !strings.Contains(branches, "raw-branch") {
		t.Fatalf("raw branch should be kept: %s", branches)
	}

	if stdout, stderr, err = env.run("worktree", "delete", "managed-piece"); err != nil {
		t.Fatalf("delete managed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	managedPath := filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", "managed-piece")
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed worktree still exists: %v", err)
	}
	if branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)"); !strings.Contains(branches, "managed-piece") {
		t.Fatalf("managed branch should be kept: %s", branches)
	}

	if _, stderr, err = env.run("worktree", "delete", "main", "--force"); err == nil || !strings.Contains(stderr, "main worktree") {
		t.Fatalf("main deletion: err=%v stderr=%s", err, stderr)
	}
}

func TestCLI_WorktreeDeleteJSONAndBranchRemoval(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("worktree-test")
	if _, stderr, err := env.run("create", "--name", "json-worktree", "--skip-switch"); err != nil {
		t.Fatalf("create managed piece: %v\n%s", err, stderr)
	}
	stdout, stderr, err := env.runWithStdin(`{"selector":"json-worktree","delete_branch":true}`, "worktree", "delete")
	if err != nil {
		t.Fatalf("JSON delete: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"branch_deleted": true`) {
		t.Fatalf("delete result: %s", stdout)
	}
	if branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)"); strings.Contains(branches, "json-worktree") {
		t.Fatalf("branch still exists: %s", branches)
	}
}
