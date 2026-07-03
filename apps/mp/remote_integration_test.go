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

// sshShim installs a fake `ssh` first on PATH that records its argv to a file
// and replays a canned response, so the remote proxy is tested without a live
// host. Returns the record file path and the PATH value to run mp with.
func sshShim(t *testing.T, e *testEnv, stdout string, exitCode int) (recordFile, path string) {
	t.Helper()
	shimDir := filepath.Join(e.tmpDir, "ssh-shim")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordFile = filepath.Join(shimDir, "argv")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf '%%s' %q\nexit %d\n", recordFile, stdout, exitCode)
	if err := os.WriteFile(filepath.Join(shimDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return recordFile, shimDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// runProxy invokes mp with the shim PATH plus any extra env, preserving the
// data/config isolation from testEnv.env().
func runProxy(e *testEnv, path string, extraEnv map[string]string, args ...string) (string, string, error) {
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = e.tmpDir
	cmd.Env = append(e.env(), "PATH="+path)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
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
	record, path := sshShim(t, e, canned, 0)

	stdout, stderr, err := runProxy(e, path, nil,
		"--host", "wire", "--dir", "/home/u/api", "create", "--name", "add-rate-limiting", "--json")
	if err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if stdout != canned {
		t.Errorf("stdout = %q, want canned shim response %q", stdout, canned)
	}

	argv, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("shim was not invoked: %v", err)
	}
	got := strings.Split(strings.TrimRight(string(argv), "\n"), "\n")
	want := []string{
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "wire", "--",
		`export PATH="$HOME/.local/bin:$PATH"; cd '/home/u/api' && exec 'mp' 'create' '--name' 'add-rate-limiting' '--json'`,
	}
	if len(got) != len(want) {
		t.Fatalf("ssh argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ssh argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCLI_RemoteProxy_ExitCodePassthrough(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	_, path := sshShim(t, e, "", 7)

	_, _, err := runProxy(e, path, nil, "--host", "wire", "list")
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 7 {
		t.Fatalf("err = %v, want exit code 7", err)
	}
}

func TestCLI_RemoteProxy_SSHFailureHint(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	_, path := sshShim(t, e, "", 255)

	_, stderr, err := runProxy(e, path, nil, "--host", "wire", "list")
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
	record, path := sshShim(t, e, "{}", 0)

	_, stderr, err := runProxy(e, path, map[string]string{"MP_HOST": "wire"}, "go", "--json")
	if err != nil {
		t.Fatalf("proxied mp failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "MP_HOST is set") {
		t.Errorf("stderr = %q, want MP_HOST banner", stderr)
	}
	if _, err := os.ReadFile(record); err != nil {
		t.Errorf("shim was not invoked for MP_HOST forwarding: %v", err)
	}
}

func TestCLI_RemoteProxy_LocalRunsStayLocal(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	record, path := sshShim(t, e, "{}", 0)

	if _, stderr, err := runProxy(e, path, nil, "--version"); err != nil {
		t.Fatalf("local mp --version failed: %v\nstderr: %s", err, stderr)
	}
	if _, err := os.Stat(record); err == nil {
		t.Error("ssh shim was invoked for a local command")
	}
}
