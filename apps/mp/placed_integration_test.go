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

const placedPath = boxProject + "/.monkeypuzzle/pieces/fix-auth"

// placedEnv is a local project with fix-auth placed on wire (link + hidden
// registry row), plus the extra placements given.
func placedEnv(t *testing.T, extra string) *testEnv {
	t.Helper()
	e := placeEnv(t)
	placements := `{"fix-auth":{"box":"wire","remote_path":"` + placedPath + `","remote_project":"` + boxProject + `"}` + extra + `}`
	if err := os.WriteFile(filepath.Join(e.tmpDir, ".monkeypuzzle", "placements.json"), []byte(placements), 0o644); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.EvalSymlinks(e.tmpDir)
	reg := `{"version":"1","projects":[
		{"name":"placed-proj","path":"` + root + `","added_at":"2026-01-01T00:00:00Z"},
		{"name":"placed-proj@wire","path":"` + boxProject + `","host":"wire","hidden":true,"linked_from":"` + root + `","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(e.dataDir, "projects.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCLI_Placed_StatusProxies(t *testing.T) {
	e := placedEnv(t, "")
	canned := `{"in_piece":true,"piece_name":"fix-auth"}`
	shimDir, path := sshShim(t, e, canned, 0)

	stdout, stderr, err := runProxy(e, path, "", nil, "status", "fix-auth", "--json")
	if err != nil {
		t.Fatalf("status placed: %v\n%s", err, stderr)
	}
	if stdout != canned {
		t.Errorf("stdout = %q, want box's response", stdout)
	}
	argv := shimFile(t, shimDir, "argv")
	if !strings.Contains(argv, `cd '\''`+placedPath+`'\'' && exec '\''mp'\'' '\''status'\'' '\''--json'\''`) {
		t.Errorf("proxied argv must cd into the box worktree and drop the selector:\n%s", argv)
	}
	// --piece form strips too.
	_, _, _ = runProxy(e, path, "", nil, "status", "--piece=fix-auth")
	if argv := shimFile(t, shimDir, "argv"); strings.Contains(argv, "--piece") || !strings.Contains(argv, `'\''status'\''`) {
		t.Errorf("--piece= not stripped:\n%s", argv)
	}
	// Link untouched by a non-ending verb.
	if _, ok := readPlacements(t, e)["fix-auth"]; !ok {
		t.Error("status dropped the link")
	}
	// Local pieces still resolve locally (no ssh).
	if _, stderr, err := runProxy(e, path, "", nil, "create", "--name", "local-a", "--skip-switch"); err != nil {
		t.Fatalf("create: %v\n%s", err, stderr)
	}
	_ = os.Remove(filepath.Join(shimDir, "argv"))
	if _, stderr, err := runProxy(e, path, "", nil, "status", "local-a"); err != nil {
		t.Errorf("status local: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err == nil {
		t.Error("local piece status went over ssh")
	}
}

func TestCLI_Placed_DoneDropsLinkAndReapsRow(t *testing.T) {
	e := placedEnv(t, "")
	// Box-side failure keeps the link and passes the exit code through.
	_, path := sshShim(t, e, "", 3)
	_, _, err := runProxy(e, path, "", nil, "done", "fix-auth")
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit 3 passthrough", err)
	}
	if _, ok := readPlacements(t, e)["fix-auth"]; !ok {
		t.Fatal("link dropped although the box-side done failed")
	}

	shimDir, path := sshShim(t, e, `{"status":"done"}`, 0)
	_, stderr, err := runProxy(e, path, "", nil, "done", "--piece", "fix-auth")
	if err != nil {
		t.Fatalf("done placed: %v\n%s", err, stderr)
	}
	if argv := shimFile(t, shimDir, "argv"); !strings.Contains(argv, `'\''done'\''`) || strings.Contains(argv, "--piece") {
		t.Errorf("argv = %s", argv)
	}
	if _, ok := readPlacements(t, e)["fix-auth"]; ok {
		t.Error("link not dropped after done")
	}
	if !strings.Contains(stderr, "Dropped placement fix-auth") {
		t.Errorf("stderr = %q", stderr)
	}
	// Last link on wire: the hidden row goes; the local row stays.
	reg := readRegistry(t, e)
	if strings.Contains(reg, "placed-proj@wire") || !strings.Contains(reg, `"name": "placed-proj"`) {
		t.Errorf("registry after last link dropped:\n%s", reg)
	}
}

func TestCLI_Placed_AbandonKeepsRowWhileLinksRemain(t *testing.T) {
	e := placedEnv(t, `,"second":{"box":"wire","remote_path":"`+boxProject+`/.monkeypuzzle/pieces/second","remote_project":"`+boxProject+`"}`)
	shimDir, path := sshShim(t, e, `{}`, 0)
	if _, stderr, err := runProxy(e, path, "", nil, "abandon", "fix-auth", "--force"); err != nil {
		t.Fatalf("abandon placed: %v\n%s", err, stderr)
	}
	if argv := shimFile(t, shimDir, "argv"); !strings.Contains(argv, `'\''abandon'\'' '\''--force'\''`) {
		t.Errorf("argv = %s", argv)
	}
	p := readPlacements(t, e)
	if _, ok := p["fix-auth"]; ok {
		t.Error("link not dropped")
	}
	if _, ok := p["second"]; !ok {
		t.Error("sibling link dropped")
	}
	if !strings.Contains(readRegistry(t, e), "placed-proj@wire") {
		t.Error("hidden row reaped while a link remains")
	}
}

func TestCLI_Placed_PendingRefused(t *testing.T) {
	e := placedEnv(t, `,"half":{"box":"wire","pending":true}`)
	shimDir, path := sshShim(t, e, `{}`, 0)
	for _, verb := range []string{"status", "done", "abandon"} {
		_, stderr, err := runProxy(e, path, "", nil, verb, "half")
		if err == nil || !strings.Contains(stderr, "still pending") {
			t.Errorf("%s pending: err = %v stderr = %q, want ErrLinkPending", verb, err, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(shimDir, "argv")); err == nil {
		t.Error("pending link reached ssh")
	}
}

func TestCLI_Placed_CleanupReapsStaleAndPending(t *testing.T) {
	// fix-auth present on the box, gone stale, half pending (connected box so
	// its would-be path is probed), orphan pending on a box never connected.
	e := placedEnv(t, `,"gone":{"box":"wire","remote_path":"`+boxProject+`/.monkeypuzzle/pieces/gone","remote_project":"`+boxProject+`"},"half":{"box":"wire","pending":true},"orphan":{"box":"other","pending":true}`)
	// Probes run in sorted link order: fix-auth, gone, half (orphan has no
	// path → no ssh). Every cleanup run previews first, so --yes probes twice.
	present, absent := shimResponse{exit: 0}, shimResponse{exit: 1}
	shim := newSeqShim(t, e,
		present, absent, absent, // --dry-run
		present, absent, absent, // --yes preview
		present, absent, absent, // --yes apply
	)

	stdout, stderr, err := runProxy(e, shim.path, "", nil, "cleanup", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("cleanup --dry-run: %v\n%s", err, stderr)
	}
	if shim.calls(t) != 3 {
		t.Errorf("dry-run probed %d links, want 3", shim.calls(t))
	}
	if probe := shim.argv(t, 3); !strings.Contains(probe, "test -d '\\''"+boxProject+"/.monkeypuzzle/pieces/half'\\''") {
		t.Errorf("pending link probe path:\n%s", probe)
	}
	var out struct {
		Links []struct {
			Piece   string `json:"piece"`
			Present bool   `json:"present"`
			Pending bool   `json:"pending"`
			Dropped bool   `json:"dropped"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("cleanup JSON: %v\n%s", err, stdout)
	}
	if len(out.Links) != 4 {
		t.Fatalf("links = %+v", out.Links)
	}
	for _, l := range out.Links {
		if l.Dropped {
			t.Errorf("dry-run dropped %s", l.Piece)
		}
		if l.Piece == "fix-auth" && !l.Present {
			t.Error("fix-auth should be present")
		}
	}
	if !strings.Contains(stderr, "Would drop placement gone") || !strings.Contains(stderr, "Would drop placement half") || !strings.Contains(stderr, "Would drop placement orphan") {
		t.Errorf("dry-run stderr = %q", stderr)
	}
	if len(readPlacements(t, e)) != 4 {
		t.Error("dry-run mutated placements")
	}

	_, stderr, err = runProxy(e, shim.path, "", nil, "cleanup", "--yes")
	if err != nil {
		t.Fatalf("cleanup --yes: %v\n%s", err, stderr)
	}
	p := readPlacements(t, e)
	if _, ok := p["fix-auth"]; !ok || len(p) != 1 {
		t.Errorf("after apply placements = %+v, want only fix-auth", p)
	}
	if !strings.Contains(readRegistry(t, e), "placed-proj@wire") {
		t.Error("hidden row reaped while fix-auth remains")
	}

	// Unreachable box: link kept, warning.
	shim2 := newSeqShim(t, e, shimResponse{exit: 255})
	_, stderr, err = runProxy(e, shim2.path, "", nil, "cleanup", "--yes")
	if err != nil {
		t.Fatalf("cleanup unreachable: %v\n%s", err, stderr)
	}
	if _, ok := readPlacements(t, e)["fix-auth"]; !ok || !strings.Contains(stderr, "keeping placement fix-auth") {
		t.Errorf("unreachable box must keep the link; stderr = %q", stderr)
	}
}

func TestCLI_Placed_DoctorListsPending(t *testing.T) {
	e := placedEnv(t, `,"half":{"box":"wire","pending":true}`)
	probe := "mp=mp version v9.9.9\ngit=yes\ntmux=yes\ngh=yes\ngh_auth=yes\n"
	_, path := sshShim(t, e, probe, 0)

	stdout, stderr, err := runProxy(e, path, "", nil, "remote", "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"pending_links": [`) || !strings.Contains(stdout, `"placed-proj/half"`) || strings.Contains(stdout, `"placed-proj/fix-auth"`) {
		t.Errorf("doctor JSON pending links:\n%s", stdout)
	}
	if !strings.Contains(stderr, "pending placements: placed-proj/half") {
		t.Errorf("doctor stderr = %q", stderr)
	}
}
