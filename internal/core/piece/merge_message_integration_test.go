//go:build integration

package piece_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

// The unit tests around buildSquashCommitMessage ran entirely on a mocked exec,
// so they asserted whatever string the code happened to produce — and passed
// while every real merge discarded the author's commit body. These drive git
// for real and assert on the commit that actually lands.

// mergeMessageRepo builds a project with one piece and returns the repo root,
// the handler, and the piece's worktree.
func mergeMessageRepo(t *testing.T, name string) (string, *piece.Handler, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	repoRoot, err := os.MkdirTemp("", "mp-msg-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(repoRoot) })
	setupGitRepo(t, repoRoot)
	setupMonkeypuzzleConfig(t, repoRoot)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	deps := core.Deps{FS: adapters.NewOSFS(""), Output: adapters.NewBufferOutput(), Exec: adapters.NewOSExec()}
	handler := piece.NewHandlerWithMultiplexer(deps, newRecordingMux(false))
	info, err := handler.CreatePiece(context.Background(), name, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece: %v", err)
	}
	return repoRoot, handler, info.WorktreePath
}

// commitIn writes a file and commits it with the exact message given.
func commitIn(t *testing.T, worktree, file, body, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(worktree, file), []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", message}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = worktree
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[0], err, out)
		}
	}
}

// headMessage returns the full message of the branch tip.
func headMessage(t *testing.T, repoRoot, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%B", ref)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// A piece with one commit is the common case, and its message is the whole
// point of having written one. It must reach the trunk unaltered — not
// demoted to a bullet under an invented "feat: <piece>" subject.
func TestIntegration_MergePiece_SingleCommitKeepsItsMessageVerbatim(t *testing.T) {
	repoRoot, handler, worktree := mergeMessageRepo(t, "verbatim")

	const message = `fix(cleanup): don't announce a dry-run while applying one

Every invocation ran a preview pass first, so a caller who had already
opted in got the preview's output anyway.

Co-Authored-By: Someone <s@example.com>`
	commitIn(t, worktree, "work.txt", "work", message)

	if _, err := handler.MergePiece(context.Background(), worktree, piece.WithMergeDefaults(piece.MergeInput{})); err != nil {
		t.Fatalf("MergePiece: %v", err)
	}

	got := headMessage(t, repoRoot, "main")
	if got != strings.TrimSpace(message) {
		t.Errorf("the author's message did not survive the squash.\n--- got ---\n%s\n--- want ---\n%s", got, strings.TrimSpace(message))
	}
	// The specific regressions: a fake subject, and the body vanishing.
	if strings.HasPrefix(got, "feat: verbatim") {
		t.Error("the squash invented a subject from the piece name")
	}
	if !strings.Contains(got, "Co-Authored-By: Someone <s@example.com>") {
		t.Error("the trailer was dropped")
	}
}

// With several commits there is no single authored subject, so the piece name
// stands in — but every message must still survive in full, and mp must not
// assert a conventional-commit type it cannot know.
func TestIntegration_MergePiece_SeveralCommitsKeepEveryBody(t *testing.T) {
	repoRoot, handler, worktree := mergeMessageRepo(t, "several")

	first := "feat(a): add the thing\n\nWhy the thing is needed."
	second := "fix(b): correct the thing\n\nWhat was wrong with it."
	commitIn(t, worktree, "a.txt", "a", first)
	commitIn(t, worktree, "b.txt", "b", second)

	if _, err := handler.MergePiece(context.Background(), worktree, piece.WithMergeDefaults(piece.MergeInput{})); err != nil {
		t.Fatalf("MergePiece: %v", err)
	}

	got := headMessage(t, repoRoot, "main")
	for _, want := range []string{
		"Why the thing is needed.",
		"What was wrong with it.",
		"feat(a): add the thing",
		"fix(b): correct the thing",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("squash dropped %q:\n%s", want, got)
		}
	}
	// A piece holding a feat and a fix is neither; claiming one is what made
	// the old format misleading.
	if strings.HasPrefix(got, "feat:") || strings.HasPrefix(got, "fix:") {
		t.Errorf("squash asserted a conventional-commit type it cannot know:\n%s", got)
	}
	if !strings.HasPrefix(got, "several") {
		t.Errorf("expected the piece name as the stand-in subject, got:\n%s", got)
	}
}
