//go:build integration

package main_test

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
	// --apply because sync is dry-run by default.
	stdout, stderr, err := env.run("stack", "sync", "--apply")
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

	// --apply must go straight to the sync: no preview pass, no "[dry-run]"
	// lines telling the caller to pass the flag they just passed.
	if strings.Contains(stderr, "[dry-run]") {
		t.Errorf("--apply still ran the dry-run preview:\n%s", stderr)
	}

	// The core guarantee: main's new commit is now reachable from both pieces.
	if !isAncestor(t, pieceA, mainCommit, "HEAD") {
		t.Errorf("main commit %s not reachable from piece a at %s", mainCommit, pieceA)
	}
	if !isAncestor(t, pieceB, mainCommit, "HEAD") {
		t.Errorf("main commit %s not reachable from piece b at %s", mainCommit, pieceB)
	}
}

// TestCLI_StackSync_DryRunByDefault verifies that a non-interactive `mp stack
// sync` (no --apply) previews the work without touching any branch.
func TestCLI_StackSync_DryRunByDefault(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	pieceA := createPiece(t, env, "a", "main")
	createPiece(t, env, "b", "a")
	shaABefore := strings.TrimSpace(gitOut(t, pieceA, "rev-parse", "HEAD"))

	// Advance main so a real sync would have something to propagate.
	gitCmd(t, env.tmpDir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(env.tmpDir, "main-change.txt"), []byte("from main\n"), 0o644); err != nil {
		t.Fatalf("write main-change.txt: %v", err)
	}
	gitCmd(t, env.tmpDir, "add", "main-change.txt")
	gitCmd(t, env.tmpDir, "commit", "-m", "feat: advance main")

	// No --apply: must be a dry-run that mutates nothing.
	stdout, stderr, err := env.run("stack", "sync")
	if err != nil {
		t.Fatalf("stack sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		Updated []string `json:"updated"`
		Status  string   `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result.Status != "dry-run" {
		t.Errorf("expected status=dry-run, got %q", result.Status)
	}
	if !contains(result.Updated, "a") || !contains(result.Updated, "b") {
		t.Errorf("expected preview to list both a and b, got %v", result.Updated)
	}
	if !strings.Contains(stderr, "[dry-run]") {
		t.Errorf("expected [dry-run] notice on stderr, got: %s", stderr)
	}

	// The core guarantee: nothing moved.
	if got := strings.TrimSpace(gitOut(t, pieceA, "rev-parse", "HEAD")); got != shaABefore {
		t.Errorf("dry-run moved piece a: got %s want %s", got, shaABefore)
	}
	if _, err := os.Stat(filepath.Join(env.tmpDir, ".monkeypuzzle", "stack-snapshot.json")); err == nil {
		t.Error("dry-run wrote an undo snapshot; it should not mutate state")
	}
}

// TestCLI_StackSync_ApplyAndDryRunConflict: the two flags contradict each other
// and must fail loudly rather than pick one.
func TestCLI_StackSync_ApplyAndDryRunConflict(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	createPiece(t, env, "a", "main")

	stdout, stderr, err := env.run("stack", "sync", "--apply", "--dry-run")
	if err == nil {
		t.Fatalf("expected error for --apply --dry-run\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "cannot use --apply and --dry-run together") {
		t.Errorf("expected conflict message on stderr, got: %s", stderr)
	}
}

// TestCLI_StackSync_FromCustomRef verifies that --from drives the main update:
// it fetches the named remote (not a hardcoded "origin") and fast-forwards local
// main to that ref before propagating it down the stack.
func TestCLI_StackSync_FromCustomRef(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Wire a bare repo as a non-origin remote ("upstream") and seed it with main.
	bare := filepath.Join(env.tmpDir, "upstream.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}
	env.gitInDir(env.tmpDir, "remote", "add", "upstream", bare)
	env.gitInDir(env.tmpDir, "push", "upstream", "main")

	pieceA := createPiece(t, env, "a", "main")
	pieceB := createPiece(t, env, "b", "a")

	// Advance upstream/main with a commit local main does not yet have, then move
	// local main back so it must fast-forward to upstream to pick the commit up.
	if err := os.WriteFile(filepath.Join(env.tmpDir, "upstream-change.txt"), []byte("from upstream\n"), 0o644); err != nil {
		t.Fatalf("write upstream-change.txt: %v", err)
	}
	env.gitInDir(env.tmpDir, "add", "upstream-change.txt")
	env.gitInDir(env.tmpDir, "commit", "-m", "feat: advance upstream")
	upstreamCommit := env.gitInDir(env.tmpDir, "rev-parse", "HEAD")
	env.gitInDir(env.tmpDir, "push", "upstream", "main")
	env.gitInDir(env.tmpDir, "reset", "--hard", "HEAD~1")

	stdout, stderr, err := env.run("stack", "sync", "--from", "upstream/main", "--apply")
	if err != nil {
		t.Fatalf("stack sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		Updated []string `json:"updated"`
		Status  string   `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result.Status != "synced" {
		t.Errorf("expected status=synced, got %q", result.Status)
	}

	// Local main fast-forwarded to upstream/main, and the commit reached the stack.
	if got := env.gitInDir(env.tmpDir, "rev-parse", "HEAD"); got != upstreamCommit {
		t.Errorf("local main not fast-forwarded to upstream: got %s want %s", got, upstreamCommit)
	}
	if !isAncestor(t, pieceA, upstreamCommit, "HEAD") {
		t.Errorf("upstream commit %s not reachable from piece a", upstreamCommit)
	}
	if !isAncestor(t, pieceB, upstreamCommit, "HEAD") {
		t.Errorf("upstream commit %s not reachable from piece b", upstreamCommit)
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

// TestCLI_StackUndo_RestoresPreSyncSHAs: sync writes a snapshot before mutating;
// undo restores every piece branch to its pre-sync commit.
func TestCLI_StackUndo_RestoresPreSyncSHAs(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	pieceA := createPiece(t, env, "a", "main")
	createPiece(t, env, "b", "a")

	shaA := strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "a"))
	shaB := strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "b"))

	// Advance main so sync has something to propagate.
	gitCmd(t, env.tmpDir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(env.tmpDir, "main-change.txt"), []byte("from main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitCmd(t, env.tmpDir, "add", "main-change.txt")
	gitCmd(t, env.tmpDir, "commit", "-m", "main moves on")

	if _, stderr, err := env.run("stack", "sync", "--apply"); err != nil {
		t.Fatalf("stack sync failed: %v\nstderr: %s", err, stderr)
	}

	// Sanity: sync moved the branches.
	if strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "a")) == shaA {
		t.Fatal("sync did not move a; test setup broken")
	}

	stdout, stderr, err := env.runInDir(pieceA, "stack", "undo")
	if err != nil {
		t.Logf("porcelain in a:\n%s", gitOut(t, pieceA, "status", "--porcelain"))
		t.Fatalf("stack undo failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if got := strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "a")); got != shaA {
		t.Errorf("a not restored: got %s want %s", got, shaA)
	}
	if got := strings.TrimSpace(gitOut(t, env.tmpDir, "rev-parse", "b")); got != shaB {
		t.Errorf("b not restored: got %s want %s", got, shaB)
	}
}

// TestCLI_StackUndo_NoSnapshotFails: undo without a prior sync fails loudly.
func TestCLI_StackUndo_NoSnapshotFails(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	pieceA := createPiece(t, env, "a", "main")

	_, stderr, err := env.runInDir(pieceA, "stack", "undo")
	if err == nil {
		t.Fatal("expected undo without snapshot to fail")
	}
	if !strings.Contains(stderr, "snapshot") {
		t.Errorf("error should mention snapshot, got: %s", stderr)
	}
}

// TestCLI_StackUndo_DirtyWorktreeFails: undo refuses to discard uncommitted work.
func TestCLI_StackUndo_DirtyWorktreeFails(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	pieceA := createPiece(t, env, "a", "main")

	gitCmd(t, env.tmpDir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(env.tmpDir, "main-change.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitCmd(t, env.tmpDir, "add", "main-change.txt")
	gitCmd(t, env.tmpDir, "commit", "-m", "main moves on")

	if _, stderr, err := env.run("stack", "sync", "--apply"); err != nil {
		t.Fatalf("stack sync failed: %v\nstderr: %s", err, stderr)
	}

	// Dirty a's worktree with a tracked modification, then try to undo.
	// (Untracked files survive reset --hard, so only tracked changes block.)
	if err := os.WriteFile(filepath.Join(pieceA, "README.md"), []byte("locally modified\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, stderr, err := env.runInDir(pieceA, "stack", "undo")
	if err == nil {
		t.Fatal("expected undo with dirty worktree to fail")
	}
	if !strings.Contains(stderr, "uncommitted") && !strings.Contains(stderr, "dirty") {
		t.Errorf("error should mention dirty/uncommitted state, got: %s", stderr)
	}
}
