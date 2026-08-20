package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Placement: `mp create --remote=<box>` puts one piece on a long-lived ssh
// box while the project stays here (the controller). The box holds one clone
// per (project, box) under remoteMPDir; the piece itself is a normal worktree
// made by a proxied `mp create` there. The controller remembers the link in
// placements.json and a hidden registry row for the clone.
//
// Vocabulary: controller = the machine running this mp; box = the ssh target
// (mp's existing --host flag / registry.Project.Host).

// remoteMPDir is where box-side clones live, relative to $HOME on the box.
const remoteMPDir = ".local/share/mp"

// Named placement errors, all wrapped with %w so callers can errors.Is.
var (
	ErrCrossBoxParent    = errors.New("parent must be main or a piece on the same box")
	ErrBoxUnreachable    = errors.New("box unreachable over ssh")
	ErrBoxConnect        = errors.New("box connect failed")
	ErrBoxNotInitialised = errors.New("box clone is not an mp project")
	ErrRemoteMPMissing   = errors.New("mp is not installed on the box")
)

// placeRequest is everything step 0 establishes before any side effect.
type placeRequest struct {
	box      string
	name     string // sanitized piece name (what the box will create)
	repoRoot string // controller-side repo root
	project  string
	input    piececmd.NewPieceInput
}

// runPieceCreateRemote is `mp create --remote=<box>`; see the flow in
// docs/remote-development.md "Placing a piece on a box".
func runPieceCreateRemote(ctx context.Context, deps core.Deps, handler *piececmd.Handler, input piececmd.NewPieceInput) error {
	req, err := validatePlacement(ctx, handler, deps.FS, input)
	if err != nil {
		return err
	}
	fs := deps.FS

	// 1. Link first: a crash anywhere below leaves a visible pending link,
	// never an orphaned box-side worktree nobody can find.
	err = piececmd.UpdatePlacements(req.repoRoot, fs, func(p piececmd.Placements) error {
		p[req.name] = piececmd.Placement{Box: req.box, Pending: true}
		return nil
	})
	if err != nil {
		return err
	}
	info, err := placePiece(ctx, req, deps)
	if err != nil {
		if rmErr := piececmd.RemovePlacement(req.repoRoot, req.name, fs); rmErr != nil {
			fmt.Fprintf(os.Stderr, "%s could not remove pending link for %s: %v\n", cli.GlyphWarn, req.name, rmErr)
		}
		return err
	}

	fmt.Fprintf(os.Stderr, "%s Placed %s on %s (%s)\n", cli.GlyphOK, info.Name, req.box, info.WorktreePath)
	if err := cli.PrintJSON(info); err != nil {
		return err
	}
	cli.Hint(fmt.Sprintf("mp --host %s --dir %s pr create --draft", req.box, info.WorktreePath))
	return nil
}

