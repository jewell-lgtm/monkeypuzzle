//go:build integration

package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shellAvailable reports whether a shell is installed, so the test skips
// rather than fails on a machine without fish.
func shellAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runShell runs a script in the named shell with mp on PATH and the test's
// data/config dirs, returning trimmed stdout.
func runShell(t *testing.T, e *testEnv, shell, dataDir, script string) string {
	t.Helper()
	cmd := exec.Command(shell, "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(e.binPath)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MP_DATA_DIR="+dataDir,
		"MP_CONFIG_DIR="+e.configDir,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s script failed: %v\nscript: %s\nstderr: %s", shell, err, script, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// TestShellInit_SyntaxIsValid parses each emitted wrapper with its own shell.
func TestShellInit_SyntaxIsValid(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	for _, sh := range []string{"bash", "zsh", "fish"} {
		if !shellAvailable(sh) {
			t.Logf("skipping %s: not installed", sh)
			continue
		}
		script := mpRun(t, e, e.tmpDir, filepath.Join(e.tmpDir, "data"), "shell-init", sh)
		file := filepath.Join(t.TempDir(), "init."+sh)
		if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
		var cmd *exec.Cmd
		if sh == "fish" {
			cmd = exec.Command(sh, "--no-execute", file)
		} else {
			cmd = exec.Command(sh, "-n", file)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s wrapper is not valid %s: %v\n%s", sh, sh, err, out)
		}
	}
}

// TestShellInit_FollowsSwitch is the point of the wrapper: after `mp switch`,
// the shell is standing in the piece worktree.
func TestShellInit_FollowsSwitch(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			if !shellAvailable(sh) {
				t.Skipf("%s not installed", sh)
			}
			e := setupTestEnv(t)
			defer e.cleanup()

			dataDir := filepath.Join(e.tmpDir, "data")
			repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
			mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
			wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

			script := fmt.Sprintf(`eval "$(mp shell-init %s)"; cd %s; mp switch fix-x >/dev/null; pwd`, sh, repo)
			if got := runShell(t, e, sh, dataDir, script); got != wt {
				t.Errorf("wrapper should leave the shell in %q, got %q", wt, got)
			}
		})
	}
}

// TestShellInit_FollowsDoneBackToRoot pins the strand fix through the wrapper:
// finishing the piece you stand in lands you back in the main repo.
func TestShellInit_FollowsDoneBackToRoot(t *testing.T) {
	if !shellAvailable("bash") {
		t.Skip("bash not installed")
	}
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	script := fmt.Sprintf(`eval "$(mp shell-init bash)"; cd %s; mp done --force >/dev/null 2>&1; pwd`, wt)
	if got := runShell(t, e, "bash", dataDir, script); got != repo {
		t.Errorf("after done the shell should be back in %q, got %q", repo, got)
	}
}

// TestShellInit_PreservesExitCodeAndStdout pins what the wrapper must not
// break: mp's exit status and its stdout (so `cd "$(mp switch x)"` and every
// `--json` pipe still work).
func TestShellInit_PreservesExitCodeAndStdout(t *testing.T) {
	if !shellAvailable("bash") {
		t.Skip("bash not installed")
	}
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	script := fmt.Sprintf(`eval "$(mp shell-init bash)"; cd %s; out="$(mp switch fix-x)"; echo "$out"`, repo)
	if got := runShell(t, e, "bash", dataDir, script); got != wt {
		t.Errorf("stdout through the wrapper should still be the path %q, got %q", wt, got)
	}

	failScript := fmt.Sprintf(`eval "$(mp shell-init bash)"; cd %s; mp switch no-such-piece >/dev/null 2>&1; echo $?`, repo)
	if got := runShell(t, e, "bash", dataDir, failScript); got == "0" {
		t.Error("wrapper should preserve a non-zero exit code from mp")
	}
}

// TestShellInit_WorksBeforeFirstRun pins that the rc-file line and the setup
// check work on a fresh machine: with no user config, shell-init and doctor
// succeed and write nothing, while ordinary verbs still hit the first-run gate.
func TestShellInit_WorksBeforeFirstRun(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	cfgFile := filepath.Join(e.configDir, "config.json")
	if err := os.Remove(cfgFile); err != nil {
		t.Fatalf("remove seeded config: %v", err)
	}

	for _, args := range [][]string{{"shell-init", "zsh"}, {"doctor", "--json"}} {
		stdout, stderr, err := e.run(args...)
		if err != nil {
			t.Errorf("mp %s with no config: %v\nstderr: %s", strings.Join(args, " "), err, stderr)
			continue
		}
		if strings.TrimSpace(stdout) == "" {
			t.Errorf("mp %s printed nothing on stdout", strings.Join(args, " "))
		}
	}
	if _, err := os.Stat(cfgFile); !os.IsNotExist(err) {
		t.Errorf("shell-init/doctor must not write the config, stat err: %v", err)
	}

	if _, stderr, err := e.run("list"); err == nil || !strings.Contains(stderr, "not configured") {
		t.Errorf("list should still require config, err=%v stderr=%s", err, stderr)
	}
}
