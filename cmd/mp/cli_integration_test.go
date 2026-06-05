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

// testEnv holds test environment state
type testEnv struct {
	t         *testing.T
	tmpDir    string
	binPath   string
	dataDir   string // MP_DATA_DIR for the binary, isolates the global registry
	configDir string // MP_CONFIG_DIR for the binary, isolates user config
}

// env returns the environment for invoking the test binary, isolating both the
// monkeypuzzle data directory and the user config directory so tests never
// touch the real user state.
func (e *testEnv) env() []string {
	return append(os.Environ(),
		"MP_DATA_DIR="+e.dataDir,
		"MP_CONFIG_DIR="+e.configDir,
	)
}

// setupTestEnv creates a temp directory and builds the binary
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mp-cli-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Build binary to temp location
	binPath := filepath.Join(tmpDir, "mp")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = filepath.Join(os.Getenv("PWD"), "../../")
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}

	configDir := filepath.Join(tmpDir, "mp-config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"multiplexer":"none"}`), 0o644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to seed config: %v", err)
	}

	return &testEnv{
		t:         t,
		tmpDir:    tmpDir,
		binPath:   binPath,
		dataDir:   filepath.Join(tmpDir, "mp-data"),
		configDir: configDir,
	}
}

// cleanup removes temp directory
func (e *testEnv) cleanup() {
	os.RemoveAll(e.tmpDir)
}

