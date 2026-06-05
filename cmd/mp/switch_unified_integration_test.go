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

// TestSwitchUnified_FromIssue_CreatesPieceAndAttaches exercises the happy path
// for the unified picker's non-interactive form when given an issue: it should
// create a piece from the issue and print the new worktree path (since the test
// env has multiplexer=none).
func TestSwitchUnified_FromIssue_CreatesPieceAndAttaches(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	// `mp init` leaves a `.claude/` directory the test helper doesn't commit.
	// AdoptPiece refuses a dirty working tree, so stash it here.
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	// Seed an issue in alpha.
	mpRun(t, e, repo, dataDir, "issue", "create", "--title", "Wire the picker")

	// Switch by issue path (markdown provider).
	cmd := exec.Command(e.binPath,
		"switch", "--project", "alpha", "--issue", "issues/wire-the-picker.md",
	)
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mp switch --issue: %v\n%s", err, out)
	}

	// With multiplexer=none, attach prints the worktree path. The piece name is
	// derived from the issue title via SanitizePieceName.
	got := string(out)
	if !strings.Contains(got, "wire-the-picker") {
		t.Errorf("expected switch output to mention 'wire-the-picker', got: %q", got)
	}

	// The piece must now show up in `piece list --all`.
	listOut, _ := mpJSON(t, e, e.tmpDir, dataDir, "list", "--all")
	var listed struct {
		Projects []struct {
			Name   string `json:"name"`
			Pieces []struct {
				Name string `json:"name"`
			} `json:"pieces"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(listOut, &listed); err != nil {
		t.Fatalf("unmarshal piece list: %v\n%s", err, listOut)
	}
	found := false
	for _, p := range listed.Projects {
		if p.Name != "alpha" {
			continue
		}
		for _, pc := range p.Pieces {
			if strings.Contains(pc.Name, "wire-the-picker") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a piece derived from 'wire-the-picker' in piece list --all, got: %s", listOut)
	}
}

// TestSwitchUnified_FromBranch_AdoptsAndAttaches exercises the unified picker
// when given a pre-existing local branch: switch should adopt the branch as a
// piece and print its worktree path.
func TestSwitchUnified_FromBranch_AdoptsAndAttaches(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	// `mp init` leaves a `.claude/` directory the test helper doesn't commit.
	// AdoptPiece refuses a dirty working tree, so stash it here.
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	// Create a stray local branch from main, then switch back to main so the
	// adopt path doesn't have to handle "branch is currently checked out".
	gitCmd(t, repo, "branch", "stray-spike", "main")

	cmd := exec.Command(e.binPath,
		"switch", "--project", "alpha", "--branch", "stray-spike",
	)
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mp switch --branch: %v\n%s", err, out)
	}

	got := string(out)
	if !strings.Contains(got, "stray-spike") {
		t.Errorf("expected switch output to mention 'stray-spike', got: %q", got)
	}

	// The adopted piece must now show up.
	listOut, _ := mpJSON(t, e, e.tmpDir, dataDir, "list", "--all")
	var listed struct {
		Projects []struct {
			Name   string `json:"name"`
			Pieces []struct {
				Name string `json:"name"`
			} `json:"pieces"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(listOut, &listed); err != nil {
		t.Fatalf("unmarshal piece list: %v\n%s", err, listOut)
	}
	found := false
	for _, p := range listed.Projects {
		if p.Name != "alpha" {
			continue
		}
		for _, pc := range p.Pieces {
			if pc.Name == "stray-spike" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected adopted piece 'stray-spike' in piece list --all, got: %s", listOut)
	}
}

// TestSwitchUnified_DashJSON_IncludesIssuesAndBranches verifies the
// non-interactive JSON shape grows new arrays for issues and branches that the
// picker would surface.
func TestSwitchUnified_DashJSON_IncludesIssuesAndBranches(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	// `mp init` leaves a `.claude/` directory the test helper doesn't commit.
	// AdoptPiece refuses a dirty working tree, so stash it here.
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	mpRun(t, e, repo, dataDir, "issue", "create", "--title", "Open issue one")
	gitCmd(t, repo, "branch", "spike-branch", "main")

	out, _ := mpJSON(t, e, e.tmpDir, dataDir, "dash", "--json")
	var dash struct {
		Projects []struct {
			Name   string `json:"name"`
			Issues []struct {
				Title string `json:"title"`
				Path  string `json:"path"`
			} `json:"issues"`
			Branches []struct {
				Name string `json:"name"`
			} `json:"branches"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &dash); err != nil {
		t.Fatalf("unmarshal dash --json: %v\n%s", err, out)
	}

	if len(dash.Projects) == 0 {
		t.Fatalf("expected at least one project, got: %s", out)
	}
	p := dash.Projects[0]
	if p.Name != "alpha" {
		t.Fatalf("expected project 'alpha', got %q", p.Name)
	}

	foundIssue := false
	for _, is := range p.Issues {
		if is.Title == "Open issue one" {
			foundIssue = true
		}
	}
	if !foundIssue {
		t.Errorf("expected issue 'Open issue one' in dash --json, got: %s", out)
	}

	foundBranch := false
	for _, b := range p.Branches {
		if b.Name == "spike-branch" {
			foundBranch = true
		}
	}
	if !foundBranch {
		t.Errorf("expected branch 'spike-branch' in dash --json, got: %s", out)
	}
}

// TestDash_BareMpScopesToCurrentProject verifies the headline behaviour: bare
// `mp` run inside one registered project shows only that project (repo-local),
// while `mp dash` still shows every registered project.
func TestDash_BareMpScopesToCurrentProject(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repos := filepath.Join(e.tmpDir, "repos")
	alpha := projectTestRepo(t, e, dataDir, repos, "alpha")
	_ = projectTestRepo(t, e, dataDir, repos, "beta")

	// Bare `mp` from inside alpha: only alpha should appear.
	scoped := dashProjectNames(t, e, alpha, dataDir)
	if len(scoped) != 1 || scoped[0] != "alpha" {
		t.Errorf("bare mp in alpha should show only [alpha], got %v", scoped)
	}

	// `mp dash` from inside alpha: both projects should appear.
	all := dashProjectNames(t, e, alpha, dataDir, "dash")
	if !contains(all, "alpha") || !contains(all, "beta") {
		t.Errorf("mp dash should show both alpha and beta, got %v", all)
	}
}

// TestDash_BareMpFallsBackOutsideProject verifies bare `mp` from a directory
// that isn't a registered project falls back to the cross-project view.
func TestDash_BareMpFallsBackOutsideProject(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repos := filepath.Join(e.tmpDir, "repos")
	_ = projectTestRepo(t, e, dataDir, repos, "alpha")
	_ = projectTestRepo(t, e, dataDir, repos, "beta")

	// e.tmpDir is not a registered project: expect the full list.
	names := dashProjectNames(t, e, e.tmpDir, dataDir)
	if !contains(names, "alpha") || !contains(names, "beta") {
		t.Errorf("bare mp outside a project should fall back to all projects, got %v", names)
	}
}

// TestDash_RemoteOnlyBranchSurfaces verifies that a branch that exists only on
// a remote (origin) appears in the dashboard branch list as a remote ref, so a
// user can create a piece from it.
func TestDash_RemoteOnlyBranchSurfaces(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repos := filepath.Join(e.tmpDir, "repos")

	// Build a bare "remote" with a feature branch, then clone it as the project.
	remote := filepath.Join(repos, "origin.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatalf("mkdir remote: %v", err)
	}
	seed := filepath.Join(e.tmpDir, "seed")
	gitCmd(t, e.tmpDir, "init", seed)
	gitCmd(t, seed, "commit", "--allow-empty", "-m", "init")
	gitCmd(t, seed, "branch", "-M", "main")
	gitCmd(t, seed, "checkout", "-b", "remote-spike")
	gitCmd(t, seed, "commit", "--allow-empty", "-m", "spike work")
	gitCmd(t, seed, "checkout", "main")
	gitCmd(t, e.tmpDir, "clone", "--bare", seed, remote)

	// Clone the bare remote into the project location and `mp init` it.
	gamma := filepath.Join(repos, "gamma")
	gitCmd(t, e.tmpDir, "clone", remote, gamma)
	gitCmd(t, gamma, "checkout", "main")
	cmd := exec.Command(e.binPath, "init", "--name", "gamma", "--issue-provider", "markdown", "--pr-provider", "github")
	cmd.Dir = gamma
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mp init in gamma: %v\n%s", err, out)
	}

	out, _ := mpJSON(t, e, gamma, dataDir, "dash", "--json")
	var dash struct {
		Projects []struct {
			Name     string `json:"name"`
			Branches []struct {
				Name   string `json:"name"`
				Remote bool   `json:"remote"`
			} `json:"branches"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &dash); err != nil {
		t.Fatalf("unmarshal dash --json: %v\n%s", err, out)
	}

	foundRemote := false
	for _, p := range dash.Projects {
		if p.Name != "gamma" {
			continue
		}
		for _, b := range p.Branches {
			if b.Name == "origin/remote-spike" && b.Remote {
				foundRemote = true
			}
		}
	}
	if !foundRemote {
		t.Errorf("expected remote-only branch 'origin/remote-spike' (remote=true) in dash --json, got: %s", out)
	}
}

// TestPiece_StatusCommand verifies `mp status` returns the status JSON.
func TestPiece_StatusCommand(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repos := filepath.Join(e.tmpDir, "repos")
	alpha := projectTestRepo(t, e, dataDir, repos, "alpha")

	// `mp status` should emit JSON with in_piece on stdout.
	statusOut := mpRun(t, e, alpha, dataDir, "status")
	if !strings.Contains(statusOut, "in_piece") {
		t.Errorf("expected `mp status` to emit status JSON, got: %q", statusOut)
	}
}

// dashProjectNames runs the dashboard (bare `mp` by default, or the given args
// like "dash") in dir with JSON output and returns the project names.
func dashProjectNames(t *testing.T, e *testEnv, dir, dataDir string, args ...string) []string {
	t.Helper()
	if len(args) == 0 {
		args = []string{"--json"}
	} else {
		args = append(args, "--json")
	}
	out, _ := mpJSON(t, e, dir, dataDir, args...)
	var dash struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(out, &dash); err != nil {
		t.Fatalf("unmarshal dash --json: %v\n%s", err, out)
	}
	names := make([]string, 0, len(dash.Projects))
	for _, p := range dash.Projects {
		names = append(names, p.Name)
	}
	return names
}

// TestSwitchUnified_MutuallyExclusiveSelectors verifies that --piece, --issue,
// and --branch cannot be combined.
func TestSwitchUnified_MutuallyExclusiveSelectors(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	_ = projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath,
		"switch", "--project", "alpha",
		"--issue", "issues/x.md",
		"--branch", "y",
	)
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when combining --issue and --branch, got success\n%s", out)
	}
	if !strings.Contains(string(out), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %s", out)
	}
}
