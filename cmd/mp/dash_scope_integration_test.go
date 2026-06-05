//go:build integration

package mp_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// dashProjectsJSON runs `mp <args>` in dir and decodes the dashboard's project
// list. Bare `mp` (no extra args beyond what callers pass) prints the dashboard
// as JSON when stdout is not a TTY.
func dashProjectsJSON(t *testing.T, e *testEnv, dir, dataDir string, args ...string) []string {
	t.Helper()
	out, _ := mpJSON(t, e, dir, dataDir, args...)
	var dash struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &dash); err != nil {
		t.Fatalf("unmarshal dashboard: %v\n%s", err, out)
	}
	names := make([]string, 0, len(dash.Projects))
	for _, p := range dash.Projects {
		names = append(names, p.Name)
	}
	return names
}

// TestDashboardScopedToCurrentRepo is the happy-path integration test for
// detecting the current project: bare `mp` inside a repo shows only that repo,
// `--all` shows everything, a piece worktree resolves to its owning repo, and
// running outside any registered repo falls back to showing all.
func TestDashboardScopedToCurrentRepo(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	reposDir := filepath.Join(e.tmpDir, "repos")
	repoA := projectTestRepo(t, e, dataDir, reposDir, "alpha")
	repoB := projectTestRepo(t, e, dataDir, reposDir, "bravo")
	mpRun(t, e, repoA, dataDir, "piece", "create", "--name", "feature-x", "--skip-switch")

	// Bare `mp` inside alpha shows only alpha.
	got := dashProjectsJSON(t, e, repoA, dataDir)
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("mp in alpha: expected only [alpha], got %v", got)
	}

	// Bare `mp` inside bravo shows only bravo.
	got = dashProjectsJSON(t, e, repoB, dataDir)
	if len(got) != 1 || got[0] != "bravo" {
		t.Errorf("mp in bravo: expected only [bravo], got %v", got)
	}

	// From inside a piece worktree of alpha, still scoped to alpha.
	worktree := filepath.Join(repoA, ".monkeypuzzle", "pieces", "feature-x")
	got = dashProjectsJSON(t, e, worktree, dataDir)
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("mp in alpha worktree: expected only [alpha], got %v", got)
	}

	// `mp --all` inside alpha shows every project.
	got = dashProjectsJSON(t, e, repoA, dataDir, "--all")
	if len(got) != 2 {
		t.Errorf("mp --all in alpha: expected both projects, got %v", got)
	}

	// Outside any registered repo, fall back to showing all.
	got = dashProjectsJSON(t, e, e.tmpDir, dataDir)
	if len(got) != 2 {
		t.Errorf("mp outside any repo: expected all projects, got %v", got)
	}
}