// run executes mp command and returns stdout, stderr, and error
func (e *testEnv) run(args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir
	cmd.Env = e.env()

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runWithStdin executes mp command with stdin input
func (e *testEnv) runWithStdin(stdin string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir
	cmd.Env = e.env()
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runInDir executes mp command from a specific directory
func (e *testEnv) runInDir(dir string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = e.env()

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runInDirWithStdin executes mp command from a specific directory with stdin
func (e *testEnv) runInDirWithStdin(dir, stdin string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = e.env()
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// initProject initializes monkeypuzzle in the temp directory
func (e *testEnv) initProject(name string) {
	e.t.Helper()
	input := `{"name":"` + name + `","issue_provider":"markdown","pr_provider":"github"}`
	stdout, stderr, err := e.runWithStdin(input, "init")
	if err != nil {
		e.t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

// createIssue creates an issue file directly
func (e *testEnv) createIssue(filename, title, status string) {
	e.t.Helper()
	issuesDir := filepath.Join(e.tmpDir, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		e.t.Fatalf("failed to create issues dir: %v", err)
	}
	content := "---\ntitle: " + title + "\nstatus: " + status + "\n---\n\n# " + title + "\n"
	if err := os.WriteFile(filepath.Join(issuesDir, filename), []byte(content), 0644); err != nil {
		e.t.Fatalf("failed to write issue: %v", err)
	}
}

// TestCLI_Init tests mp init command
func TestCLI_Init(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Test init with JSON stdin
	input := `{"name":"testproject","issue_provider":"markdown","pr_provider":"github"}`
	stdout, _, err := env.runWithStdin(input, "init")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Check nested project.name
	project, ok := result["project"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'project' object in result, got %v", result)
	}
	if project["name"] != "testproject" {
		t.Errorf("expected project.name 'testproject', got %v", project["name"])
	}

	// Verify config file created
	configPath := filepath.Join(env.tmpDir, ".monkeypuzzle", "monkeypuzzle.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file not created")
	}
}

// TestCLI_Init_Schema tests mp init --schema
func TestCLI_Init_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("init", "--schema")
	if err != nil {
		t.Fatalf("init --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}

	// Should have expected fields
	if _, ok := schema["name"]; !ok {
		t.Error("schema missing 'name' field")
	}
	if _, ok := schema["issue_provider"]; !ok {
		t.Error("schema missing 'issue_provider' field")
	}
}

// TestCLI_IssueList tests mp issue list command
func TestCLI_IssueList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	env.createIssue("todo-feature.md", "Todo Feature", "todo")
	env.createIssue("done-feature.md", "Done Feature", "done")

	// List all issues
	stdout, _, err := env.run("issue", "list")
	if err != nil {
		t.Fatalf("issue list failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
}

// TestCLI_IssueList_StatusFilter tests mp issue list --status
func TestCLI_IssueList_ReturnsAllIgnoringLegacyStatus(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	// Legacy files carrying a status field must still list without error.
	env.createIssue("todo-feature.md", "Todo Feature", "todo")
	env.createIssue("done-feature.md", "Done Feature", "done")
	env.createIssue("wip-feature.md", "WIP Feature", "in-progress")

	stdout, _, err := env.run("issue", "list")
	if err != nil {
		t.Fatalf("issue list failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(issues))
	}
}

// TestCLI_IssueList_Schema tests mp issue list --schema
func TestCLI_IssueList_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("issue", "list", "--schema")
	if err != nil {
		t.Fatalf("issue list --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}

	if _, ok := schema["status"]; ok {
		t.Error("schema should no longer contain a 'status' field")
	}
}

// TestCLI_IssueCreate tests mp issue create command
func TestCLI_IssueCreate(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")

	// Create issue with JSON stdin
	input := `{"title":"New Feature","description":"Feature description"}`
	stdout, _, err := env.runWithStdin(input, "issue", "create")
	if err != nil {
		t.Fatalf("issue create failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result["title"] != "New Feature" {
		t.Errorf("expected title 'New Feature', got %v", result["title"])
	}

	// Verify file created
	path, ok := result["path"].(string)
	if !ok {
		t.Fatal("result missing 'path' field")
	}
	fullPath := filepath.Join(env.tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("issue file not created at %s", fullPath)
	}
}

// TestCLI_IssueCreate_Flags tests mp issue create with flags
func TestCLI_IssueCreate_Flags(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")

	stdout, _, err := env.run("issue", "create", "--title", "Flag Feature")
	if err != nil {
		t.Fatalf("issue create --title failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result["title"] != "Flag Feature" {
		t.Errorf("expected title 'Flag Feature', got %v", result["title"])
	}
}

// initGitRepo initializes a git repo in the temp directory
func (e *testEnv) initGitRepo() {
	e.t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git init failed: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git config email failed: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git config name failed: %v\n%s", err, output)
	}

	// Create initial commit
	readmePath := filepath.Join(e.tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		e.t.Fatalf("failed to write README: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git add failed: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git commit failed: %v\n%s", err, output)
	}

	// Normalize the default branch name so commands defaulting to --main-branch=main work.
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = e.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git branch -M main failed: %v\n%s", err, output)
	}
}

// TestCLI_PieceCreate tests mp create outputs valid JSON
func TestCLI_PieceCreate(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("add-feature.md", "Add Feature", "todo")

	stdout, stderr, err := env.run("create", "--issue", "Add Feature", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if name, _ := result["name"].(string); name != "add-feature" {
		t.Errorf("expected piece name 'add-feature', got %v", result["name"])
	}
}

// TestCLI_PieceCreate_WithName tests mp create --name outputs valid JSON
func TestCLI_PieceCreate_WithName(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	stdout, stderr, err := env.run("create", "--name", "my-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create --name failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceCreate_Schema tests mp create --schema outputs valid JSON
func TestCLI_PieceCreate_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("create", "--schema")
	if err != nil {
		t.Fatalf("piece create --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceList tests mp list --flat outputs valid JSON
func TestCLI_PieceList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	_, _, err := env.run("create", "--name", "test-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v", err)
	}

	stdout, _, err := env.run("list", "--flat")
	if err != nil {
		t.Fatalf("piece list failed: %v", err)
	}

	var pieces []map[string]any
	if err := json.Unmarshal([]byte(stdout), &pieces); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PRCreate_Schema tests mp pr create --schema outputs valid JSON
func TestCLI_PRCreate_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("pr", "create", "--schema")
	if err != nil {
		t.Fatalf("pr create --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_ClaudeSkill tests mp claude skill creates skill file
func TestCLI_ClaudeSkill(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")

	stdout, stderr, err := env.run("claude", "skill")
	if err != nil {
		t.Fatalf("claude skill failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Verify skill file was created
	skillPath := filepath.Join(env.tmpDir, ".claude", "skills", "managing-monkeypuzzle", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Error("skill file not created")
	}
}

// TestCLI_Init_CreatesSkill tests init with create_skill=true creates skill
func TestCLI_Init_CreatesSkill(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	input := `{"name":"test","issue_provider":"markdown","pr_provider":"github","create_skill":true}`
	stdout, stderr, err := env.runWithStdin(input, "init")
	if err != nil {
		t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify skill file was created
	skillPath := filepath.Join(env.tmpDir, ".claude", "skills", "managing-monkeypuzzle", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Error("skill file not created during init")
	}
}

// TestCLI_Init_SkipsSkill tests init with create_skill=false skips skill
func TestCLI_Init_SkipsSkill(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	input := `{"name":"test","issue_provider":"markdown","pr_provider":"github","create_skill":false}`
	stdout, stderr, err := env.runWithStdin(input, "init")
	if err != nil {
		t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify skill file was NOT created
	skillPath := filepath.Join(env.tmpDir, ".claude", "skills", "managing-monkeypuzzle", "SKILL.md")
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Error("skill file should not be created when create_skill=false")
	}
}

// TestCLI_PieceDone_FromMergedPiece tests mp done from inside merged piece
func TestCLI_PieceDone_FromMergedPiece(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Create a piece
	stdout, stderr, err := env.run("create", "--name", "test-done", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var newResult map[string]any
	if err := json.Unmarshal([]byte(stdout), &newResult); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktreePath := newResult["worktree_path"].(string)

	// Simulate merge: merge piece branch into main
	cmd := exec.Command("git", "merge", "test-done", "--no-edit")
	cmd.Dir = env.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge failed: %v\n%s", err, output)
	}

	// Run piece done from the worktree
	stdout, stderr, err = env.runInDirWithStdin(worktreePath, "{}", "done")
	if err != nil {
		t.Fatalf("piece done failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Verify output has main_path
	if result["main_path"] == nil {
		t.Error("result missing main_path")
	}

	// Verify worktree was removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree should have been removed")
	}
}

// TestCLI_PieceAbandon_CurrentPiece tests mp abandon from current piece without --name
func TestCLI_PieceAbandon_CurrentPiece(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Create a piece
	stdout, stderr, err := env.run("create", "--name", "test-abandon", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var newResult map[string]any
	if err := json.Unmarshal([]byte(stdout), &newResult); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktreePath := newResult["worktree_path"].(string)

	// Run piece abandon from the worktree (no --name, detect current)
	stdout, stderr, err = env.runInDirWithStdin(worktreePath, `{"force":true}`, "abandon")
	if err != nil {
		t.Fatalf("piece abandon failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Verify output has main_path
	if result["main_path"] == nil {
		t.Error("result missing main_path")
	}

	// Verify worktree was removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree should have been removed")
	}
}

// TestCLI_Flatten_RemovesAllPieces tests that mp flatten removes every piece
// worktree, returning the repo to a flat main-only state.
func TestCLI_Flatten_RemovesAllPieces(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Create a couple of pieces.
	var worktrees []string
	for _, name := range []string{"piece-a", "piece-b"} {
		stdout, stderr, err := env.run("create", "--name", name, "--skip-switch")
		if err != nil {
			t.Fatalf("piece create %s failed: %v\nstdout: %s\nstderr: %s", name, err, stdout, stderr)
		}
		var created map[string]any
		if err := json.Unmarshal([]byte(stdout), &created); err != nil {
			t.Fatalf("invalid JSON from piece create: %v", err)
		}
		worktrees = append(worktrees, created["worktree_path"].(string))
	}

	// Flatten via stdin JSON (non-interactive; skips the confirmation prompt).
	stdout, stderr, err := env.runWithStdin(`{"force":true}`, "flatten")
	if err != nil {
		t.Fatalf("flatten failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		Removed  []map[string]any `json:"removed"`
		Count    int              `json:"count"`
		MainPath string           `json:"main_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result.Count != 2 {
		t.Errorf("expected 2 pieces removed, got %d", result.Count)
	}
	if result.MainPath == "" {
		t.Error("result missing main_path")
	}

	// Every worktree should be gone.
	for _, wt := range worktrees {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("worktree %s should have been removed", wt)
		}
	}

	// And listing pieces should now return an empty set.
	stdout, _, err = env.run("list", "--flat")
	if err != nil {
		t.Fatalf("piece list failed: %v", err)
	}
	var remaining []map[string]any
	if err := json.Unmarshal([]byte(stdout), &remaining); err != nil {
		t.Fatalf("invalid JSON from piece list: %v\noutput: %s", err, stdout)
	}
	if len(remaining) != 0 {
		t.Errorf("expected no pieces after flatten, got %d", len(remaining))
	}
}

// TestCLI_Flatten_Schema tests mp flatten --schema outputs valid JSON
func TestCLI_Flatten_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("flatten", "--schema")
	if err != nil {
		t.Fatalf("flatten --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_Flatten_DryRun tests that mp flatten --dry-run reports pieces without
// removing them.
func TestCLI_Flatten_DryRun(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	stdout, stderr, err := env.run("create", "--name", "keep-me", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktree := created["worktree_path"].(string)

	stdout, stderr, err = env.run("flatten", "--dry-run")
	if err != nil {
		t.Fatalf("flatten --dry-run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var result struct {
		Count  int  `json:"count"`
		DryRun bool `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result.Count != 1 || !result.DryRun {
		t.Errorf("expected dry-run reporting 1 piece, got count=%d dry_run=%v", result.Count, result.DryRun)
	}

	// The worktree must still exist.
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree should still exist after dry-run: %v", err)
	}
}

// TestCLI_IssueSearch tests mp issue search command with query
func TestCLI_IssueSearch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	env.createIssue("add-auth.md", "Add authentication", "todo")
	env.createIssue("fix-auth-bug.md", "Fix authentication bug", "in-progress")
	env.createIssue("add-logging.md", "Add logging", "todo")

	// Search with query flag
	stdout, _, err := env.run("issue", "search", "--query", "auth")
	if err != nil {
		t.Fatalf("issue search failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Should find 2 issues matching "auth"
	if len(issues) != 2 {
		t.Errorf("expected 2 issues matching 'auth', got %d", len(issues))
	}
}

// TestCLI_IssueSearch_StatusFilter tests mp issue search with status filter
func TestCLI_IssueSearch_Query(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	env.createIssue("add-auth.md", "Add authentication", "todo")
	env.createIssue("fix-auth-bug.md", "Fix authentication bug", "in-progress")
	env.createIssue("add-logging.md", "Add logging", "todo")

	// Search by query (fuzzy match on title)
	stdout, _, err := env.run("issue", "search", "--query", "auth")
	if err != nil {
		t.Fatalf("issue search --query failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Should find 2 issues matching "auth"
	if len(issues) != 2 {
		t.Errorf("expected 2 issues matching 'auth', got %d", len(issues))
	}
}

// TestCLI_IssueSearch_Stdin tests mp issue search with stdin JSON
func TestCLI_IssueSearch_Stdin(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	env.createIssue("add-auth.md", "Add authentication", "todo")
	env.createIssue("fix-bug.md", "Fix bug", "done")

	// Search with stdin JSON
	input := `{"query":"auth"}`
	stdout, _, err := env.runWithStdin(input, "issue", "search")
	if err != nil {
		t.Fatalf("issue search with stdin failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
}

// TestCLI_IssueSearch_Schema tests mp issue search --schema
func TestCLI_IssueSearch_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("issue", "search", "--schema")
	if err != nil {
		t.Fatalf("issue search --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}

	if _, ok := schema["query"]; !ok {
		t.Error("schema missing 'query' field")
	}
	if _, ok := schema["status"]; ok {
		t.Error("schema should no longer contain a 'status' field")
	}
}

// runWithEnv executes mp command with custom environment variables
func (e *testEnv) runWithEnv(env map[string]string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir
	cmd.Env = e.env()
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// TestCLI_ConfigGet tests mp config get returns default value
func TestCLI_ConfigGet(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Use temp dir for XDG_CONFIG_HOME to isolate config
	configHome := filepath.Join(env.tmpDir, "config")

	stdout, stderr, err := env.runWithEnv(map[string]string{"XDG_CONFIG_HOME": configHome}, "config", "get", "multiplexer")
	if err != nil {
		t.Fatalf("config get failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// Default value is "none"
	if result["value"] != "none" {
		t.Errorf("expected default multiplexer 'none', got %v", result["value"])
	}
}

// TestCLI_ConfigSet tests mp config set changes value
func TestCLI_ConfigSet(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	configHome := filepath.Join(env.tmpDir, "config")

	// Set multiplexer to tmux
	stdout, stderr, err := env.runWithEnv(map[string]string{"XDG_CONFIG_HOME": configHome}, "config", "set", "multiplexer", "tmux")
	if err != nil {
		t.Fatalf("config set failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result["value"] != "tmux" {
		t.Errorf("expected set value 'tmux', got %v", result["value"])
	}

	// Verify get returns new value
	stdout, stderr, err = env.runWithEnv(map[string]string{"XDG_CONFIG_HOME": configHome}, "config", "get", "multiplexer")
	if err != nil {
		t.Fatalf("config get after set failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if result["value"] != "tmux" {
		t.Errorf("expected 'tmux' after set, got %v", result["value"])
	}
}

// TestCLI_ConfigSet_InvalidKey tests mp config set with unknown key
func TestCLI_ConfigSet_InvalidKey(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	configHome := filepath.Join(env.tmpDir, "config")

	_, _, err := env.runWithEnv(map[string]string{"XDG_CONFIG_HOME": configHome}, "config", "set", "unknown_key", "value")
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

// TestCLI_ConfigSet_InvalidValue tests mp config set with invalid value
func TestCLI_ConfigSet_InvalidValue(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	configHome := filepath.Join(env.tmpDir, "config")

	_, _, err := env.runWithEnv(map[string]string{"XDG_CONFIG_HOME": configHome}, "config", "set", "multiplexer", "invalid")
	if err == nil {
		t.Error("expected error for invalid multiplexer value, got nil")
	}
}
