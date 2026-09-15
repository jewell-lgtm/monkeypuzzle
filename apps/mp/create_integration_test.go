//go:build integration

package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreate_HelpNamesTheRealWorktreePath pins `mp create --help` to where
// create actually puts the worktree, so the help can't drift back to the old
// data-dir layout.
func TestCreate_HelpNamesTheRealWorktreePath(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	help := mpRun(t, e, repo, dataDir, "create", "--help")
	if !strings.Contains(help, "<repo>/.monkeypuzzle/pieces/<name>") {
		t.Errorf("create --help should name <repo>/.monkeypuzzle/pieces/<name>, got:\n%s", help)
	}
	if strings.Contains(help, "repo-hash") {
		t.Errorf("create --help still describes the old data-dir layout:\n%s", help)
	}

	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	if _, err := os.Stat(filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")); err != nil {
		t.Errorf("create should put the worktree where the help says: %v", err)
	}
}
