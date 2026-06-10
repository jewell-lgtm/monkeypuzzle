//go:build integration

package mp_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// gitOut runs a git command in dir and returns trimmed combined output, failing the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// isAncestor reports whether commit is reachable from ref in dir.
func isAncestor(t *testing.T, dir, commit, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", commit, ref)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestCLI_StackSync_PropagatesMainThroughTwoPieceStack is the happy-path integration
// test: a commit added to main must reach every piece in a two-deep stack after
// `mp stack sync` (default merge strategy).
func TestCLI_StackSync_PropagatesMainThroughTwoPieceStack(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Build a two-piece stack: a (parent main) -> b (parent a).
	pieceA := createPiece(t, env, "a", "main")
	pieceB := createPiece(t, env, "b", "a")

	// Advance main with a new commit (simulating upstream moving forward).
	gitCmd(t, env.tmpDir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(env.tmpDir, "main-change.txt"), []byte("from main\n"), 0o644); err != nil {
		t.Fatalf("write main-change.txt: %v", err)
	}
	gitCmd(t, env.tmpDir, "add", "main-change.txt")
	gitCmd(t, env.tmpDir, "commit", "-m", "feat: advance main")
	mainCommit := strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "HEAD"))

	// Sync the whole stack from the main repo (default merge strategy, no remote).
	stdout, stderr, err := env.run("stack", "sync")
	if err != nil {
		t.Fatalf("stack sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		Strategy string   `json:"strategy"`
		Updated  []string `json:"updated"`
		Status   string   `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result.Status != "synced" {
		t.Errorf("expected status=synced, got %q", result.Status)
	}
	if !contains(result.Updated, "a") || !contains(result.Updated, "b") {
		t.Errorf("expected updated to contain both a and b, got %v", result.Updated)
	}

	// The core guarantee: main's new commit is now reachable from both pieces.
	if !isAncestor(t, pieceA, mainCommit, "HEAD") {
		t.Errorf("main commit %s not reachable from piece a at %s", mainCommit, pieceA)
	}
	if !isAncestor(t, pieceB, mainCommit, "HEAD") {
		t.Errorf("main commit %s not reachable from piece b at %s", mainCommit, pieceB)
	}
}

// createPiece creates a piece with the given name and parent, returning its worktree path.
func createPiece(t *testing.T, env *testEnv, name, parent string) string {
	t.Helper()
	args := []string{"create", "--name", name, "--skip-switch"}
	if parent != "" && parent != "main" {
		args = append(args, "--parent", parent)
	}
	stdout, stderr, err := env.run(args...)
	if err != nil {
		t.Fatalf("piece create %s failed: %v\nstdout: %s\nstderr: %s", name, err, stdout, stderr)
	}
	var res struct {
		WorktreePath string `json:"worktree_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("invalid JSON from piece create %s: %v\noutput: %s", name, err, stdout)
	}
	return res.WorktreePath
}

// TestCLI_StackSetParent_ReparentsPiece is the happy path for `mp stack
// set-parent`: move piece c from parent a to parent b, verify metadata and
// that stack status reflects the new lineage.
func TestCLI_StackSetParent_ReparentsPiece(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	createPiece(t, env, "a", "main")
	createPiece(t, env, "b", "main")
	pieceC := createPiece(t, env, "c", "a")

	// Re-parent c onto b, running from inside c's worktree (current piece).
	stdout, stderr, err := env.runInDir(pieceC, "stack", "set-parent", "--parent", "b")
	if err != nil {
		t.Fatalf("set-parent failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var res struct {
		Piece     string `json:"piece"`
		OldParent string `json:"old_parent"`
		NewParent string `json:"new_parent"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}
	if res.Piece != "c" || res.OldParent != "a" || res.NewParent != "b" {
		t.Errorf("unexpected result: %+v", res)
	}

	// Metadata must record the new parent.
	data, err := os.ReadFile(filepath.Join(pieceC, ".monkeypuzzle", "piece-metadata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if !strings.Contains(string(data), `"parent": "b"`) && !strings.Contains(string(data), `"parent":"b"`) {
		t.Errorf("metadata does not record new parent: %s", data)
	}
}

// TestCLI_StackSetParent_RejectsCycle ensures re-parenting onto a descendant
// fails loudly instead of corrupting the stack DAG.
func TestCLI_StackSetParent_RejectsCycle(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	pieceA := createPiece(t, env, "a", "main")
	createPiece(t, env, "b", "a")

	// a -> b exists; making b a's parent would create a cycle.
	_, stderr, err := env.runInDir(pieceA, "stack", "set-parent", "--parent", "b")
	if err == nil {
		t.Fatal("expected cycle to be rejected")
	}
	if !strings.Contains(stderr, "descendant") {
		t.Errorf("error should mention descendant, got: %s", stderr)
	}
}

// TestCLI_StackSetParent_UnknownParentFails ensures a missing target parent
// fails loudly (non-interactive contract).
func TestCLI_StackSetParent_UnknownParentFails(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	pieceA := createPiece(t, env, "a", "main")

	_, stderr, err := env.runInDir(pieceA, "stack", "set-parent", "--parent", "nope")
	if err == nil {
		t.Fatal("expected unknown parent to fail")
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("error should name the unknown parent, got: %s", stderr)
	}
}
