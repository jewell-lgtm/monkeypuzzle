package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Verb routing for placed pieces: a piece selector that names a link in
// placements.json is proxied to its box (`--host box --dir remote_path`)
// instead of resolved to a local worktree. Piece-ending verbs drop the link
// after the box has done its part.

var (
	// ErrLinkPending: the piece's create never finished on the box.
	ErrLinkPending = errors.New("piece placement is still pending")
	// ErrLinkStale: the link points at a box-side piece that no longer exists.
	ErrLinkStale = errors.New("piece placement is stale (gone on the box)")
)

// pieceLocation is where a selected piece lives: a local worktree, or a
// placement on a box.
type pieceLocation struct {
	workDir   string // local worktree (or cwd when no selector was given)
	repoRoot  string
	name      string
	placement *piececmd.Placement // nil for local pieces
}

func (l pieceLocation) placed() bool { return l.placement != nil }

// locatePiece resolves a piece selector. An empty selector is the current
// directory, as always. A name in placements.json is a placed piece; anything
// else must be a local worktree (resolvePieceWorkDir's rules apply).
func locatePiece(ctx context.Context, fs core.FS, selector string) (pieceLocation, error) {
	if selector == "" {
		wd, err := resolvePieceWorkDir(ctx, selector)
		return pieceLocation{workDir: wd}, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return pieceLocation{}, fmt.Errorf("failed to get working directory: %w", err)
	}
	if root, err := projectdir.MainRepoRoot(wd); err == nil {
		placements, err := piececmd.ReadPlacements(root, fs)
		if err != nil {
			return pieceLocation{}, err
		}
		if pl, ok := placements[selector]; ok {
			if pl.Pending {
				return pieceLocation{}, fmt.Errorf("%w: %q on %s (run `mp cleanup` to drop it, then create again)", ErrLinkPending, selector, pl.Box)
			}
			return pieceLocation{repoRoot: root, name: selector, placement: &pl}, nil
		}
	}
	workDir, err := resolvePieceWorkDir(ctx, selector)
	return pieceLocation{workDir: workDir, name: selector}, err
}

// proxyPlaced forwards the current invocation to the placed piece's box,
// minus the selector (the box-side mp runs inside the worktree, so the verb
// needs none). Piece-ending verbs pass ending=true: on success the link is
// dropped and, when the box holds no more of this project's pieces, its
// hidden registry row goes too. Never returns on success — it exits with the
// box's exit code, like every other proxied command.
func proxyPlaced(loc pieceLocation, fs core.FS, selector string, ending bool) error {
	args := stripSelector(os.Args[1:], selector)
	target := &remoteTarget{host: loc.placement.Box, dir: loc.placement.RemotePath, placement: true}
	code := runRemote(target, args)
	if code != 0 {
		os.Exit(code)
	}
	if ending {
		if err := dropLink(loc.repoRoot, loc.name, fs); err != nil {
			return err
		}
	}
	os.Exit(0)
	return nil
}

// stripSelector removes the piece selector from argv: the first positional
// equal to it, and any --piece/--name flag carrying it (both forms).
func stripSelector(args []string, selector string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--piece", "--name":
			if i+1 < len(args) && args[i+1] == selector {
				i++
				continue
			}
		case "--piece=" + selector, "--name=" + selector, selector:
			continue
		}
		out = append(out, a)
	}
	return out
}

