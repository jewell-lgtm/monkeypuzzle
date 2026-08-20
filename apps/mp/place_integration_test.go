//go:build integration

package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// seqShim installs fake `ssh` and `rsync` binaries that replay one canned
// response per call (stdout-N / exit-N files, 1-based) and record each call's
// argv-N / stdin-N, so multi-round-trip flows like `mp create --remote` run
// without a box. Unlisted calls succeed with empty output.
type seqShim struct {
	dir  string
	path string
}

func newSeqShim(t *testing.T, e *testEnv, responses ...shimResponse) seqShim {
	t.Helper()
	dir := filepath.Join(e.tmpDir, "seq-shim")
	_ = os.RemoveAll(dir) // a fresh shim restarts the call counter
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, r := range responses {
		n := strconv.Itoa(i + 1)
		if err := os.WriteFile(filepath.Join(dir, "stdout-"+n), []byte(r.stdout), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "exit-"+n), []byte(strconv.Itoa(r.exit)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf(`#!/bin/sh
cd %q
n=$(cat counter 2>/dev/null || echo 0); n=$((n+1)); echo $n > counter
printf '%%s\n' "$(basename "$0")" "$@" > argv-$n
cat > stdin-$n
[ -f stdout-$n ] && cat stdout-$n
code=0; [ -f exit-$n ] && code=$(cat exit-$n)
exit $code
`, dir)
	for _, name := range []string{"ssh", "rsync"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return seqShim{dir: dir, path: dir + string(os.PathListSeparator) + os.Getenv("PATH")}
}

type shimResponse struct {
	stdout string
	exit   int
}

func (s seqShim) calls(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, "counter"))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}

func (s seqShim) argv(t *testing.T, n int) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, "argv-"+strconv.Itoa(n)))
	if err != nil {
		t.Fatalf("shim call %d not recorded: %v", n, err)
	}
	return string(data)
}

const (
	boxProject = "/home/u/.local/share/mp/placed-proj"
	probeOK    = "mp=mp version v9.9.9\ngit=yes\ntmux=yes\ngh=yes\ngh_auth=yes\ninit=yes\n"
	probeNoMP  = "mp=missing\ngit=yes\ntmux=no\ngh=no\ngh_auth=no\ninit=no\n"
	probeNoIni = "mp=mp version v9.9.9\ngit=yes\ntmux=yes\ngh=yes\ngh_auth=yes\ninit=no\n"
)

func createJSON(name string) string {
	return fmt.Sprintf(`{"name":%q,"worktree_path":"%s/.monkeypuzzle/pieces/%s","session_name":"mp/placed-proj/%s"}`, name, boxProject, name, name)
}

// placeEnv is a local project with an origin, ready to place pieces from.
func placeEnv(t *testing.T) *testEnv {
	t.Helper()
	e := setupTestEnv(t)
	t.Cleanup(e.cleanup)
	e.initGitRepo()
	e.initProject("placed-proj")
	e.addBareOrigin()
	return e
}

func readPlacements(t *testing.T, e *testEnv) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.tmpDir, ".monkeypuzzle", "placements.json"))
	if os.IsNotExist(err) {
		return map[string]map[string]any{}
	}
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("placements.json: %v\n%s", err, data)
	}
	return p
}

func readRegistry(t *testing.T, e *testEnv) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(e.dataDir, "projects.json"))
	return string(data)
}

