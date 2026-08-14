//go:build integration

package piece_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

// resolveTestHandler builds a handler over a fresh temp repo with monkeypuzzle
// config, isolated data dir, and cwd pinned to the repo. Returns the handler
// and the repo root.
func resolveTestHandler(t *testing.T) (*piece.Handler, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-resolve-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	deps := core.Deps{
		FS:      adapters.NewOSFS(""),
		Output:  adapters.NewBufferOutput(),
		Exec:    adapters.NewOSExec(),
		HTTP:    http.DefaultClient,
		Loading: adapters.SetupNoopLoading(),
	}
	return piece.NewHandler(deps), tmpDir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestIntegration_ResolveSwitchTarget_Precedence pins the resolver's full
// precedence matrix on a real repo: trunk, piece by name, piece by sanitized
// name, piece by checked-out branch (reverse lookup on a slash-named adopted
// branch), unadopted local branch, and brand-new name.
func TestIntegration_ResolveSwitchTarget_Precedence(t *testing.T) {
	handler, repo := resolveTestHandler(t)
	ctx := context.Background()

	// An adopted slash-branch: piece "dark-mode" on branch "feat/dark-mode".
	gitIn(t, repo, "branch", "feat/dark-mode", "main")
	if _, err := handler.AdoptPiece(ctx, piece.AdoptPieceInput{RepoRoot: repo, Branch: "feat/dark-mode"}); err != nil {
		t.Fatalf("adopt feat/dark-mode: %v", err)
	}
	// An unadopted local branch.
	gitIn(t, repo, "branch", "spike-idea", "main")

	cases := []struct {
		target    string
		wantKind  piece.SwitchTargetKind
		wantPiece string // for TargetPiece
		wantBr    string // for adopt/new kinds
	}{
		{"main", piece.TargetMain, "", ""},
		{"master", piece.TargetMain, "", ""},
		{"dark-mode", piece.TargetPiece, "dark-mode", ""},
		{"Dark Mode", piece.TargetPiece, "dark-mode", ""}, // sanitized-name match
		{"feat/dark-mode", piece.TargetPiece, "dark-mode", ""},
		{"spike-idea", piece.TargetAdoptLocal, "", "spike-idea"},
		{"feat/brand-new", piece.TargetNew, "", "feat/brand-new"},
	}
	for _, tc := range cases {
		got, err := handler.ResolveSwitchTarget(ctx, repo, tc.target)
		if err != nil {
			t.Errorf("resolve %q: %v", tc.target, err)
			continue
		}
		if got.Kind != tc.wantKind {
			t.Errorf("resolve %q: kind = %v, want %v", tc.target, got.Kind, tc.wantKind)
			continue
		}
		if tc.wantPiece != "" && (got.Piece == nil || got.Piece.Name != tc.wantPiece) {
			t.Errorf("resolve %q: piece = %+v, want name %q", tc.target, got.Piece, tc.wantPiece)
		}
		if tc.wantBr != "" && got.Branch != tc.wantBr {
			t.Errorf("resolve %q: branch = %q, want %q", tc.target, got.Branch, tc.wantBr)
		}
	}

	// A piece shadowed by nothing still derives a sensible new-piece name.
	res, err := handler.ResolveSwitchTarget(ctx, repo, "feat/brand-new")
	if err != nil {
		t.Fatalf("resolve feat/brand-new: %v", err)
	}
	if res.PieceName != "brand-new" {
		t.Errorf("TargetNew piece name = %q, want %q", res.PieceName, "brand-new")
	}
}

// TestIntegration_ResolveSwitchTarget_PieceBeatsBranch pins the ambiguity rule:
// when a name is both a piece and an unadopted branch, the piece wins.
func TestIntegration_ResolveSwitchTarget_PieceBeatsBranch(t *testing.T) {
	handler, repo := resolveTestHandler(t)
	ctx := context.Background()

	// Piece "shadow" on branch "shadow" (created), plus a *different* local
	// branch also named... can't duplicate names; instead: create piece "shadow"
	// from a branch "feat/shadow", then create an unadopted local branch
	// "shadow". Target "shadow" must resolve to the piece, not the branch.
	gitIn(t, repo, "branch", "feat/shadow", "main")
	if _, err := handler.AdoptPiece(ctx, piece.AdoptPieceInput{RepoRoot: repo, Branch: "feat/shadow"}); err != nil {
		t.Fatalf("adopt feat/shadow: %v", err)
	}
	gitIn(t, repo, "branch", "shadow", "main")

	res, err := handler.ResolveSwitchTarget(ctx, repo, "shadow")
	if err != nil {
		t.Fatalf("resolve shadow: %v", err)
	}
	if res.Kind != piece.TargetPiece || res.Piece == nil || res.Piece.Name != "shadow" {
		t.Errorf("piece should beat unadopted branch: got kind=%v piece=%+v", res.Kind, res.Piece)
	}
}

// TestIntegration_ListPieces_PopulatesBranch pins the branch field feeding both
// the reverse lookup and the JSON consumers.
func TestIntegration_ListPieces_PopulatesBranch(t *testing.T) {
	handler, repo := resolveTestHandler(t)
	ctx := context.Background()

	gitIn(t, repo, "branch", "feat/adopted", "main")
	if _, err := handler.AdoptPiece(ctx, piece.AdoptPieceInput{RepoRoot: repo, Branch: "feat/adopted"}); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := handler.CreatePiece(ctx, "plain-piece", piece.CreatePieceOptions{RepoRoot: repo}); err != nil {
		t.Fatalf("create: %v", err)
	}

	pieces, err := handler.ListPieces(ctx, repo)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]string{}
	for _, p := range pieces {
		got[p.Name] = p.Branch
	}
	if got["adopted"] != "feat/adopted" {
		t.Errorf("adopted piece branch = %q, want feat/adopted", got["adopted"])
	}
	if got["plain-piece"] != "plain-piece" {
		t.Errorf("created piece branch = %q, want plain-piece", got["plain-piece"])
	}
}

// TestIntegration_CreatePiece_VerbatimBranch pins NewPieceInput.Branch: the
// branch is created with the exact requested name while the piece keeps the
// derived name, and an existing branch is refused with an adopt hint.
func TestIntegration_CreatePiece_VerbatimBranch(t *testing.T) {
	handler, repo := resolveTestHandler(t)
	ctx := context.Background()

	input := piece.WithNewPieceDefaults(piece.NewPieceInput{Name: "login-rework", Branch: "feat/login-rework"})
	info, err := handler.CreatePieceWithInput(ctx, input, piece.CreatePieceOptions{RepoRoot: repo})
	if err != nil {
		t.Fatalf("create with branch: %v", err)
	}
	if info.Name != "login-rework" {
		t.Errorf("piece name = %q, want login-rework", info.Name)
	}
	out, err := exec.Command("git", "-C", info.WorktreePath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if string(out) != "feat/login-rework\n" {
		t.Errorf("worktree branch = %q, want feat/login-rework", string(out))
	}

	// An existing branch must be refused (adopt is the right verb).
	gitIn(t, repo, "branch", "already-there", "main")
	_, err = handler.CreatePieceWithInput(ctx,
		piece.WithNewPieceDefaults(piece.NewPieceInput{Name: "other", Branch: "already-there"}),
		piece.CreatePieceOptions{RepoRoot: repo})
	if err == nil {
		t.Errorf("creating over an existing branch should error")
	}
}