// dropLink removes a placement and reaps the box's hidden registry row when
// no links to that box remain for this project.
func dropLink(repoRoot, name string, fs core.FS) error {
	var box string
	var remaining int
	err := piececmd.UpdatePlacements(repoRoot, fs, func(p piececmd.Placements) error {
		box = p[name].Box
		delete(p, name)
		remaining = len(p.On(box))
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s Dropped placement %s (%s)\n", cli.GlyphOK, name, box)
	if remaining == 0 && box != "" {
		return reapHiddenRow(repoRoot, box)
	}
	return nil
}

// reapHiddenRow removes the hidden registry row that marks box as connected
// for the project at repoRoot. User-registered (visible) rows are left alone.
func reapHiddenRow(repoRoot, box string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	kept := reg.Projects[:0]
	var reaped []registry.Project
	for _, p := range reg.Projects {
		if p.Hidden && p.Host == box && p.LinkedFrom == repoRoot {
			reaped = append(reaped, p)
			continue
		}
		kept = append(kept, p)
	}
	if len(reaped) == 0 {
		return nil
	}
	reg.Projects = kept
	if err := reg.Save(); err != nil {
		return err
	}
	for _, p := range reaped {
		fmt.Fprintf(os.Stderr, "%s Forgot box clone %s (%s)\n", cli.GlyphOK, p.Name, p.Location())
	}
	return nil
}

// linkCheck is one placement's verdict from a cleanup pass.
type linkCheck struct {
	Piece   string `json:"piece"`
	Box     string `json:"box"`
	Pending bool   `json:"pending,omitempty"`
	// Present reports whether the box-side piece directory exists; Unreachable
	// means the box could not be asked (the link is kept).
	Present     bool   `json:"present"`
	Unreachable bool   `json:"unreachable,omitempty"`
	Dropped     bool   `json:"dropped,omitempty"`
	Error       string `json:"error,omitempty"`
}

// boxPiecePath is the worktree path a link points at, or where a pending
// link's piece would have landed if the box is connected. Empty = unknown.
func boxPiecePath(reg registry.Registry, repoRoot, name string, pl piececmd.Placement) string {
	if pl.RemotePath != "" {
		return pl.RemotePath
	}
	for _, p := range reg.Projects {
		if p.Host == pl.Box && p.LinkedFrom == repoRoot {
			return p.Path + "/" + projectdir.DefaultDirName + "/pieces/" + name
		}
	}
	return ""
}

// checkLinks asks each box whether its side of every link still exists:
// stale links (box says no) and pending links are reported, and dropped when
// dryRun is false. Unreachable boxes keep their links.
func checkLinks(repoRoot string, fs core.FS, dryRun bool) ([]linkCheck, error) {
	placements, err := piececmd.ReadPlacements(repoRoot, fs)
	if err != nil {
		return nil, err
	}
	if len(placements) == 0 {
		return nil, nil
	}
	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(placements))
	for n := range placements {
		names = append(names, n)
	}
	sort.Strings(names)

	var checks []linkCheck
	for _, name := range names {
		pl := placements[name]
		c := linkCheck{Piece: name, Box: pl.Box, Pending: pl.Pending}
		path := boxPiecePath(reg, repoRoot, name, pl)
		if path == "" {
			// Pending and never connected: nothing can exist on the box.
			c.Present = false
		} else if err := cli.ValidSSHDest(pl.Box); err != nil {
			c.Error = err.Error()
		} else {
			_, stderr, code, err := sshScript(pl.Box, "test -d "+cli.ShQuote(path))
			switch {
			case err == nil:
				c.Present = true
			case code == 1:
				c.Present = false
			default:
				c.Unreachable = true
				c.Error = fmt.Sprintf("%s: %s", ErrBoxUnreachable, strings.TrimSpace(stderr))
			}
		}
		droppable := !c.Unreachable && c.Error == "" && (!c.Present || c.Pending)
		switch {
		case droppable && !dryRun:
			if err := dropLink(repoRoot, name, fs); err != nil {
				c.Error = err.Error()
			} else {
				c.Dropped = true
			}
		case droppable:
			why := ErrLinkStale
			if c.Pending {
				why = ErrLinkPending
			}
			fmt.Fprintf(os.Stderr, "[dry-run] Would drop placement %s on %s: %v\n", name, pl.Box, why)
		case c.Unreachable:
			fmt.Fprintf(os.Stderr, "%s keeping placement %s: %s\n", cli.GlyphWarn, name, c.Error)
		}
		checks = append(checks, c)
	}
	return checks, nil
}

// droppableLinks counts checks a cleanup apply would remove.
func droppableLinks(checks []linkCheck) int {
	n := 0
	for _, c := range checks {
		if !c.Unreachable && c.Error == "" && (!c.Present || c.Pending) {
			n++
		}
	}
	return n
}

// pendingLinks lists "project/piece" for every pending placement whose box
// is host, across all projects that have connected a box (hidden registry
// rows carry LinkedFrom). Used by `mp remote doctor`.
func pendingLinks(reg registry.Registry, host string) []string {
	fs := adapters.NewOSFS("")
	seen := map[string]bool{}
	var out []string
	for _, p := range reg.Projects {
		if p.Host != host || p.LinkedFrom == "" || seen[p.LinkedFrom] {
			continue
		}
		seen[p.LinkedFrom] = true
		placements, err := piececmd.ReadPlacements(p.LinkedFrom, fs)
		if err != nil {
			continue
		}
		project, _ := registry.ProjectName(p.LinkedFrom)
		for name, pl := range placements {
			if pl.Box == host && pl.Pending {
				out = append(out, project+"/"+name)
			}
		}
	}
	sort.Strings(out)
	return out
}
