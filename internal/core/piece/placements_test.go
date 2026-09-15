package piece_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

func TestPlacements_RoundTripLockAndRemove(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, t.TempDir())
	repo := t.TempDir()
	fs := adapters.NewOSFS("")

	// Missing file = empty store.
	got, err := piece.ReadPlacements(repo, fs)
	if err != nil || len(got) != 0 {
		t.Fatalf("ReadPlacements(missing) = %v, %v", got, err)
	}

	err = piece.UpdatePlacements(repo, fs, func(p piece.Placements) error {
		p["fix-auth"] = piece.Placement{Box: "wire", Pending: true}
		return nil
	})
	if err != nil {
		t.Fatalf("UpdatePlacements: %v", err)
	}
	path := piece.PlacementsPath(repo)
	if path != filepath.Join(repo, ".monkeypuzzle", "placements.json") {
		t.Errorf("PlacementsPath = %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("placements.json not written: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}

	// Flip pending → placed, preserving other entries.
	err = piece.UpdatePlacements(repo, fs, func(p piece.Placements) error {
		pl := p["fix-auth"]
		pl.Pending = false
		pl.RemotePath = "/home/u/.local/share/mp/api/.monkeypuzzle/pieces/fix-auth"
		pl.RemoteProject = "/home/u/.local/share/mp/api"
		p["fix-auth"] = pl
		p["other"] = piece.Placement{Box: "box2"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = piece.ReadPlacements(repo, fs)
	if err != nil {
		t.Fatal(err)
	}
	if got["fix-auth"].Pending || got["fix-auth"].RemotePath == "" || got["other"].Box != "box2" {
		t.Errorf("round trip = %+v", got)
	}
	if on := got.On("wire"); len(on) != 1 || on[0] != "fix-auth" {
		t.Errorf("On(wire) = %v", on)
	}

	// Lock is held during Update: a second lock attempt blocks until release.
	unlock, err := piece.LockPlacements(repo, fs)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = piece.RemovePlacement(repo, "other", fs)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("RemovePlacement did not wait for the lock")
	default:
	}
	unlock()
	<-done

	if err := piece.RemovePlacement(repo, "fix-auth", fs); err != nil {
		t.Fatal(err)
	}
	// Empty store removes the file.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("empty placements.json should be removed, stat err = %v", err)
	}
	// Removing a missing name is fine.
	if err := piece.RemovePlacement(repo, "nope", fs); err != nil {
		t.Errorf("RemovePlacement(missing) = %v", err)
	}
}

func TestPlacements_CorruptFileErrors(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repo := "/repo"
	_ = fs.MkdirAll("/repo/.monkeypuzzle", 0o755)
	_ = fs.WriteFile(piece.PlacementsPath(repo), []byte("{nope"), 0o644)
	if _, err := piece.ReadPlacements(repo, fs); err == nil {
		t.Error("corrupt placements.json should error, not be silently dropped")
	}
}

func seedPlacements(t *testing.T, fs core.FS, repo string, p piece.Placements) {
	t.Helper()
	if err := piece.WritePlacements(repo, p, fs); err != nil {
		t.Fatal(err)
	}
}

func TestHandler_ListPieces_IncludesPlaced(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	handler := piece.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: mockExec})
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	repo := "/test-repo"
	_ = fs.MkdirAll(filepath.Join(repo, ".monkeypuzzle", "pieces", "local-one"), 0o755)
	seedPlacements(t, fs, repo, piece.Placements{
		"on-wire": {Box: "wire", RemotePath: "/home/u/api/.monkeypuzzle/pieces/on-wire", Cached: &piece.PieceListItem{Parent: "local-one", Branch: "on-wire"}},
		"half":    {Box: "wire", Pending: true},
	})

	pieces, err := handler.ListPieces(context.Background(), repo)
	if err != nil {
		t.Fatalf("ListPieces: %v", err)
	}
	byName := map[string]piece.PieceListItem{}
	for _, p := range pieces {
		byName[p.Name] = p
	}
	if len(byName) != 3 {
		t.Fatalf("got %d pieces, want 3: %+v", len(byName), pieces)
	}
	if l := byName["local-one"]; l.IsPlaced() || l.State != "" {
		t.Errorf("local piece polluted: %+v", l)
	}
	w := byName["on-wire"]
	if w.Host != "wire" || w.WorktreePath != "/home/u/api/.monkeypuzzle/pieces/on-wire" || w.State != piece.PlacedState || w.Parent != "local-one" || w.Branch != "on-wire" {
		t.Errorf("placed row = %+v", w)
	}
	if h := byName["half"]; h.State != "pending" || h.Parent != "main" || h.WorktreePath != "" {
		t.Errorf("pending row = %+v", h)
	}
	if got := piece.LocalPieces(pieces); len(got) != 1 || got[0].Name != "local-one" {
		t.Errorf("LocalPieces = %+v", got)
	}

	// No pieces dir at all: placed rows still list.
	repo2 := "/other-repo"
	seedPlacements(t, fs, repo2, piece.Placements{"solo": {Box: "wire"}})
	pieces, err = handler.ListPieces(context.Background(), repo2)
	if err != nil || len(pieces) != 1 || pieces[0].Name != "solo" || !pieces[0].IsPlaced() {
		t.Errorf("placed-only ListPieces = %+v, %v", pieces, err)
	}
}

func TestHandler_CreatePiece_RejectsPlacedName(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	handler := piece.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: mockExec})
	repo := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(repo+"/.git\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repo+"\n"), nil)
	seedPlacements(t, fs, repo, piece.Placements{"taken": {Box: "wire"}})

	_, err := handler.CreatePiece(context.Background(), "taken", piece.CreatePieceOptions{})
	if !errors.Is(err, piece.ErrPieceExists) {
		t.Fatalf("err = %v, want ErrPieceExists", err)
	}
	taken, err := handler.PieceExists(repo, "taken")
	if err != nil || !taken {
		t.Errorf("PieceExists(taken) = %v, %v", taken, err)
	}
	free, _ := handler.PieceExists(repo, "free")
	if free {
		t.Error("PieceExists(free) = true")
	}
	// Local worktree names collide too.
	_ = fs.MkdirAll("/repo/.monkeypuzzle/pieces/local", 0o755)
	if _, err := handler.CreatePiece(context.Background(), "local", piece.CreatePieceOptions{}); !errors.Is(err, piece.ErrPieceExists) {
		t.Errorf("local collision err = %v", err)
	}
}

func TestHandler_SwitchAndAbandon_RefusePlaced(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	handler := piece.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: mockExec})
	repo := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(repo+"/.git\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repo+"\n"), nil)
	seedPlacements(t, fs, repo, piece.Placements{"remote": {Box: "wire", RemotePath: "/box/path"}})

	if _, err := handler.SwitchPiece(context.Background(), "remote"); !errors.Is(err, piece.ErrPiecePlaced) {
		t.Errorf("SwitchPiece(placed) err = %v, want ErrPiecePlaced", err)
	}
	if _, err := handler.AbandonPiece(context.Background(), "remote", piece.AbandonOptions{}); !errors.Is(err, piece.ErrPiecePlaced) {
		t.Errorf("AbandonPiece(placed) err = %v, want ErrPiecePlaced", err)
	}
}
