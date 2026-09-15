package piece_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

// isolateHistory points the log at a per-test file and returns a reader.
func isolateHistory(t *testing.T) func() []history.Event {
	t.Helper()
	t.Setenv(history.EnvFile, filepath.Join(t.TempDir(), "history.jsonl"))
	return func() []history.Event {
		t.Helper()
		evs, err := history.Read(history.ReadOptions{})
		if err != nil {
			t.Fatalf("history.Read: %v", err)
		}
		return evs
	}
}

func TestHistory_CreatePiece_EmitsPieceCreated_WithoutHookScript(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)
	read := isolateHistory(t)

	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	handler := piece.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: mockExec})

	repoRoot := "/repo"
	pieceName := "feat"
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", pieceName)
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(repoRoot+"/.git\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", "-b", pieceName, worktreePath, "main"}, nil, nil)

	if _, err := handler.CreatePiece(context.Background(), pieceName, piece.CreatePieceOptions{}); err != nil {
		t.Fatalf("CreatePiece: %v", err)
	}

	evs := read()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(evs), evs)
	}
	ev := evs[0]
	if ev.Event != "piece.created" || ev.Project != "repo" || ev.Piece != pieceName || ev.Branch != pieceName || ev.Parent != "main" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.Actor.Kind == "" || ev.TS == "" {
		t.Errorf("actor/ts not filled: %+v", ev)
	}
}

func TestHistory_DonePiece_EmitsPieceDone(t *testing.T) {
	read := isolateHistory(t)

	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	handler := piece.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: mockExec})

	// Real temp root so projectdir resolves the worktree's .monkeypuzzle dir
	// deterministically (see TestHandler_IsBranchMerged_RecordedMarker).
	repoRoot := t.TempDir()
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", "feat")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(filepath.Join(repoRoot, ".git", "worktrees", "feat")+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("feat\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "remove", "--force", worktreePath}, nil, nil)
	if err := piece.WritePieceMetadata(worktreePath, piece.PieceMetadata{Parent: "main", Merged: true}, fs); err != nil {
		t.Fatal(err)
	}

	res, err := handler.DonePiece(context.Background(), worktreePath, piece.DoneInput{MainBranch: "main"})
	if err != nil {
		t.Fatalf("DonePiece: %v", err)
	}
	if !res.Cleaned {
		t.Fatalf("expected cleaned, got %+v", res)
	}

	evs := read()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(evs), evs)
	}
	ev := evs[0]
	if ev.Event != "piece.done" || ev.Piece != "feat" || ev.Branch != "feat" || ev.Project != filepath.Base(repoRoot) {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.Data["merged_via"] != "recorded" {
		t.Errorf("want merged_via=recorded, got %+v", ev.Data)
	}
}

func TestHistory_HookRunner_RecordsCompletedTransitionsOnly(t *testing.T) {
	read := isolateHistory(t)
	runner := piece.NewHookRunner(core.Deps{FS: adapters.NewMemoryFS(), Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()})

	// No hook scripts exist: every call is a no-op for execution, yet the
	// after-* hooks must still leave a trace.
	ctx := piece.HookContext{PieceName: "feat", RepoRoot: "/repo", Branch: "feat", PRNumber: 12, PRURL: "https://x/pr/12", PRBaseBranch: "main"}
	for _, hook := range []string{piece.HookBeforePRCreate, piece.HookAfterPRCreate, piece.HookIsPieceDone, piece.HookAfterPRReady} {
		if err := runner.RunHook(context.Background(), "/repo", hook, ctx); err != nil {
			t.Fatalf("%s: %v", hook, err)
		}
	}
	if err := runner.RunHookDetached("/repo", piece.HookAgentBlocked, piece.HookContext{PieceName: "feat", RepoRoot: "/repo", AgentID: "a1", AgentKind: "claude", AgentStatus: "blocked"}); err != nil {
		t.Fatal(err)
	}

	evs := read()
	var names []string
	for _, ev := range evs {
		names = append(names, ev.Event)
	}
	want := []string{"pr.created", "pr.ready", "agent.blocked"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
	if n, _ := evs[0].Data["pr_number"].(float64); n != 12 || evs[0].Data["base"] != "main" {
		t.Errorf("pr.created data: %+v", evs[0].Data)
	}
	if evs[2].Data["agent_id"] != "a1" || evs[2].Data["agent_status"] != "blocked" {
		t.Errorf("agent.blocked data: %+v", evs[2].Data)
	}
}

func TestHistory_UnwritableLog_DoesNotFailHook(t *testing.T) {
	t.Setenv(history.EnvFile, t.TempDir()) // a directory: open fails
	out := adapters.NewBufferOutput()
	runner := piece.NewHookRunner(core.Deps{FS: adapters.NewMemoryFS(), Output: out, Exec: adapters.NewMockExec()})

	err := runner.RunHook(context.Background(), "/repo", piece.HookAfterPieceUpdate, piece.HookContext{PieceName: "feat", RepoRoot: "/repo"})
	if err != nil {
		t.Fatalf("hook must not fail on history error: %v", err)
	}
	if len(out.Messages) != 1 || out.Messages[0].Type != core.MsgWarning {
		t.Fatalf("expected one warning, got %+v", out.Messages)
	}
}