func TestCLI_CreateRemote_HappyPath(t *testing.T) {
	e := placeEnv(t)
	hooks := filepath.Join(e.tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := newSeqShim(t, e,
		shimResponse{stdout: boxProject + "\n"}, // 1 connect: clone + init, prints resolved path
		shimResponse{},                          // 2 rsync hooks
		shimResponse{stdout: probeOK},           // 3 doctor probe
		shimResponse{stdout: createJSON("fix-auth")},
		shimResponse{stdout: probeOK}, // second create: already connected, probe only
		shimResponse{stdout: createJSON("fix-auth-2")},
	)

	stdout, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	if err != nil {
		t.Fatalf("create --remote: %v\nstderr: %s", err, stderr)
	}
	if shim.calls(t) != 4 {
		t.Fatalf("shim calls = %d, want 4 (connect, rsync, doctor, create)", shim.calls(t))
	}
	connect := shim.argv(t, 1)
	for _, want := range []string{"ssh\n", "BatchMode=yes", "wire\n", "git clone", "placed-proj", "readlink -f", `'\''init'\'' '\''--name'\'' '\''placed-proj'\'' '\''--pr-provider'\'' '\''github'\''`, ".local/share/mp"} {
		if !strings.Contains(connect, want) {
			t.Errorf("connect argv missing %q:\n%s", want, connect)
		}
	}
	if rs := shim.argv(t, 2); !strings.HasPrefix(rs, "rsync\n") || !strings.Contains(rs, "wire:"+boxProject+"/.monkeypuzzle/hooks/") {
		t.Errorf("rsync argv = %q", rs)
	}
	if probe := shim.argv(t, 3); !strings.Contains(probe, `init=$(test -f '\''`+boxProject+`/.monkeypuzzle/monkeypuzzle.json'\''`) {
		t.Errorf("doctor probe lacks --dir init check:\n%s", probe)
	}
	create := shim.argv(t, 4)
	for _, want := range []string{`cd '\''` + boxProject + `'\''`, `'\''create'\'' '\''--name'\'' '\''fix-auth'\'' '\''--skip-switch'\'' '\''--json'\''`} {
		if !strings.Contains(create, want) {
			t.Errorf("create argv missing %q:\n%s", want, create)
		}
	}
	if strings.Contains(create, "--parent") {
		t.Errorf("main parent must not be forwarded:\n%s", create)
	}
	var info struct {
		Name, WorktreePath, Host string
	}
	if err := json.Unmarshal([]byte(stdout), &struct {
		Name         *string `json:"name"`
		WorktreePath *string `json:"worktree_path"`
		Host         *string `json:"host"`
	}{&info.Name, &info.WorktreePath, &info.Host}); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if info.Host != "wire" || info.Name != "fix-auth" || !strings.HasPrefix(info.WorktreePath, boxProject) {
		t.Errorf("create JSON = %+v", info)
	}

	p := readPlacements(t, e)
	link := p["fix-auth"]
	if link == nil || link["box"] != "wire" || link["pending"] == true || link["remote_path"] != boxProject+"/.monkeypuzzle/pieces/fix-auth" || link["remote_project"] != boxProject {
		t.Errorf("placement = %+v", link)
	}
	reg := readRegistry(t, e)
	for _, want := range []string{`"name": "placed-proj@wire"`, `"host": "wire"`, `"hidden": true`, `"path": "` + boxProject + `"`, `"linked_from":`} {
		if !strings.Contains(reg, want) {
			t.Errorf("registry missing %s:\n%s", want, reg)
		}
	}

	// It shows up in the local list, placed.
	stdout, _, _ = runProxy(e, shim.path, "", nil, "list", "--flat", "--json")
	if !strings.Contains(stdout, `"name": "fix-auth"`) || !strings.Contains(stdout, `"host": "wire"`) {
		t.Errorf("list missing placed piece:\n%s", stdout)
	}

	// Second placement on a connected box skips connect; stacked parent is
	// forwarded; stdin JSON "remote" works.
	_, stderr, err = runProxy(e, shim.path, `{"name":"fix-auth-2","parent":"fix-auth","remote":"wire"}`, nil, "create")
	if err != nil {
		t.Fatalf("second create --remote: %v\nstderr: %s", err, stderr)
	}
	if shim.calls(t) != 6 {
		t.Errorf("shim calls = %d, want 6 (no reconnect)", shim.calls(t))
	}
	if create := shim.argv(t, 6); !strings.Contains(create, `'\''--parent'\'' '\''fix-auth'\''`) {
		t.Errorf("parent not forwarded:\n%s", create)
	}
	if strings.Count(readRegistry(t, e), `"placed-proj@wire"`) != 1 {
		t.Error("registry row duplicated")
	}
}

func TestCLI_CreateRemote_Unreachable(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e, shimResponse{exit: 255})

	_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	if err == nil || !strings.Contains(stderr, "box unreachable") {
		t.Fatalf("err = %v stderr = %q, want ErrBoxUnreachable", err, stderr)
	}
	if len(readPlacements(t, e)) != 0 {
		t.Error("pending link not removed after ssh failure")
	}
	if strings.Contains(readRegistry(t, e), "wire") {
		t.Error("registry row written for unconnected box")
	}
}

func TestCLI_CreateRemote_ConnectFails(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e, shimResponse{exit: 128})

	_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	if err == nil || !strings.Contains(stderr, "box connect failed") {
		t.Fatalf("err = %v stderr = %q, want ErrBoxConnect", err, stderr)
	}
	if len(readPlacements(t, e)) != 0 {
		t.Error("pending link not removed after connect failure")
	}
}