// validatePlacement is step 0: box syntax, name uniqueness across local
// worktrees and placements, parent on the same box.
func validatePlacement(ctx context.Context, handler *piececmd.Handler, fs core.FS, input piececmd.NewPieceInput) (placeRequest, error) {
	box := strings.TrimSpace(input.Remote)
	if err := cli.ValidSSHDest(box); err != nil {
		return placeRequest{}, fmt.Errorf("--remote: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return placeRequest{}, err
	}
	repoRoot, err := projectdir.MainRepoRoot(wd)
	if err != nil {
		return placeRequest{}, fmt.Errorf("not in a git repository: %w", err)
	}
	if !registry.IsProject(repoRoot) {
		return placeRequest{}, fmt.Errorf("%s is not a monkeypuzzle project (run `mp init` first)", repoRoot)
	}
	project, _ := registry.ProjectName(repoRoot)

	name := piececmd.SanitizePieceName(input.Name)
	if name == "" {
		return placeRequest{}, fmt.Errorf("--remote needs a piece name (or a prompt to derive one from)")
	}
	taken, err := handler.PieceExists(repoRoot, name)
	if err != nil {
		return placeRequest{}, err
	}
	if taken {
		return placeRequest{}, fmt.Errorf("%w: %q in %s", piececmd.ErrPieceExists, name, repoRoot)
	}

	if input.Parent != "" && input.Parent != "main" {
		placements, err := piececmd.ReadPlacements(repoRoot, fs)
		if err != nil {
			return placeRequest{}, err
		}
		parent, ok := placements[input.Parent]
		switch {
		case !ok:
			return placeRequest{}, fmt.Errorf("%w: %q is not on %s", ErrCrossBoxParent, input.Parent, box)
		case parent.Box != box:
			return placeRequest{}, fmt.Errorf("%w: %q is on %s, not %s", ErrCrossBoxParent, input.Parent, parent.Box, box)
		case parent.Pending:
			return placeRequest{}, fmt.Errorf("%w: %q is still pending on %s", ErrCrossBoxParent, input.Parent, box)
		}
	}
	_ = ctx
	return placeRequest{box: box, name: name, repoRoot: repoRoot, project: project, input: input}, nil
}

// placePiece is steps 2–7; the caller owns the pending link.
func placePiece(ctx context.Context, req placeRequest, deps core.Deps) (piececmd.PieceInfo, error) {
	fs := deps.FS
	reg, err := registry.Load()
	if err != nil {
		return piececmd.PieceInfo{}, err
	}
	rowName := req.project + "@" + req.box

	// 2–3. Connect the box for this project once; the hidden registry row is
	// the "connected" marker and carries the path resolved on the box.
	var remoteProject string
	for _, p := range reg.Projects {
		if p.Host == req.box && p.Name == rowName {
			remoteProject = p.Path
			break
		}
	}
	if remoteProject == "" {
		remoteProject, err = connectBox(ctx, req, deps)
		if err != nil {
			return piececmd.PieceInfo{}, err
		}
	}

	// 4. Contract check, scoped to the clone.
	r := probeHost(req.box, remoteProject)
	switch {
	case !r.Reachable:
		return piececmd.PieceInfo{}, fmt.Errorf("%w: %s: %s", ErrBoxUnreachable, req.box, r.SSHError)
	case r.MPVersion == "missing":
		return piececmd.PieceInfo{}, fmt.Errorf("%w: %s (install it on PATH or ~/.local/bin)", ErrRemoteMPMissing, req.box)
	case !r.Init:
		return piececmd.PieceInfo{}, fmt.Errorf("%w: %s:%s (run `mp --host %s --dir %s init --name %s`)", ErrBoxNotInitialised, req.box, remoteProject, req.box, remoteProject, req.project)
	}

	// 5. Connected: record the clone (stays even if the create below fails).
	reg.UpsertHidden(req.box, remoteProject, rowName, req.repoRoot)
	if err := reg.Save(); err != nil {
		return piececmd.PieceInfo{}, err
	}

	// 6. The piece itself is an ordinary proxied create on the box.
	argv := []string{"create", "--name", req.name, "--skip-switch", "--json"}
	if req.input.Parent != "" && req.input.Parent != "main" {
		argv = append(argv, "--parent", req.input.Parent)
	}
	if req.input.Prompt != "" {
		argv = append(argv, "--prompt", req.input.Prompt)
	}
	if req.input.Branch != "" {
		argv = append(argv, "--branch", req.input.Branch)
	}
	if req.input.Agent != "" {
		argv = append(argv, "--agent", req.input.Agent)
	}
	target := &remoteTarget{host: req.box, dir: remoteProject, placement: true}
	out, code, err := runRemoteCapture(target, argv, 0)
	if err != nil {
		if code == 255 {
			return piececmd.PieceInfo{}, fmt.Errorf("%w: %s", ErrBoxUnreachable, req.box)
		}
		return piececmd.PieceInfo{}, fmt.Errorf("remote create on %s failed: %w", req.box, err)
	}
	var info piececmd.PieceInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil || info.WorktreePath == "" {
		return piececmd.PieceInfo{}, fmt.Errorf("remote create on %s returned no worktree_path (got %q)", req.box, strings.TrimSpace(out))
	}
	info.Host = req.box

	// 7. Placed.
	err = piececmd.UpdatePlacements(req.repoRoot, fs, func(p piececmd.Placements) error {
		p[req.name] = piececmd.Placement{
			Box:           req.box,
			RemotePath:    info.WorktreePath,
			RemoteProject: remoteProject,
			Cached:        &piececmd.PieceListItem{Name: info.Name, Parent: req.input.Parent, Branch: req.input.Branch, ModTime: time.Now().UTC()},
		}
		return nil
	})
	if err != nil {
		return piececmd.PieceInfo{}, err
	}
	return info, nil
}

// connectBox is the first touch of a box for a project. It fires at most
// once successfully per (project, box): the hidden registry row the caller
// writes afterwards is the marker, so a failed connect is retried next time.
// The controller-side on-box-connect.sh hook, when present, replaces the
// built-in connect entirely (it sees MP_BOX, MP_REMOTE_PATH, MP_REPO_URL,
// MP_PROJECT, MP_HOOKS_DIR); built-in = clone the origin into
// $HOME/.local/share/mp/<project> (skipped when present), `mp init` it
// (skipped when already a project), ship the controller's hooks. Either way
// the clone's path is then resolved on the box — $HOME only expands there.
func connectBox(ctx context.Context, req placeRequest, deps core.Deps) (string, error) {
	originB, err := exec.CommandContext(ctx, "git", "-C", req.repoRoot, "remote", "get-url", "origin").Output()
	origin := string(originB)
	if err != nil || strings.TrimSpace(origin) == "" {
		return "", fmt.Errorf("%w: %s has no origin remote to clone from", ErrBoxConnect, req.repoRoot)
	}
	origin = strings.TrimSpace(origin)
	// The project dir is quoted as a suffix so the box expands $HOME itself.
	dir := `"$HOME"/` + cli.ShQuote(remoteMPDir+"/"+req.project)

	hooks := piececmd.NewHookRunner(deps)
	out, ran, err := hooks.RunHookOutput(ctx, req.repoRoot, piececmd.HookOnBoxConnect, piececmd.HookContext{
		Box:        req.box,
		RemotePath: "$HOME/" + remoteMPDir + "/" + req.project,
		RepoURL:    origin,
		Project:    req.project,
		HooksDir:   projectdir.HooksDir(req.repoRoot),
	})
	if err != nil {
		return "", fmt.Errorf("%w: %s: %s: %s", ErrBoxConnect, req.box, piececmd.HookOnBoxConnect, strings.TrimSpace(string(out)))
	}
	var script string
	if ran {
		script = `cd ` + dir + ` && readlink -f .`
	} else {
		script = builtinConnectScript(req, deps.FS, origin, dir)
		fmt.Fprintf(os.Stderr, "Connecting %s for %s (clone + mp init under ~/%s)...\n", req.box, req.project, remoteMPDir)
	}
	stdout, stderr, code, err := sshScript(req.box, script)
	if err != nil {
		if code == 255 {
			return "", fmt.Errorf("%w: %s: %s", ErrBoxUnreachable, req.box, strings.TrimSpace(stderr))
		}
		return "", fmt.Errorf("%w: %s: %s", ErrBoxConnect, req.box, strings.TrimSpace(stderr))
	}
	root := strings.TrimSpace(stdout)
	if root == "" || !strings.HasPrefix(root, "/") {
		return "", fmt.Errorf("%w: %s: could not resolve clone path (got %q)", ErrBoxConnect, req.box, root)
	}
	if ran {
		return root, nil // the hook owns hooks/toolchain/creds on the box
	}

	// Hooks exist on the box only if shipped from here.
	hooksDir := projectdir.HooksDir(req.repoRoot)
	if fi, err := os.Stat(hooksDir); err == nil && fi.IsDir() {
		dest := req.box + ":" + root + "/.monkeypuzzle/hooks/"
		cmd := exec.Command("rsync", "-a", "--", hooksDir+string(filepath.Separator), dest)
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("%w: shipping hooks to %s: %s", ErrBoxConnect, dest, strings.TrimSpace(errb.String()))
		}
	}
	return root, nil
}

