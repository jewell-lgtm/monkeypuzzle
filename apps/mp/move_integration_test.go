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

func TestMoveRelocatesMonkeypuzzleDir(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	configDir := filepath.Join(e.tmpDir, "config")
	repo := filepath.Join(e.tmpDir, "repos", "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"multiplexer":"none"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mpEnv := append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+configDir)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mp := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(e.binPath, args...)
		cmd.Dir = dir
		cmd.Env = mpEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("mp %v in %s: %v\n%s", args, dir, err, out)
		}
		return string(out)
	}

	git("init")
	git("commit", "--allow-empty", "-m", "init")
	git("branch", "-M", "main")

	mp(repo, "init", "--name", "demo", "--pr-provider", "github")
	mp(repo, "create", "--name", "feat", "--skip-switch")

	mustDir := func(p string) {
		t.Helper()
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Fatalf("expected directory %s (err=%v)", p, err)
		}
	}
	mustFile := func(p string) {
		t.Helper()
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			t.Fatalf("expected file %s (err=%v)", p, err)
		}
	}
	mustGone := func(p string) {
		t.Helper()
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be gone (err=%v)", p, err)
		}
	}

	// Sanity: default layout before the move.
	mustFile(filepath.Join(repo, ".monkeypuzzle", "monkeypuzzle.json"))
	mustDir(filepath.Join(repo, ".monkeypuzzle", "pieces", "feat"))
	mustFile(filepath.Join(repo, ".monkeypuzzle", "pieces", "feat", ".monkeypuzzle", "piece-metadata.json"))

	// Move it.
	out := mp(repo, "move", ".DONOTCOMMIT/monkeypuzzle")
	t.Logf("mp move output:\n%s", out)

	mustGone(filepath.Join(repo, ".monkeypuzzle"))
	mustFile(filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle", "monkeypuzzle.json"))
	wt := filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle", "pieces", "feat")
	mustDir(wt)
	mustFile(filepath.Join(wt, ".DONOTCOMMIT", "monkeypuzzle", "piece-metadata.json"))

	// The relocated worktree is still a working git worktree.
	cmd := exec.Command("git", "-C", wt, "status", "--porcelain")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git status in relocated worktree failed: %v\n%s", err, out)
	}

	// Mapping recorded.
	mapData, err := os.ReadFile(filepath.Join(configDir, "project-dirs.json"))
	if err != nil {
		t.Fatalf("read project-dirs.json: %v", err)
	}
	var m struct {
		Dirs map[string]string `json:"dirs"`
	}
	if err := json.Unmarshal(mapData, &m); err != nil {
		t.Fatalf("unmarshal mapping: %v\n%s", err, mapData)
	}
	repoResolved, _ := filepath.EvalSymlinks(repo)
	if got := m.Dirs[repoResolved]; got != ".DONOTCOMMIT/monkeypuzzle" {
		t.Fatalf("mapping[%q] = %q, want .DONOTCOMMIT/monkeypuzzle (full: %v)", repoResolved, got, m.Dirs)
	}

	// mp still finds the piece via the relocated dir.
	if listOut := mp(repo, "list"); !strings.Contains(listOut, "feat") {
		t.Fatalf("piece 'feat' not found after move: %s", listOut)
	}

	// Move back to the default; the mapping entry should disappear.
	mp(repo, "move", ".monkeypuzzle")
	mustGone(filepath.Join(repo, ".DONOTCOMMIT", "monkeypuzzle"))
	mustFile(filepath.Join(repo, ".monkeypuzzle", "monkeypuzzle.json"))
	mustDir(filepath.Join(repo, ".monkeypuzzle", "pieces", "feat"))

	mapData, err = os.ReadFile(filepath.Join(configDir, "project-dirs.json"))
	if err != nil {
		t.Fatalf("read project-dirs.json after revert: %v", err)
	}
	var m2 struct {
		Dirs map[string]string `json:"dirs"`
	}
	if err := json.Unmarshal(mapData, &m2); err != nil {
		t.Fatalf("unmarshal mapping after revert: %v", err)
	}
	if _, ok := m2.Dirs[repoResolved]; ok {
		t.Fatalf("expected no mapping entry for %q after revert, got %v", repoResolved, m2.Dirs)
	}
}
