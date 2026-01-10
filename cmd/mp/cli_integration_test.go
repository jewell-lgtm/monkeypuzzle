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
	t       *testing.T
	tmpDir  string
	binPath string
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

	return &testEnv{t: t, tmpDir: tmpDir, binPath: binPath}
}

// cleanup removes temp directory
func (e *testEnv) cleanup() {
	os.RemoveAll(e.tmpDir)
}

// run executes mp command and returns stdout, stderr, and error
func (e *testEnv) run(args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir

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
func TestCLI_IssueList_StatusFilter(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initProject("test")
	env.createIssue("todo-feature.md", "Todo Feature", "todo")
	env.createIssue("done-feature.md", "Done Feature", "done")
	env.createIssue("wip-feature.md", "WIP Feature", "in-progress")

	// Filter by todo status
	stdout, _, err := env.run("issue", "list", "--status", "todo")
	if err != nil {
		t.Fatalf("issue list --status failed: %v", err)
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	if len(issues) != 1 {
		t.Errorf("expected 1 todo issue, got %d", len(issues))
	}

	if len(issues) > 0 && issues[0]["status"] != "todo" {
		t.Errorf("expected status 'todo', got %v", issues[0]["status"])
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

	if _, ok := schema["status"]; !ok {
		t.Error("schema missing 'status' field")
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

// TestCLI_IssueList_InvalidStatus tests error handling for invalid status
func TestCLI_IssueList_InvalidStatus(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	_, _, err := env.run("issue", "list", "--status", "invalid")
	if err == nil {
		t.Error("expected error for invalid status, got nil")
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
}

// TestCLI_PieceNew tests mp piece new outputs valid JSON
func TestCLI_PieceNew(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.createIssue("add-feature.md", "Add Feature", "todo")

	input := `{"issue_path":"issues/add-feature.md","skip_switch":true}`
	stdout, stderr, err := env.runWithStdin(input, "piece", "new")
	if err != nil {
		t.Fatalf("piece new failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceNew_WithName tests mp piece new --name outputs valid JSON
func TestCLI_PieceNew_WithName(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	stdout, stderr, err := env.run("piece", "new", "--name", "my-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece new --name failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceNew_Schema tests mp piece new --schema outputs valid JSON
func TestCLI_PieceNew_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("piece", "new", "--schema")
	if err != nil {
		t.Fatalf("piece new --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceList tests mp piece list --flat outputs valid JSON
func TestCLI_PieceList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	_, _, err := env.run("piece", "new", "--name", "test-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece new failed: %v", err)
	}

	stdout, _, err := env.run("piece", "list", "--flat")
	if err != nil {
		t.Fatalf("piece list failed: %v", err)
	}

	var pieces []map[string]any
	if err := json.Unmarshal([]byte(stdout), &pieces); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PieceSwitch_Schema tests mp piece switch --schema outputs valid JSON
func TestCLI_PieceSwitch_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("piece", "switch", "--schema")
	if err != nil {
		t.Fatalf("piece switch --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
}

// TestCLI_PRCreate_Schema tests mp piece pr create --schema outputs valid JSON
func TestCLI_PRCreate_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("piece", "pr", "create", "--schema")
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
