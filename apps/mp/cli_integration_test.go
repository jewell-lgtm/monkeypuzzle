//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Build binary to temp location. Build the package that contains this test
	// file (apps/mp = package main) rather than relying on cwd/$PWD, which is
	// brittle under `go test`.
	binPath := filepath.Join(tmpDir, "mp")
	_, thisFile, _, _ := runtime.Caller(0)
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = filepath.Dir(thisFile)
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
	input := `{"name":"` + name + `","pr_provider":"github"}`
	stdout, stderr, err := e.runWithStdin(input, "init")
	if err != nil {
		e.t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

// TestCLI_Init tests mp init command
func TestCLI_Init(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Test init with JSON stdin
	input := `{"name":"testproject","pr_provider":"github"}`
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

// gitInDir runs a git command in dir, failing the test on error.
func (e *testEnv) gitInDir(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// addBareOrigin creates a bare repo, wires it as "origin", and pushes main to it
// so origin/main exists. Returns the bare repo path.
func (e *testEnv) addBareOrigin() string {
	e.t.Helper()
	bare := filepath.Join(e.tmpDir, "origin.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	if output, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git init --bare failed: %v\n%s", err, output)
	}
	e.gitInDir(e.tmpDir, "remote", "add", "origin", bare)
	e.gitInDir(e.tmpDir, "push", "origin", "main")
	return bare
}

// branchContainsCommit reports whether the branch checked out in worktreeDir
// contains commitSHA in its history.
func (e *testEnv) branchContainsCommit(worktreeDir, commitSHA string) bool {
	e.t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", commitSHA, "HEAD")
	cmd.Dir = worktreeDir
	return cmd.Run() == nil
}

// TestCLI_PieceCreate_BranchesOffOriginMain verifies that a top-level piece is
// based on the trunk's remote tip (origin/main) — not whatever branch is
// checked out in the repo root — and falls back to the local trunk with no
// remote. Regression test for create branching off current HEAD.
func TestCLI_PieceCreate_BranchesOffOriginMain(t *testing.T) {
	tests := []struct {
		name      string
		useRemote bool
	}{
		{name: "with origin remote", useRemote: true},
		{name: "local-only fallback", useRemote: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()

			env.initGitRepo()
			env.initProject("test")

			if tt.useRemote {
				env.addBareOrigin()
			}

			// main's tip after init/push.
			mainTip := env.gitInDir(env.tmpDir, "rev-parse", "HEAD")

			// Create and check out a divergent non-main branch with its own commit.
			env.gitInDir(env.tmpDir, "checkout", "-b", "homebrew")
			if err := os.WriteFile(filepath.Join(env.tmpDir, "brew.txt"), []byte("brew\n"), 0o644); err != nil {
				t.Fatalf("write brew.txt: %v", err)
			}
			env.gitInDir(env.tmpDir, "add", "brew.txt")
			env.gitInDir(env.tmpDir, "commit", "-m", "homebrew commit")
			homebrewTip := env.gitInDir(env.tmpDir, "rev-parse", "HEAD")

			// Create a top-level piece while homebrew is checked out.
			stdout, stderr, err := env.run("create", "--name", "my-piece", "--skip-switch")
			if err != nil {
				t.Fatalf("create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
			}

			var result struct {
				WorktreePath string `json:"worktree_path"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
			}
			if result.WorktreePath == "" {
				t.Fatalf("no worktree_path in output: %s", stdout)
			}

			// The new piece must NOT contain homebrew's commit...
			if env.branchContainsCommit(result.WorktreePath, homebrewTip) {
				t.Errorf("piece branch incorrectly contains homebrew commit %s (branched off current HEAD)", homebrewTip)
			}
			// ...and MUST contain main's tip.
			if !env.branchContainsCommit(result.WorktreePath, mainTip) {
				t.Errorf("piece branch missing main tip %s", mainTip)
			}
		})
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

	input := `{"name":"test","pr_provider":"github","create_skill":true}`
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

	input := `{"name":"test","pr_provider":"github","create_skill":false}`
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

// initGitRepoMaster initializes a git repo whose trunk is "master" (not "main"),
// to exercise commands that must honor a caller-supplied main_branch.
func (e *testEnv) initGitRepoMaster() {
	e.t.Helper()
	e.initGitRepo()
	// initGitRepo normalizes to "main"; rename the trunk to "master" and ensure
	// no "main" branch lingers, so any code defaulting to "main" would break.
	e.gitInDir(e.tmpDir, "branch", "-M", "master")
}

// TestCLI_Update_HonorsStdinMainBranch is a regression test for the agent
// contract: on a master-trunk repo, `echo '{"main_branch":"master"}' | mp update`
// must use "master" and not silently fall back to the flag's "main" default
// (which would fail with a merge-base error against a non-existent main).
func TestCLI_Update_HonorsStdinMainBranch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepoMaster()
	env.initProject("test")

	// Sanity: there is no "main" branch — only "master".
	branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "main") {
		t.Fatalf("expected master-only trunk, got branches: %q", branches)
	}

	stdout, stderr, err := env.run("create", "--name", "feat", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktreePath := created["worktree_path"].(string)

	// Honoring stdin main_branch="master" merges master (a no-op) and succeeds.
	stdout, stderr, err = env.runInDirWithStdin(worktreePath, `{"main_branch":"master"}`, "update")
	if err != nil {
		t.Fatalf("update with stdin main_branch=master failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "merge-base") || strings.Contains(stdout, "merge-base") {
		t.Errorf("update should not hit a merge-base error against main\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

// TestCLI_Merge_HonorsStdinMainBranch is the agent-contract regression test for
// `mp merge`: on a master-trunk repo, `echo '{"main_branch":"master"}' | mp merge`
// must squash-merge into "master" and not silently fall back to the flag's "main"
// default (which would fail to find a merge-base against a non-existent main).
func TestCLI_Merge_HonorsStdinMainBranch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepoMaster()
	env.initProject("test")

	// Sanity: there is no "main" branch — only "master".
	branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "main") {
		t.Fatalf("expected master-only trunk, got branches: %q", branches)
	}

	stdout, stderr, err := env.run("create", "--name", "feat", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktreePath := created["worktree_path"].(string)

	// Give the piece a commit so there is something to squash-merge.
	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("work\n"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}
	env.gitInDir(worktreePath, "add", "-A")
	env.gitInDir(worktreePath, "commit", "-m", "add feature")

	// Honoring stdin main_branch="master" squash-merges into master and succeeds.
	stdout, stderr, err = env.runInDirWithStdin(worktreePath, `{"main_branch":"master"}`, "merge")
	if err != nil {
		t.Fatalf("merge with stdin main_branch=master failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "merge-base") || strings.Contains(stdout, "merge-base") {
		t.Errorf("merge should not hit a merge-base error against main\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

// TestCLI_Done_HonorsStdinMainBranch is the agent-contract regression test for
// `mp done`: on a master-trunk repo, `echo '{"main_branch":"master"}' | mp done`
// must verify the merge against "master" and not silently fall back to the flag's
// "main" default (which would report the piece unmerged and refuse to clean up).
func TestCLI_Done_HonorsStdinMainBranch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepoMaster()
	env.initProject("test")

	// Sanity: there is no "main" branch — only "master".
	branches := env.gitInDir(env.tmpDir, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "main") {
		t.Fatalf("expected master-only trunk, got branches: %q", branches)
	}

	stdout, stderr, err := env.run("create", "--name", "feat", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktreePath := created["worktree_path"].(string)

	// Simulate a merge into the master trunk (mirrors TestCLI_PieceDone_FromMergedPiece).
	cmd := exec.Command("git", "merge", "feat", "--no-edit")
	cmd.Dir = env.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge failed: %v\n%s", err, output)
	}

	// Honoring stdin main_branch="master" detects the merge and cleans up.
	stdout, stderr, err = env.runInDirWithStdin(worktreePath, `{"main_branch":"master"}`, "done")
	if err != nil {
		t.Fatalf("done with stdin main_branch=master failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "merge-base") || strings.Contains(stdout, "merge-base") {
		t.Errorf("done should not hit a merge-base error against main\nstdout: %s\nstderr: %s", stdout, stderr)
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

// TestCLI_PieceAbandon_NotInPiece verifies abandon fails loudly (rather than
// prompting or guessing) when run with no --name outside any piece. Abandon only
// ever targets the current piece, so there is nothing to detect here.
func TestCLI_PieceAbandon_NotInPiece(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Run from the main repo root (not inside any piece), no --name.
	stdout, stderr, err := env.run("abandon")
	if err == nil {
		t.Fatalf("expected abandon to fail when not in a piece\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "not inside a piece") {
		t.Errorf("expected 'not inside a piece' error, got stderr: %s", stderr)
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

// TestCLI_Cleanup_StdinDryRun is a regression test for a bug where
// `echo '{"dry_run":true}' | mp cleanup` ignored the stdin dry_run and
// destructively removed merged pieces. A dry-run must never mutate.
func TestCLI_Cleanup_StdinDryRun(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	// Create a piece.
	stdout, stderr, err := env.run("create", "--name", "merged-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	worktree := created["worktree_path"].(string)

	// Simulate merge: merge the piece branch into main so cleanup detects it.
	cmd := exec.Command("git", "merge", "merged-piece", "--no-edit")
	cmd.Dir = env.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge failed: %v\n%s", err, output)
	}

	// Dry-run via stdin must only preview, never delete.
	stdout, stderr, err = env.runWithStdin(`{"dry_run":true}`, "cleanup")
	if err != nil {
		t.Fatalf("cleanup dry-run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		CleanedPieces []struct {
			PieceName string `json:"piece_name"`
		} `json:"cleaned_pieces"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}

	// The merged piece should be reported as a dry-run preview...
	found := false
	for _, p := range result.CleanedPieces {
		if p.PieceName == "merged-piece" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected merged-piece in dry-run preview, got %+v", result.CleanedPieces)
	}
	if !strings.Contains(stderr, "[dry-run]") {
		t.Errorf("expected [dry-run] notice on stderr, got: %s", stderr)
	}

	// ...but the worktree must still exist.
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree should still exist after dry-run: %v", err)
	}
}

// setupMergedPiece creates a piece, merges its branch into main, and returns its
// worktree path — the fixture shared by the cleanup default/apply tests. It first
// commits the mp config so piece worktrees are clean (as in a real repo), letting
// `--apply` remove them without --force.
func setupMergedPiece(t *testing.T, env *testEnv, name string) string {
	t.Helper()
	commit := exec.Command("git", "add", "-A")
	commit.Dir = env.tmpDir
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git add config failed: %v\n%s", err, out)
	}
	commit = exec.Command("git", "commit", "-m", "track mp config")
	commit.Dir = env.tmpDir
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit config failed: %v\n%s", err, out)
	}

	stdout, stderr, err := env.run("create", "--name", name, "--skip-switch")
	if err != nil {
		t.Fatalf("piece create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from piece create: %v", err)
	}
	cmd := exec.Command("git", "merge", name, "--no-edit")
	cmd.Dir = env.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge failed: %v\n%s", err, output)
	}
	return created["worktree_path"].(string)
}

// TestCLI_Cleanup_DryRunByDefault verifies that a non-interactive `mp repair`
// (no --apply) previews merged pieces without removing them.
func TestCLI_Cleanup_DryRunByDefault(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	worktree := setupMergedPiece(t, env, "merged-piece")

	// "repair" alias, no flags: must preview, never delete.
	stdout, stderr, err := env.run("repair")
	if err != nil {
		t.Fatalf("repair failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		CleanedPieces []struct {
			PieceName string `json:"piece_name"`
		} `json:"cleaned_pieces"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	found := false
	for _, p := range result.CleanedPieces {
		if p.PieceName == "merged-piece" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected merged-piece in dry-run preview, got %+v", result.CleanedPieces)
	}
	if !strings.Contains(stderr, "[dry-run]") {
		t.Errorf("expected [dry-run] notice on stderr, got: %s", stderr)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree should still exist after default dry-run: %v", err)
	}
}

// TestCLI_Cleanup_Apply verifies that --apply actually removes merged pieces.
func TestCLI_Cleanup_Apply(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	worktree := setupMergedPiece(t, env, "merged-piece")

	stdout, stderr, err := env.run("cleanup", "--apply")
	if err != nil {
		t.Fatalf("cleanup --apply failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result struct {
		CleanedPieces []struct {
			PieceName string `json:"piece_name"`
		} `json:"cleaned_pieces"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	found := false
	for _, p := range result.CleanedPieces {
		if p.PieceName == "merged-piece" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected merged-piece in cleaned list, got %+v", result.CleanedPieces)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree should be removed after --apply, stat err: %v", err)
	}
}

// TestCLI_Cleanup_AppliesViaForceAndStdin covers the two non-flag mutation entry
// points the dry-run-by-default redesign must keep working: the `--force`
// back-compat alias, and the agent-facing `{"apply":true}` over stdin. Both must
// actually remove the merged worktree, not just preview.
func TestCLI_Cleanup_AppliesViaForceAndStdin(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{name: "force flag alias", args: []string{"cleanup", "--force"}},
		{name: "apply over stdin", stdin: `{"apply":true}`, args: []string{"cleanup"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()

			env.initGitRepo()
			env.initProject("test")
			worktree := setupMergedPiece(t, env, "merged-piece")

			var stdout, stderr string
			var err error
			if tc.stdin != "" {
				stdout, stderr, err = env.runWithStdin(tc.stdin, tc.args...)
			} else {
				stdout, stderr, err = env.run(tc.args...)
			}
			if err != nil {
				t.Fatalf("cleanup failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
			}

			var result struct {
				CleanedPieces []struct {
					PieceName string `json:"piece_name"`
				} `json:"cleaned_pieces"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
			}
			found := false
			for _, p := range result.CleanedPieces {
				if p.PieceName == "merged-piece" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected merged-piece in cleaned list, got %+v", result.CleanedPieces)
			}
			if _, err := os.Stat(worktree); !os.IsNotExist(err) {
				t.Errorf("worktree should be removed, stat err: %v", err)
			}
		})
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

	// Use temp dir for MP_CONFIG_DIR to isolate config
	configHome := filepath.Join(env.tmpDir, "config")

	stdout, stderr, err := env.runWithEnv(map[string]string{"MP_CONFIG_DIR": configHome}, "config", "get", "multiplexer")
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
	stdout, stderr, err := env.runWithEnv(map[string]string{"MP_CONFIG_DIR": configHome}, "config", "set", "multiplexer", "tmux")
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
	stdout, stderr, err = env.runWithEnv(map[string]string{"MP_CONFIG_DIR": configHome}, "config", "get", "multiplexer")
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

	_, _, err := env.runWithEnv(map[string]string{"MP_CONFIG_DIR": configHome}, "config", "set", "unknown_key", "value")
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

// TestCLI_ConfigSet_InvalidValue tests mp config set with invalid value
func TestCLI_ConfigSet_InvalidValue(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	configHome := filepath.Join(env.tmpDir, "config")

	_, _, err := env.runWithEnv(map[string]string{"MP_CONFIG_DIR": configHome}, "config", "set", "multiplexer", "invalid")
	if err == nil {
		t.Error("expected error for invalid multiplexer value, got nil")
	}
}

// TestCLI_Version ensures `mp --version` reports a non-empty version so beta
// users can verify installs and report versions in bugs.
func TestCLI_Version(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	out := strings.TrimSpace(stdout)
	if out == "" {
		t.Fatal("--version printed nothing")
	}
	if !strings.Contains(out, "mp") {
		t.Errorf("expected version output to mention mp, got %q", out)
	}
}

// createPieceForSync creates a piece (optionally parented) and returns its
// worktree path.
func (e *testEnv) createPieceForSync(name, parent string) string {
	e.t.Helper()
	args := []string{"create", "--name", name, "--skip-switch"}
	if parent != "" {
		args = append(args, "--parent", parent)
	}
	stdout, stderr, err := e.run(args...)
	if err != nil {
		e.t.Fatalf("piece create %s failed: %v\nstdout: %s\nstderr: %s", name, err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		e.t.Fatalf("invalid JSON from piece create: %v\noutput: %s", err, stdout)
	}
	return created["worktree_path"].(string)
}

// commitFile writes a file and commits it in dir, returning the commit SHA.
func (e *testEnv) commitFile(dir, name, content, msg string) string {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		e.t.Fatalf("failed to write %s: %v", name, err)
	}
	e.gitInDir(dir, "add", name)
	e.gitInDir(dir, "commit", "-m", msg)
	return e.gitInDir(dir, "rev-parse", "HEAD")
}

// TestCLI_Sync_PrefersOriginParent verifies `mp sync` merges origin's version of
// the parent piece, not the local branch: a commit that exists only on
// origin/<parent> must land in the child, even though the local parent branch
// doesn't have it.
func TestCLI_Sync_PrefersOriginParent(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.addBareOrigin()

	parentWT := env.createPieceForSync("parent-piece", "")
	env.commitFile(parentWT, "parent.txt", "v1", "parent work")
	env.gitInDir(parentWT, "push", "-u", "origin", "parent-piece")

	childWT := env.createPieceForSync("child-piece", "parent-piece")

	// Advance the parent on origin only: commit + push, then rewind the local
	// parent branch so the new commit exists solely on origin/parent-piece.
	originOnly := env.commitFile(parentWT, "parent.txt", "v2", "parent origin-only work")
	env.gitInDir(parentWT, "push", "origin", "parent-piece")
	env.gitInDir(parentWT, "reset", "--hard", "HEAD~1")

	stdout, stderr, err := env.runInDir(childWT, "sync")
	if err != nil {
		t.Fatalf("sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result["source"] != "origin" {
		t.Errorf("expected source \"origin\", got %v", result["source"])
	}
	if result["merged_ref"] != "origin/parent-piece" {
		t.Errorf("expected merged_ref \"origin/parent-piece\", got %v", result["merged_ref"])
	}
	if result["parent"] != "parent-piece" {
		t.Errorf("expected parent \"parent-piece\", got %v", result["parent"])
	}
	if !env.branchContainsCommit(childWT, originOnly) {
		t.Error("child should contain the origin-only parent commit after sync")
	}
}

// TestCLI_Sync_LocalFallbackWithoutRemote verifies `mp sync` falls back to the
// local parent branch when the parent isn't on origin (here: no origin at all).
func TestCLI_Sync_LocalFallbackWithoutRemote(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")

	parentWT := env.createPieceForSync("parent-piece", "")
	childWT := env.createPieceForSync("child-piece", "parent-piece")
	localCommit := env.commitFile(parentWT, "parent.txt", "v1", "parent local work")

	stdout, stderr, err := env.runInDir(childWT, "sync")
	if err != nil {
		t.Fatalf("sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result["source"] != "local" {
		t.Errorf("expected source \"local\", got %v", result["source"])
	}
	if result["merged_ref"] != "parent-piece" {
		t.Errorf("expected merged_ref \"parent-piece\", got %v", result["merged_ref"])
	}
	if !env.branchContainsCommit(childWT, localCommit) {
		t.Error("child should contain the local parent commit after sync")
	}
}

// TestCLI_Sync_RootPieceUsesOriginMain verifies a root piece (parent=main) syncs
// from origin/main by default: a commit only on origin/main lands in the piece
// even though local main was rewound.
func TestCLI_Sync_RootPieceUsesOriginMain(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.addBareOrigin()

	pieceWT := env.createPieceForSync("feat", "")

	// Advance main on origin only.
	originOnly := env.commitFile(env.tmpDir, "trunk.txt", "v1", "trunk origin-only work")
	env.gitInDir(env.tmpDir, "push", "origin", "main")
	env.gitInDir(env.tmpDir, "reset", "--hard", "HEAD~1")

	stdout, stderr, err := env.runInDir(pieceWT, "sync")
	if err != nil {
		t.Fatalf("sync failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result["source"] != "origin" {
		t.Errorf("expected source \"origin\", got %v", result["source"])
	}
	if result["merged_ref"] != "origin/main" {
		t.Errorf("expected merged_ref \"origin/main\", got %v", result["merged_ref"])
	}
	if !env.branchContainsCommit(pieceWT, originOnly) {
		t.Error("piece should contain the origin-only main commit after sync")
	}
}

// TestCLI_Sync_LocalFlagSkipsOrigin verifies --local merges the local parent
// branch even when origin has a newer version.
func TestCLI_Sync_LocalFlagSkipsOrigin(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("test")
	env.addBareOrigin()

	parentWT := env.createPieceForSync("parent-piece", "")
	env.commitFile(parentWT, "parent.txt", "v1", "parent work")
	env.gitInDir(parentWT, "push", "-u", "origin", "parent-piece")

	childWT := env.createPieceForSync("child-piece", "parent-piece")

	originOnly := env.commitFile(parentWT, "parent.txt", "v2", "parent origin-only work")
	env.gitInDir(parentWT, "push", "origin", "parent-piece")
	env.gitInDir(parentWT, "reset", "--hard", "HEAD~1")

	stdout, stderr, err := env.runInDir(childWT, "sync", "--local")
	if err != nil {
		t.Fatalf("sync --local failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	if result["source"] != "local" {
		t.Errorf("expected source \"local\", got %v", result["source"])
	}
	if env.branchContainsCommit(childWT, originOnly) {
		t.Error("--local should not pull in origin-only parent commits")
	}
}

// TestCLI_Sync_Schema tests mp sync --schema.
func TestCLI_Sync_Schema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	stdout, _, err := env.run("sync", "--schema")
	if err != nil {
		t.Fatalf("sync --schema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v\noutput: %s", err, stdout)
	}
	for _, field := range []string{"main_branch", "from", "local"} {
		if _, ok := schema[field]; !ok {
			t.Errorf("schema missing %q field", field)
		}
	}
}
