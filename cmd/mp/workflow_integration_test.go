//go:build integration

package mp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkflow_Default_PieceCreate_FiresBranchCreated covers the default
// workflow: creating a piece from an issue should drive the issue from
// todo → in-progress via the branch.created event.
func TestWorkflow_Default_PieceCreate_FiresBranchCreated(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("add-feature.md", "Add Feature", "todo")

	stdout, stderr, err := env.run("piece", "create", "--issue", "Add Feature", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	body, err := os.ReadFile(filepath.Join(env.tmpDir, "issues", "add-feature.md"))
	if err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if !strings.Contains(string(body), "status: in-progress") {
		t.Errorf("expected status: in-progress in frontmatter, got:\n%s", body)
	}
}

// TestWorkflow_Default_PieceMerge_FiresPRMerged covers the second half of the
// default workflow: merging a piece moves the linked issue in-progress → done
// via the pr.merged event.
func TestWorkflow_Default_PieceMerge_FiresPRMerged(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("ship-it.md", "Ship It", "todo")

	stdout, stderr, err := env.run("piece", "create", "--issue", "Ship It", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\n%s\n%s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	worktreePath := created["worktree_path"].(string)

	// Make a real commit on the piece so merge has something to integrate.
	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	gitCmd(t, worktreePath, "add", ".")
	gitCmd(t, worktreePath, "commit", "-m", "feat: ship it")

	stdout, stderr, err = env.runInDirWithStdin(worktreePath, "{}", "piece", "merge")
	if err != nil {
		t.Fatalf("piece merge failed: %v\n%s\n%s", err, stdout, stderr)
	}

	body, err := os.ReadFile(filepath.Join(env.tmpDir, "issues", "ship-it.md"))
	if err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if !strings.Contains(string(body), "status: done") {
		t.Errorf("expected status: done after merge, got:\n%s", body)
	}
}

// TestWorkflow_Default_IssueAbandon_FiresAbandoned covers the cancel axis:
// `mp issue abandon` should move an issue to `cancelled`, distinct from
// `done`.
func TestWorkflow_Default_IssueAbandon_FiresAbandoned(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("drop-this.md", "Drop This", "in-progress")

	stdout, stderr, err := env.runWithStdin(
		`{"id":"issues/drop-this.md"}`, "issue", "abandon",
	)
	if err != nil {
		t.Fatalf("issue abandon failed: %v\n%s\n%s", err, stdout, stderr)
	}

	body, err := os.ReadFile(filepath.Join(env.tmpDir, "issues", "drop-this.md"))
	if err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if !strings.Contains(string(body), "status: cancelled") {
		t.Errorf("expected status: cancelled, got:\n%s", body)
	}
}

// TestWorkflow_Custom_BlockProgressesViaAdvance covers a project that ships a
// non-default workflow. Creating a piece moves it to the workflow's
// "in_progress" state, and `mp issue advance` walks it through the manual
// stages.
func TestWorkflow_Custom_BlockProgressesViaAdvance(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Replace monkeypuzzle.json with a custom workflow.
	cfgPath := filepath.Join(env.tmpDir, ".monkeypuzzle", "monkeypuzzle.json")
	custom := `{
  "version": "1",
  "project": { "name": "test" },
  "issues": { "provider": "markdown", "config": { "directory": "issues" } },
  "pr": { "provider": "github", "config": {} },
  "workflow": {
    "states": ["backlog", "in_progress", "development_done", "ready_for_qa", "ready_for_production", "done"],
    "initial": "backlog",
    "terminal": ["done"],
    "cancel": { "state": "cancelled", "from_any": true },
    "transitions": [
      { "from": "backlog", "to": "in_progress", "on": "branch.created" },
      { "from": "in_progress", "to": "development_done", "on": "pr.opened.ready" },
      { "from": "development_done", "to": "ready_for_qa", "on": "pr.checks.green" },
      { "from": "ready_for_qa", "to": "ready_for_production", "on": "acceptance.passed" },
      { "from": "ready_for_production", "to": "done", "on": "released" },
      { "from": "*", "to": "cancelled", "on": "abandoned" }
    ],
    "provider_map": {
      "markdown": {
        "backlog":              { "frontmatter": "backlog" },
        "in_progress":          { "frontmatter": "in_progress" },
        "development_done":     { "frontmatter": "development_done" },
        "ready_for_qa":         { "frontmatter": "ready_for_qa" },
        "ready_for_production": { "frontmatter": "ready_for_production" },
        "done":                 { "frontmatter": "done" },
        "cancelled":            { "frontmatter": "cancelled" }
      }
    }
  }
}`
	if err := os.WriteFile(cfgPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}

	env.createIssue("multi-stage.md", "Multi Stage", "backlog")

	// Create the piece. Default-flow's branch.created should move backlog → in_progress.
	if _, stderr, err := env.run("piece", "create", "--issue", "Multi Stage", "--skip-switch"); err != nil {
		t.Fatalf("piece create failed: %v\n%s", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(env.tmpDir, "issues", "multi-stage.md"))
	if !strings.Contains(string(body), "status: in_progress") {
		t.Fatalf("after piece create, want status: in_progress, got:\n%s", body)
	}

	// Fire pr.opened.ready manually since the PR provider isn't wired here.
	if _, stderr, err := env.runWithStdin(
		`{"id":"issues/multi-stage.md","event":"pr.opened.ready"}`,
		"issue", "fire",
	); err != nil {
		t.Fatalf("issue fire pr.opened.ready: %v\n%s", err, stderr)
	}
	body, _ = os.ReadFile(filepath.Join(env.tmpDir, "issues", "multi-stage.md"))
	if !strings.Contains(string(body), "status: development_done") {
		t.Fatalf("after fire pr.opened.ready, want development_done, got:\n%s", body)
	}

	// Fire pr.checks.green by name (single outbound, so `advance` would also work).
	if _, stderr, err := env.runWithStdin(
		`{"id":"issues/multi-stage.md","event":"pr.checks.green"}`,
		"issue", "fire",
	); err != nil {
		t.Fatalf("issue fire pr.checks.green: %v\n%s", err, stderr)
	}
	body, _ = os.ReadFile(filepath.Join(env.tmpDir, "issues", "multi-stage.md"))
	if !strings.Contains(string(body), "status: ready_for_qa") {
		t.Fatalf("after pr.checks.green, want ready_for_qa, got:\n%s", body)
	}

	// `mp issue advance` from ready_for_qa: single outbound manual event (acceptance.passed).
	if _, stderr, err := env.runWithStdin(
		`{"id":"issues/multi-stage.md"}`, "issue", "advance",
	); err != nil {
		t.Fatalf("issue advance from ready_for_qa: %v\n%s", err, stderr)
	}
	body, _ = os.ReadFile(filepath.Join(env.tmpDir, "issues", "multi-stage.md"))
	if !strings.Contains(string(body), "status: ready_for_production") {
		t.Fatalf("after advance, want ready_for_production, got:\n%s", body)
	}

	// Advance again to done.
	if _, stderr, err := env.runWithStdin(
		`{"id":"issues/multi-stage.md"}`, "issue", "advance",
	); err != nil {
		t.Fatalf("issue advance to done: %v\n%s", err, stderr)
	}
	body, _ = os.ReadFile(filepath.Join(env.tmpDir, "issues", "multi-stage.md"))
	if !strings.Contains(string(body), "status: done") {
		t.Errorf("after final advance, want done, got:\n%s", body)
	}
}

// TestWorkflow_Issue_Reopen covers the uncancel path: an issue moved to
// `cancelled` can be reopened to a named state via `mp issue reopen --to`.
func TestWorkflow_Issue_Reopen(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("comeback.md", "Comeback", "cancelled")

	if _, stderr, err := env.runWithStdin(
		`{"id":"issues/comeback.md","to":"todo"}`, "issue", "reopen",
	); err != nil {
		t.Fatalf("issue reopen failed: %v\n%s", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(env.tmpDir, "issues", "comeback.md"))
	if !strings.Contains(string(body), "status: todo") {
		t.Errorf("after reopen, want status: todo, got:\n%s", body)
	}
}