func TestCLI_CreateRemote_NotInitialised(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e,
		shimResponse{stdout: boxProject + "\n"},
		shimResponse{stdout: probeNoIni},
	)
	_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	if err == nil || !strings.Contains(stderr, "not an mp project") {
		t.Fatalf("err = %v stderr = %q, want ErrBoxNotInitialised", err, stderr)
	}
	if len(readPlacements(t, e)) != 0 {
		t.Error("pending link not removed")
	}
	if strings.Contains(readRegistry(t, e), "wire") {
		t.Error("registry row written before the box passed doctor")
	}

	// Missing mp on the box.
	e2 := placeEnv(t)
	shim2 := newSeqShim(t, e2,
		shimResponse{stdout: boxProject + "\n"},
		shimResponse{stdout: probeNoMP},
	)
	_, stderr, err = runProxy(e2, shim2.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	if err == nil || !strings.Contains(stderr, "mp is not installed on the box") {
		t.Fatalf("err = %v stderr = %q, want ErrRemoteMPMissing", err, stderr)
	}
}

func TestCLI_CreateRemote_DuplicateName(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e)
	if _, stderr, err := runProxy(e, shim.path, "", nil, "create", "--name", "taken", "--skip-switch"); err != nil {
		t.Fatalf("local create: %v\n%s", err, stderr)
	}
	_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "taken")
	if err == nil || !strings.Contains(stderr, "piece already exists") {
		t.Fatalf("err = %v stderr = %q, want ErrPieceExists", err, stderr)
	}
	if shim.calls(t) != 0 {
		t.Error("ssh invoked despite local validation failure")
	}
	if len(readPlacements(t, e)) != 0 {
		t.Error("link written for a rejected name")
	}

	// Invalid box name fails before anything too.
	_, stderr, err = runProxy(e, shim.path, "", nil, "create", "--remote", "-oProxyCommand=x", "--name", "fresh")
	if err == nil || !strings.Contains(stderr, "invalid ssh host") {
		t.Errorf("bad box: err = %v stderr = %q", err, stderr)
	}
}

func TestCLI_CreateRemote_CrossBoxParent(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e)
	if _, stderr, err := runProxy(e, shim.path, "", nil, "create", "--name", "local-parent", "--skip-switch"); err != nil {
		t.Fatalf("local create: %v\n%s", err, stderr)
	}
	placements := `{"on-other":{"box":"other","remote_path":"/x/.monkeypuzzle/pieces/on-other"},"half":{"box":"wire","pending":true}}`
	if err := os.WriteFile(filepath.Join(e.tmpDir, ".monkeypuzzle", "placements.json"), []byte(placements), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{"local-parent", "on-other", "half", "nope"} {
		_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "child", "--parent", parent)
		if err == nil || !strings.Contains(stderr, "parent must be main or a piece on the same box") {
			t.Errorf("parent %s: err = %v stderr = %q, want ErrCrossBoxParent", parent, err, stderr)
		}
	}
	if shim.calls(t) != 0 {
		t.Error("ssh invoked despite parent validation failure")
	}
	if _, ok := readPlacements(t, e)["child"]; ok {
		t.Error("link written for rejected child")
	}
}

func TestCLI_CreateRemote_RemoteCreateFails(t *testing.T) {
	e := placeEnv(t)
	shim := newSeqShim(t, e,
		shimResponse{stdout: boxProject + "\n"},
		shimResponse{stdout: probeOK},
		shimResponse{stdout: "", exit: 1}, // box-side mp create fails
	)
	_, stderr, err := runProxy(e, shim.path, "", nil, "create", "--remote", "wire", "--name", "fix-auth")
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() == 0 || !strings.Contains(stderr, "remote create on wire failed") {
		t.Fatalf("err = %v stderr = %q", err, stderr)
	}
	if len(readPlacements(t, e)) != 0 {
		t.Error("link must be removed when the box-side create fails")
	}
	// The box is connected: its row stays so the next attempt skips connect.
	if !strings.Contains(readRegistry(t, e), `"placed-proj@wire"`) {
		t.Error("registry row should survive a failed create")
	}
}

func TestCLI_CreateRemote_Schema(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()
	stdout, _, err := e.run("create", "--schema")
	if err != nil || !strings.Contains(stdout, `"remote"`) {
		t.Errorf("schema missing remote: %v\n%s", err, stdout)
	}
}