// builtinConnectScript is the default connect, run on the box under sh.
func builtinConnectScript(req placeRequest, fs core.FS, origin, dir string) string {
	initArgs := []string{"init", "--name", req.project}
	if cfg, err := piececmd.ReadConfig(req.repoRoot, fs); err == nil && cfg.PR.Provider != "" {
		initArgs = append(initArgs, "--pr-provider", cfg.PR.Provider)
	}
	var init strings.Builder
	init.WriteString(cli.ShQuote(remoteBin()))
	for _, a := range initArgs {
		init.WriteString(" " + cli.ShQuote(a))
	}
	return `export PATH="$HOME/.local/bin:$PATH"
set -e
mkdir -p "$HOME"/` + cli.ShQuote(remoteMPDir) + `
if [ ! -d ` + dir + `/.git ]; then git clone ` + cli.ShQuote(origin) + ` ` + dir + ` >&2; fi
cd ` + dir + `
if [ ! -f .monkeypuzzle/monkeypuzzle.json ]; then echo '{}' | ` + init.String() + ` >/dev/null; fi
readlink -f .`
}

// sshScript runs a POSIX script on host (sh -c, BatchMode, 5s connect) and
// returns its stdout, stderr and exit code.
func sshScript(host, script string) (stdout, stderr string, code int, err error) {
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "--", host, "sh -c "+cli.ShQuote(script))
	cmd.Stdin = strings.NewReader("")
	var outb, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outb, &errb
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = 1
	}
	return outb.String(), errb.String(), code, err
}
