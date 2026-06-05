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

// projectTestRepo creates a git repo at <root>/<name>, runs `mp init`, and
// returns its path.
func projectTestRepo(t *testing.T, e *testEnv, dataDir, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "init")
	gitCmd(t, dir, "branch", "-M", "main")

	cmd := exec.Command(e.binPath, "init", "--name", name, "--issue-provider", "markdown", "--pr-provider", "github")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mp init in %s: %v\n%s", dir, err, out)
	}
	// Commit the monkeypuzzle config (incl. .monkeypuzzle/.gitignore) like a real
	// user would, so piece-metadata.json etc. stay ignored inside worktrees.
	gitCmd(t, dir, "add", ".monkeypuzzle")
	gitCmd(t, dir, "commit", "-m", "chore: add monkeypuzzle")
	return dir
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func mpJSON(t *testing.T, e *testEnv, dir, dataDir string, args ...string) ([]byte, string) {
	t.Helper()
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("mp %v: %v\nstderr: %s", args, err, stderr)
	}
	return out, ""
}

func mpRun(t *testing.T, e *testEnv, dir, dataDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mp %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func TestPieceAndIssueListAll(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	reposDir := filepath.Join(e.tmpDir, "repos")
	repoA := projectTestRepo(t, e, dataDir, reposDir, "alpha")
	repoB := projectTestRepo(t, e, dataDir, reposDir, "bravo")

	// A piece in alpha, an issue in bravo.
	mpRun(t, e, repoA, dataDir, "create", "--name", "feature-x", "--skip-switch")
	mpRun(t, e, repoB, dataDir, "issue", "create", "--title", "Fix the thing")

	// piece list --all
	out, _ := mpJSON(t, e, e.tmpDir, dataDir, "list", "--all")
	var pieceResult struct {
		Projects []struct {
			Name   string `json:"name"`
			Pieces []struct {
				Name string `json:"name"`
			} `json:"pieces"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &pieceResult); err != nil {
		t.Fatalf("unmarshal piece list --all: %v\n%s", err, out)
	}
	found := false
	for _, p := range pieceResult.Projects {
		if p.Name == "alpha" {
			for _, pc := range p.Pieces {
				if pc.Name == "feature-x" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected alpha/feature-x in piece list --all, got: %s", out)
	}

	// issue list --all
	out, _ = mpJSON(t, e, e.tmpDir, dataDir, "issue", "list", "--all")
	var issueResult struct {
		Projects []struct {
			Name   string `json:"name"`
			Issues []struct {
				Title string `json:"title"`
			} `json:"issues"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &issueResult); err != nil {
		t.Fatalf("unmarshal issue list --all: %v\n%s", err, out)
	}
	found = false
	for _, p := range issueResult.Projects {
		if p.Name == "bravo" {
			for _, is := range p.Issues {
				if is.Title == "Fix the thing" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected bravo issue 'Fix the thing' in issue list --all, got: %s", out)
	}
}

func TestDashboardAndSwitch(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	reposDir := filepath.Join(e.tmpDir, "repos")
	repoA := projectTestRepo(t, e, dataDir, reposDir, "alpha")
	mpRun(t, e, repoA, dataDir, "create", "--name", "feature-x", "--skip-switch")

	// Bare `mp` (non-TTY) prints the dashboard as JSON.
	out, _ := mpJSON(t, e, e.tmpDir, dataDir)
	var dash struct {
		Projects []struct {
			Name        string `json:"name"`
			MainSession string `json:"main_session"`
			Pieces      []struct {
				Name        string `json:"name"`
				SessionName string `json:"session_name"`
			} `json:"pieces"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &dash); err != nil {
		t.Fatalf("unmarshal dashboard: %v\n%s", err, out)
	}
	if len(dash.Projects) != 1 || dash.Projects[0].Name != "alpha" {
		t.Fatalf("expected one project 'alpha', got: %s", out)
	}
	if dash.Projects[0].MainSession != "mp/alpha" {
		t.Errorf("main_session = %q, want mp/alpha", dash.Projects[0].MainSession)
	}
	if len(dash.Projects[0].Pieces) != 1 || dash.Projects[0].Pieces[0].SessionName != "mp/alpha/feature-x" {
		t.Errorf("unexpected pieces: %+v", dash.Projects[0].Pieces)
	}

	// `mp switch --project alpha --piece feature-x` with no multiplexer prints the
	// worktree path.
	cmd := exec.Command(e.binPath, "switch", "--project", "alpha", "--piece", "feature-x")
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("mp switch: %v", err)
	}
	if !strings.Contains(string(stdout), "feature-x") {
		t.Errorf("mp switch output %q does not mention the worktree", string(stdout))
	}
	_ = repoA
}

func writeAndCommit(t *testing.T, dir, file, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", msg)
}

func TestMergeStackBaseReparentsChildren(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "stacked")

	// base piece off main, with a commit.
	mpRun(t, e, repo, dataDir, "create", "--name", "base", "--skip-switch")
	baseWt := filepath.Join(repo, ".monkeypuzzle", "pieces", "base")
	writeAndCommit(t, baseWt, "base.txt", "base change\n", "feat: base")

	// top piece stacked on base, with a commit.
	mpRun(t, e, repo, dataDir, "create", "--name", "top", "--parent", "base", "--skip-switch")
	topWt := filepath.Join(repo, ".monkeypuzzle", "pieces", "top")
	writeAndCommit(t, topWt, "top.txt", "top change\n", "feat: top")

	// Merge the BASE of the stack, re-homing children.
	out := mpRun(t, e, baseWt, dataDir, "merge", "--reparent-children")
	if !strings.Contains(out, "Re-homed top") {
		t.Errorf("expected merge output to mention re-homing top, got:\n%s", out)
	}

	// main now contains base's change.
	cmd := exec.Command("git", "-C", repo, "log", "--oneline", "main")
	logOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log main: %v\n%s", err, logOut)
	}
	if !strings.Contains(string(logOut), "base") {
		t.Errorf("expected main log to include the base squash commit, got:\n%s", logOut)
	}

	// top's metadata now points at main.
	metaBytes, err := os.ReadFile(filepath.Join(topWt, ".monkeypuzzle", "piece-metadata.json"))
	if err != nil {
		t.Fatalf("read top metadata: %v", err)
	}
	var meta struct {
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal top metadata: %v\n%s", err, metaBytes)
	}
	if meta.Parent != "main" {
		t.Errorf("top.parent = %q, want main", meta.Parent)
	}

	// top's branch should have its own commit but not base's individual commit
	// (base was squashed into main, and top was rebased off it).
	cmd = exec.Command("git", "-C", topWt, "log", "--oneline", "main..top")
	logOut, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log main..top: %v\n%s", err, logOut)
	}
	s := string(logOut)
	if !strings.Contains(s, "feat: top") {
		t.Errorf("expected top branch to contain 'feat: top', got:\n%s", s)
	}
	if strings.Contains(s, "feat: base") {
		t.Errorf("top branch should not still carry 'feat: base' after reparent, got:\n%s", s)
	}
}

func TestMergeStackBaseReparentMergeStrategy(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "stacked-merge")

	mpRun(t, e, repo, dataDir, "create", "--name", "base", "--skip-switch")
	baseWt := filepath.Join(repo, ".monkeypuzzle", "pieces", "base")
	writeAndCommit(t, baseWt, "base.txt", "base change\n", "feat: base")

	mpRun(t, e, repo, dataDir, "create", "--name", "top", "--parent", "base", "--skip-switch")
	topWt := filepath.Join(repo, ".monkeypuzzle", "pieces", "top")
	writeAndCommit(t, topWt, "top.txt", "top change\n", "feat: top")

	out := mpRun(t, e, baseWt, dataDir, "merge", "--reparent-strategy", "merge")
	if !strings.Contains(out, "Re-homed top onto main") {
		t.Errorf("expected re-home message, got:\n%s", out)
	}
	if strings.Contains(out, "force-push") {
		t.Errorf("merge strategy should not mention force-push, got:\n%s", out)
	}

	metaBytes, err := os.ReadFile(filepath.Join(topWt, ".monkeypuzzle", "piece-metadata.json"))
	if err != nil {
		t.Fatalf("read top metadata: %v", err)
	}
	var meta struct {
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if meta.Parent != "main" {
		t.Errorf("top.parent = %q, want main", meta.Parent)
	}

	// merge strategy keeps history: top still carries its 'feat: base' commit.
	cmd := exec.Command("git", "-C", topWt, "log", "--oneline", "main..top")
	logOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log main..top: %v\n%s", err, logOut)
	}
	if !strings.Contains(string(logOut), "feat: top") || !strings.Contains(string(logOut), "feat: base") {
		t.Errorf("expected top branch to contain both commits with merge strategy, got:\n%s", logOut)
	}
}

func TestProjectRegistry(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	reposDir := filepath.Join(e.tmpDir, "repos")

	repoA := projectTestRepo(t, e, dataDir, reposDir, "alpha")
	repoB := projectTestRepo(t, e, dataDir, reposDir, "bravo")

	// `mp init` should have auto-registered both.
	out, _ := mpJSON(t, e, e.tmpDir, dataDir, "project", "list", "--json")
	var projects []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(out, &projects); err != nil {
		t.Fatalf("unmarshal project list: %v\n%s", err, out)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d: %s", len(projects), out)
	}
	byName := map[string]string{}
	for _, p := range projects {
		byName[p.Name] = p.Path
	}
	if got := byName["alpha"]; got == "" {
		t.Errorf("alpha not registered: %v", byName)
	}
	if got := byName["bravo"]; got == "" {
		t.Errorf("bravo not registered: %v", byName)
	}
	// Paths must be the resolved repo roots.
	if resolved, _ := filepath.EvalSymlinks(repoA); byName["alpha"] != resolved {
		t.Errorf("alpha path = %q, want %q", byName["alpha"], resolved)
	}
	_ = repoB

	// Remove one and confirm.
	rmCmd := exec.Command(e.binPath, "project", "rm", "bravo")
	rmCmd.Dir = e.tmpDir
	rmCmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := rmCmd.CombinedOutput(); err != nil {
		t.Fatalf("project rm: %v\n%s", err, out)
	}
	out, _ = mpJSON(t, e, e.tmpDir, dataDir, "project", "list", "--json")
	projects = projects[:0]
	if err := json.Unmarshal(out, &projects); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "alpha" {
		t.Fatalf("after rm, expected only alpha, got: %s", out)
	}
}
