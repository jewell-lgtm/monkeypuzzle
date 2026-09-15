//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_BranchAtomManagesPieceStackLayers(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("branch-test")
	if _, stderr, err := env.run("create", "--name", "managed-piece", "--skip-switch"); err != nil {
		t.Fatalf("create piece: %v\n%s", err, stderr)
	}
	wt := filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", "managed-piece")

	stdout, stderr, err := env.runInDir(wt, "branches", "create", "api-layer")
	if err != nil {
		t.Fatalf("create: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created struct{ Branch, Base, Piece string }
	if err := json.Unmarshal([]byte(stdout), &created); err != nil || created.Branch != "api-layer" || created.Base != "managed-piece" || created.Piece != "managed-piece" {
		t.Fatalf("create result: %+v err=%v raw=%s", created, err, stdout)
	}
	if current := strings.TrimSpace(env.gitInDir(wt, "branch", "--show-current")); current != "api-layer" {
		t.Fatalf("current = %q", current)
	}

	stdout, stderr, err = env.runInDir(wt, "branch", "list", "--json")
	if err != nil || !strings.Contains(stdout, `"name": "managed-piece"`) || !strings.Contains(stdout, `"name": "api-layer"`) {
		t.Fatalf("list: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	stdout, stderr, err = env.runInDir(wt, "branch", "show", "api-layer", "--json")
	if err != nil || !strings.Contains(stdout, `"piece": "managed-piece"`) {
		t.Fatalf("show: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	stdout, stderr, err = env.runInDir(wt, "branch", "delete", "api-layer")
	if err != nil {
		t.Fatalf("delete: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if current := strings.TrimSpace(env.gitInDir(wt, "branch", "--show-current")); current != "managed-piece" {
		t.Fatalf("current after delete = %q", current)
	}
	if branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)"); strings.Contains(branches, "api-layer") {
		t.Fatalf("deleted layer still exists: %s", branches)
	}
}

func TestCLI_BranchAtomRejectsRawRefsAndProtectsLifecycle(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("branch-test")
	if _, stderr, err := env.run("create", "--name", "managed-piece", "--skip-switch"); err != nil {
		t.Fatalf("create piece: %v\n%s", err, stderr)
	}
	wt := filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", "managed-piece")
	env.gitInDir(env.tmpDir, "branch", "raw-ref")
	if stdout, _, err := env.run("branch", "list", "--json"); err != nil || strings.Contains(stdout, `"name": "raw-ref"`) {
		t.Fatalf("raw ref leaked into atom: err=%v %s", err, stdout)
	}
	if _, stderr, err := env.runInDir(wt, "branch", "delete", "raw-ref"); err == nil || !strings.Contains(stderr, "not managed") {
		t.Fatalf("raw delete: err=%v stderr=%s", err, stderr)
	}
	if _, stderr, err := env.runInDir(wt, "branch", "delete", "managed-piece"); err == nil || !strings.Contains(stderr, "initial branch") {
		t.Fatalf("initial delete: err=%v stderr=%s", err, stderr)
	}

	stdout, stderr, err := env.runInDirWithStdin(wt, `{"prompt":"Add cache layer"}`, "branch", "create")
	if err != nil || !strings.Contains(stdout, `"branch": "add-cache-layer"`) {
		t.Fatalf("prompt create: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := env.runInDir(wt, "branch", "delete"); err == nil || !strings.Contains(stderr, "uncommitted changes") {
		t.Fatalf("dirty delete: err=%v stderr=%s", err, stderr)
	}
}
