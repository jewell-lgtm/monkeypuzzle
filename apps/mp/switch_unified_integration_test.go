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

// TestSwitchUnified_FromBranch_AdoptsAndAttaches exercises the unified picker
// when given a pre-existing local branch: switch should adopt the branch as a
// piece and print its worktree path.
func TestSwitchUnified_FromBranch_AdoptsAndAttaches(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	// `mp init` leaves a `.claude/` directory the test helper doesn't commit.
	// Commit it so the main worktree starts clean and deterministic.
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

// TestSwitchUnified_DashJSON_IncludesBranches verifies the non-interactive JSON
// shape exposes the branches array that the picker would surface.
func TestSwitchUnified_DashJSON_IncludesBranches(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	// `mp init` leaves a `.claude/` directory the test helper doesn't commit.
	// Commit it so the main worktree starts clean and deterministic.
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	gitCmd(t, repo, "branch", "spike-branch", "main")

	out, _ := mpJSON(t, e, e.tmpDir, dataDir, "go", "--json")
	var dash struct {
		Projects []struct {
			Name     string `json:"name"`
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
// while `mp go` still shows every registered project.
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

	// `mp go` from inside alpha: both projects should appear.
	all := dashProjectNames(t, e, alpha, dataDir, "go")
	if !contains(all, "alpha") || !contains(all, "beta") {
		t.Errorf("mp go should show both alpha and beta, got %v", all)
	}
}

// TestGo_BareMpOutsideProjectGuides verifies bare `mp` outside a monkeypuzzle
// project prints context-aware guidance and then falls through to the
// cross-project view: an un-init'd git repo is pointed at `mp init`, a
// non-repo directory at cd-ing into one, and in both cases the structured
// output carries the registered projects (the `mp go` data) plus a loud
// in_project:false signal so nothing mistakes "not here" for "nowhere".
func TestGo_BareMpOutsideProjectGuides(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repos := filepath.Join(e.tmpDir, "repos")
	_ = projectTestRepo(t, e, dataDir, repos, "alpha")

	run := func(dir string) (string, error) {
		cmd := exec.Command(e.binPath)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	assertGuidedDashboard := func(out string, wantGuidance string) {
		t.Helper()
		if !strings.Contains(out, wantGuidance) {
			t.Errorf("bare mp guidance should contain %q, got: %s", wantGuidance, out)
		}
		if !strings.Contains(out, "\"in_project\": false") {
			t.Errorf("bare mp outside a project should emit in_project:false, got: %s", out)
		}
		// The cross-project detail must ride along so the caller can still jump.
		if !strings.Contains(out, "\"alpha\"") {
			t.Errorf("bare mp outside a project should fall through to the project list, got: %s", out)
		}
	}

	// A git repo that hasn't been `mp init`-ed: guide to `mp init`.
	plainRepo := filepath.Join(e.tmpDir, "plain")
	gitCmd(t, e.tmpDir, "init", plainRepo)
	repoOut, _ := run(plainRepo)
	assertGuidedDashboard(repoOut, "mp init")

	// A directory that is not a git repo at all: guide to cd-ing into one.
	nonRepo := filepath.Join(e.tmpDir, "elsewhere")
	if err := os.MkdirAll(nonRepo, 0o755); err != nil {
		t.Fatalf("mkdir nonRepo: %v", err)
	}
	nonOut, _ := run(nonRepo)
	assertGuidedDashboard(nonOut, "git repository")
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
	cmd := exec.Command(e.binPath, "init", "--name", "gamma", "--pr-provider", "github")
	cmd.Dir = gamma
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mp init in gamma: %v\n%s", err, out)
	}

	out, _ := mpJSON(t, e, gamma, dataDir, "go", "--json")
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
// like "go") in dir with JSON output and returns the project names.
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

// TestSwitchUnified_MutuallyExclusiveSelectors verifies that --piece and
// --branch cannot be combined.
func TestSwitchUnified_MutuallyExclusiveSelectors(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	_ = projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath,
		"switch", "--project", "alpha",
		"--piece", "x",
		"--branch", "y",
	)
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when combining --piece and --branch, got success\n%s", out)
	}
	if !strings.Contains(string(out), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %s", out)
	}
}

// TestSwitchUnified_TargetResolution exercises the positional-target surface
// end to end in one repo: an existing piece attaches by name AND by its
// checked-out branch (the reverse lookup), an unadopted branch adopts, a
// brand-new name is refused without --create and created with it (branch
// verbatim, piece name derived), and --branch on an already-adopted branch
// attaches instead of erroring (the idempotence regression fix).
func TestSwitchUnified_TargetResolution(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	// Adopt a slash-named branch: piece name gets the prefix stripped.
	gitCmd(t, repo, "branch", "feat/dark-mode", "main")
	out := mpRun(t, e, e.tmpDir, dataDir, "switch", "--project", "alpha", "feat/dark-mode")
	if !strings.Contains(out, "dark-mode") {
		t.Fatalf("adopting via target should mention the piece, got: %q", out)
	}

	// Same target again: must attach the existing piece, not error out of adopt.
	out = mpRun(t, e, e.tmpDir, dataDir, "switch", "--project", "alpha", "feat/dark-mode")
	if !strings.Contains(out, "dark-mode") {
		t.Errorf("re-switching by branch should attach the piece, got: %q", out)
	}
	// And by piece name.
	out = mpRun(t, e, e.tmpDir, dataDir, "switch", "--project", "alpha", "dark-mode")
	if !strings.Contains(out, "dark-mode") {
		t.Errorf("switching by piece name should attach, got: %q", out)
	}
	// Explicit --branch on the adopted branch: attach, not the old adopt error.
	out = mpRun(t, e, e.tmpDir, dataDir, "switch", "--project", "alpha", "--branch", "feat/dark-mode")
	if !strings.Contains(out, "dark-mode") {
		t.Errorf("--branch on an adopted branch should attach, got: %q", out)
	}

	// A brand-new name without --create: refuse with a hint (non-TTY).
	cmd := exec.Command(e.binPath, "switch", "--project", "alpha", "feat/brand-new")
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	rawOut, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for unmatched target without --create, got success\n%s", rawOut)
	}
	if !strings.Contains(string(rawOut), "--create") {
		t.Errorf("unmatched-target error should hint at --create, got: %s", rawOut)
	}

	// With --create: piece name derived, branch created verbatim.
	out = mpRun(t, e, e.tmpDir, dataDir, "switch", "--project", "alpha", "--create", "feat/brand-new")
	if !strings.Contains(out, "brand-new") {
		t.Fatalf("--create should mint the piece, got: %q", out)
	}
	branchOut := exec.Command("git", "-C", repo, "branch", "--list", "feat/brand-new")
	bo, _ := branchOut.CombinedOutput()
	if !strings.Contains(string(bo), "feat/brand-new") {
		t.Errorf("expected verbatim branch feat/brand-new to exist, got: %s", bo)
	}
}

// TestSwitchUnified_ProjectDefaultsToCwd verifies --project is optional inside
// a repo: both a piece selector and a positional target resolve against the
// project the caller is standing in, including from inside a piece worktree.
func TestSwitchUnified_ProjectDefaultsToCwd(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")

	// From the repo root, no --project.
	out := mpRun(t, e, repo, dataDir, "switch", "--piece", "fix-x")
	if !strings.Contains(out, "fix-x") {
		t.Errorf("switch --piece without --project should resolve from cwd, got: %q", out)
	}
	out = mpRun(t, e, repo, dataDir, "switch", "fix-x")
	if !strings.Contains(out, "fix-x") {
		t.Errorf("positional target without --project should resolve from cwd, got: %q", out)
	}
	// From inside the piece worktree itself (main repo root resolution).
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	out = mpRun(t, e, wt, dataDir, "switch", "fix-x")
	if !strings.Contains(out, "fix-x") {
		t.Errorf("target from inside a piece worktree should resolve, got: %q", out)
	}

	// Outside any repo with no --project: a clear error.
	cmd := exec.Command(e.binPath, "switch", "fix-x")
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	rawOut, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error outside a project without --project, got success\n%s", rawOut)
	}
	if !strings.Contains(string(rawOut), "--project") {
		t.Errorf("outside-project error should hint at --project, got: %s", rawOut)
	}
}
