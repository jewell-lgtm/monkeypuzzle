package projectdir_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(paths.EnvConfigDir, dir)
	return dir
}

func TestRelDir_DefaultWhenNoMapping(t *testing.T) {
	isolateConfig(t)
	if got := projectdir.RelDir("/some/repo"); got != projectdir.DefaultDirName {
		t.Errorf("RelDir = %q, want %q", got, projectdir.DefaultDirName)
	}
	if got := projectdir.RelDir(""); got != projectdir.DefaultDirName {
		t.Errorf("RelDir(\"\") = %q, want %q", got, projectdir.DefaultDirName)
	}
}

func TestSetAndResolve(t *testing.T) {
	isolateConfig(t)
	repo := "/work/myrepo"

	if err := projectdir.Set(repo, ".DONOTCOMMIT/monkeypuzzle"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := projectdir.RelDir(repo); got != ".DONOTCOMMIT/monkeypuzzle" {
		t.Errorf("RelDir = %q, want .DONOTCOMMIT/monkeypuzzle", got)
	}
	if got := projectdir.Dir(repo); got != filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle") {
		t.Errorf("Dir = %q", got)
	}
	pd, err := projectdir.PiecesDir(repo)
	if err != nil {
		t.Fatalf("PiecesDir: %v", err)
	}
	if pd != filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle", "pieces") {
		t.Errorf("PiecesDir = %q", pd)
	}
	if got := projectdir.HooksDir(repo); got != filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle", "hooks") {
		t.Errorf("HooksDir = %q", got)
	}
	if got := projectdir.ConfigFilePath(repo); got != filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle", "monkeypuzzle.json") {
		t.Errorf("ConfigFilePath = %q", got)
	}

	// Other repos are unaffected.
	if got := projectdir.RelDir("/work/other"); got != projectdir.DefaultDirName {
		t.Errorf("unrelated repo RelDir = %q, want default", got)
	}
}

func TestSet_DefaultRemovesEntry(t *testing.T) {
	isolateConfig(t)
	repo := "/r"
	if err := projectdir.Set(repo, ".mpstate"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if projectdir.RelDir(repo) != ".mpstate" {
		t.Fatal("expected .mpstate")
	}
	if err := projectdir.Set(repo, projectdir.DefaultDirName); err != nil {
		t.Fatalf("Set default: %v", err)
	}
	if got := projectdir.RelDir(repo); got != projectdir.DefaultDirName {
		t.Errorf("after reset to default, RelDir = %q", got)
	}
}

func TestSet_RejectsUnsafePaths(t *testing.T) {
	isolateConfig(t)
	for _, bad := range []string{"/abs/path", "../escape", "a/../../b", ""} {
		if err := projectdir.Set("/r", bad); err == nil {
			t.Errorf("Set(%q) should have failed", bad)
		}
	}
}

func TestRelDir_CorruptFileFallsBackToDefault(t *testing.T) {
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, "project-dirs.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectdir.RelDir("/r"); got != projectdir.DefaultDirName {
		t.Errorf("RelDir with corrupt file = %q, want default", got)
	}
}

func TestValidRel(t *testing.T) {
	good := []string{".monkeypuzzle", ".DONOTCOMMIT/monkeypuzzle", "a/b/c"}
	bad := []string{"", "/abs", "../x", "a/../../b"}
	for _, g := range good {
		if !projectdir.ValidRel(g) {
			t.Errorf("ValidRel(%q) = false, want true", g)
		}
	}
	for _, b := range bad {
		if projectdir.ValidRel(b) {
			t.Errorf("ValidRel(%q) = true, want false", b)
		}
	}
}

func TestMainRepoRootAndWorktreeDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	isolateConfig(t)

	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "T")
	run("commit", "--allow-empty", "-qm", "init")

	// Resolve symlinks so comparisons match (macOS /tmp etc.).
	repoResolved, _ := filepath.EvalSymlinks(repo)

	got, err := projectdir.MainRepoRoot(repo)
	if err != nil {
		t.Fatalf("MainRepoRoot(repo): %v", err)
	}
	if got != repoResolved {
		t.Errorf("MainRepoRoot(repo) = %q, want %q", got, repoResolved)
	}

	// From inside a linked worktree, MainRepoRoot still points at the main repo.
	wt := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-q", wt)
	got, err = projectdir.MainRepoRoot(wt)
	if err != nil {
		t.Fatalf("MainRepoRoot(worktree): %v", err)
	}
	if got != repoResolved {
		t.Errorf("MainRepoRoot(worktree) = %q, want %q", got, repoResolved)
	}

	// WorktreeDir mirrors the main repo's relocation.
	if err := projectdir.Set(repoResolved, ".DONOTCOMMIT/monkeypuzzle"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	wd, err := projectdir.WorktreeDir(wt)
	if err != nil {
		t.Fatalf("WorktreeDir: %v", err)
	}
	if wd != filepath.Join(wt, ".DONOTCOMMIT", "monkeypuzzle") {
		t.Errorf("WorktreeDir = %q", wd)
	}

	// Non-git path falls back to <path>/.monkeypuzzle.
	bare := t.TempDir()
	wd, err = projectdir.WorktreeDir(bare)
	if err != nil {
		t.Fatalf("WorktreeDir(non-git): %v", err)
	}
	if wd != filepath.Join(bare, projectdir.DefaultDirName) {
		t.Errorf("WorktreeDir(non-git) = %q", wd)
	}
}
