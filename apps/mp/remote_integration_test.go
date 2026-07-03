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

// sshShim installs a fake `ssh` first on PATH that records its argv and stdin
// to files and replays a canned response, so the remote proxy is tested
// without a live host. Returns the shim dir (argv/stdin/stdout files live in
// it) and the PATH value to run mp with.
func sshShim(t *testing.T, e *testEnv, stdout string, exitCode int) (shimDir, path string) {
	t.Helper()
	shimDir = filepath.Join(e.tmpDir, "ssh-shim")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shimDir, "stdout"), []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > %q
cat %q
exit %d
`, filepath.Join(shimDir, "argv"), filepath.Join(shimDir, "stdin"), filepath.Join(shimDir, "stdout"), exitCode)
	if err := os.WriteFile(filepath.Join(shimDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return shimDir, shimDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func shimFile(t *testing.T, shimDir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(shimDir, name))
	if err != nil {
		t.Fatalf("shim %s not recorded: %v", name, err)
	}
	return string(data)
}

// runProxy invokes mp with the shim PATH plus any extra env, preserving the
// data/config isolation from testEnv.env().
func runProxy(e *testEnv, path, stdin string, extraEnv map[string]string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir
	cmd.Env = append(e.env(), "PATH="+path)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestCLI_RemoteProxy_ForwardsOverSSH(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	canned := `{"name":"add-rate-limiting","worktree_path":"/home/u/api/.monkeypuzzle/pieces/add-rate-limiting"}`
	shimDir, path := sshShim(t, e, canned, 0)

	stdout, stderr, err := runProxy(e, path, "", nil,
		"--host", "wire", "--dir", "/home/u/api", "create", "--name", "add-rate-limiting", "--json")
	if err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if stdout != canned {
		t.Errorf("stdout = %q, want canned shim response %q", stdout, canned)
	}

	got := strings.Split(strings.TrimRight(shimFile(t, shimDir, "argv"), "\n"), "\n")
	wantLead := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "--", "wire"}
	if len(got) != len(wantLead)+1 {
		t.Fatalf("ssh argv = %#v, want %v + command", got, wantLead)
	}
	for i := range wantLead {
		if got[i] != wantLead[i] {
			t.Errorf("ssh argv[%d] = %q, want %q", i, got[i], wantLead[i])
		}
	}
	cmd := got[len(got)-1]
	if !strings.HasPrefix(cmd, "sh -c '") {
		t.Errorf("remote command not sh -c wrapped: %q", cmd)
	}
	for _, want := range []string{`$HOME/.local/bin`, `cd '\''/home/u/api'\''`, `exec '\''mp'\'' '\''create'\'' '\''--name'\'' '\''add-rate-limiting'\'' '\''--json'\''`} {
		if !strings.Contains(cmd, want) {
			t.Errorf("remote command missing %q\ngot: %q", want, cmd)
		}
	}
	// No local stdin data -> "{}" substituted so remote stdin-JSON mode parses.
	if stdin := shimFile(t, shimDir, "stdin"); stdin != "{}" {
		t.Errorf("forwarded stdin = %q, want {}", stdin)
	}
}

func TestCLI_RemoteProxy_StdinJSONPassthrough(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	shimDir, path := sshShim(t, e, "{}", 0)

	input := `{"name":"from-stdin","skip_switch":true}`
	if _, stderr, err := runProxy(e, path, input, nil, "--host", "wire", "create"); err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if stdin := shimFile(t, shimDir, "stdin"); stdin != input {
		t.Errorf("forwarded stdin = %q, want %q byte-identical", stdin, input)
	}
}

func TestCLI_RemoteProxy_ExitCodePassthrough(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	_, path := sshShim(t, e, "", 7)

	_, _, err := runProxy(e, path, "", nil, "--host", "wire", "list")
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 7 {
		t.Fatalf("err = %v, want exit code 7", err)
	}
}

func TestCLI_RemoteProxy_SSHFailureHint(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	_, path := sshShim(t, e, "", 255)

	_, stderr, err := runProxy(e, path, "", nil, "--host", "wire", "list")
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 255 {
		t.Fatalf("err = %v, want exit code 255", err)
	}
	if !strings.Contains(stderr, "ssh to wire failed") {
		t.Errorf("stderr = %q, want connection hint", stderr)
	}
}

func TestCLI_RemoteProxy_EnvHostBanner(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	shimDir, path := sshShim(t, e, "{}", 0)

	_, stderr, err := runProxy(e, path, "", map[string]string{"MP_HOST": "wire"}, "go", "--json")
	if err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "MP_HOST is set") {
		t.Errorf("stderr = %q, want MP_HOST banner", stderr)
	}
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err != nil {
		t.Errorf("shim was not invoked for MP_HOST forwarding: %v", err)
	}
}

func TestCLI_RemoteProxy_LocalRunsStayLocal(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	shimDir, path := sshShim(t, e, "{}", 0)

	if _, stderr, err := runProxy(e, path, "", nil, "--version"); err != nil {
		t.Fatalf("local mp --version failed: %v\nstderr: %s", err, stderr)
	}
	// init owns --dir: must not be stolen by the proxy layer (it errors about
	// the path, not about proxying, and ssh is never invoked).
	_, _, _ = runProxy(e, path, "", nil, "init", "--schema")
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err == nil {
		t.Error("ssh shim was invoked for a local command")
	}
}

func TestCLI_RemoteProxy_ProjectRouting(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	canned := `{"pieces":[]}`
	record, path := sshShim(t, e, canned, 0) // record = shim dir

	// Seed the registry with one remote project.
	if err := os.MkdirAll(e.dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := `{"version":"1","projects":[{"name":"api","path":"/home/u/api","host":"wire","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(e.dataDir, "projects.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runProxy(e, path, "", nil, "--project", "api", "list")
	if err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if stdout != canned {
		t.Errorf("stdout = %q, want %q", stdout, canned)
	}
	argv := shimFile(t, record, "argv")
	if !strings.Contains(argv, `cd '\''/home/u/api'\'' && exec '\''mp'\'' '\''list'\''`) {
		t.Errorf("ssh argv = %q, want cd into the registered path and --project stripped", argv)
	}

	// Unknown project fails locally, before any ssh.
	_, stderr, err = runProxy(e, path, "", nil, "--project", "nope", "list")
	if err == nil || !strings.Contains(stderr, "no registered project") {
		t.Errorf("unknown project: err = %v stderr = %q", err, stderr)
	}
}

func TestCLI_LocalProjectChdir(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	e.initGitRepo()
	e.initProject("chdir-proj")
	shimDir, path := sshShim(t, e, "{}", 0)

	// Drive the registered local project by name from an unrelated cwd:
	// resolveTarget must chdir, not proxy. (mp init registered it.)
	elsewhere := filepath.Join(e.tmpDir, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(e.binPath, "--project", "chdir-proj", "list")
	cmd.Dir = elsewhere
	cmd.Env = append(e.env(), "PATH="+path)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("mp --project <local> list failed: %v\nstderr: %s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err == nil {
		t.Error("local --project must not invoke ssh")
	}
}

func TestCLI_RemoteDoctor(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	probe := "mp=mp version v9.9.9\ngit=yes\ntmux=yes\ngh=yes\ngh_auth=no\n"
	_, path := sshShim(t, e, probe, 0)

	stdout, stderr, err := runProxy(e, path, "", nil, "remote", "doctor", "wire")
	if err != nil {
		t.Fatalf("doctor failed: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{`"host": "wire"`, `"reachable": true`, `"mp_version": "v9.9.9"`, `"version_match": false`, `"gh": true`, `"gh_auth": false`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor JSON missing %s\ngot: %s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "⚠ mp v9.9.9") {
		t.Errorf("stderr = %q, want version-skew warning", stderr)
	}

	// Unreachable host: non-zero exit, reachable=false in JSON.
	_, path = sshShim(t, e, "", 255)
	stdout, _, err = runProxy(e, path, "", nil, "remote", "doctor", "wire")
	if err == nil || !strings.Contains(stdout, `"reachable": false`) {
		t.Errorf("unreachable: err = %v stdout = %q", err, stdout)
	}
}

func TestCLI_RemoteDoctor_RegistryScan(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	probe := "mp=mp version v9.9.9\ngit=yes\ntmux=yes\ngh=no\ngh_auth=no\n"
	shimDir, path := sshShim(t, e, probe, 0)

	// Two remote projects on the same host + one local: doctor with no args
	// probes the host once and skips the local entry.
	if err := os.MkdirAll(e.dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := `{"version":"1","projects":[
		{"name":"api","path":"/home/u/api","host":"wire","added_at":"2026-01-01T00:00:00Z"},
		{"name":"web","path":"/home/u/web","host":"wire","added_at":"2026-01-01T00:00:00Z"},
		{"name":"loc","path":"/local/loc","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(e.dataDir, "projects.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runProxy(e, path, "", nil, "remote", "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\nstderr: %s", err, stderr)
	}
	if got := strings.Count(stdout, `"host": "wire"`); got != 1 {
		t.Errorf("host wire probed %d times in JSON, want exactly 1 (dedupe)", got)
	}
	if strings.Contains(stdout, "loc") {
		t.Errorf("local project leaked into doctor output: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err != nil {
		t.Error("registry scan did not probe the remote host")
	}

	// No hosts registered at all: clear error.
	if err := os.WriteFile(filepath.Join(e.dataDir, "projects.json"), []byte(`{"version":"1","projects":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err = runProxy(e, path, "", nil, "remote", "doctor")
	if err == nil || !strings.Contains(stderr, "no remote projects registered") {
		t.Errorf("no-hosts: err = %v stderr = %q", err, stderr)
	}
}
