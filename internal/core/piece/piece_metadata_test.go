package piece_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestWriteAndReadPieceMetadata(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	// Create .monkeypuzzle directory
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	metadata := piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}

	// Write metadata
	if err := piece.WritePieceMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePieceMetadata failed: %v", err)
	}

	// Read metadata back
	readMetadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("ReadPieceMetadata failed: %v", err)
	}

	// Verify fields
	if readMetadata.Parent != "parent-piece" {
		t.Errorf("expected Parent 'parent-piece', got %q", readMetadata.Parent)
	}
	if readMetadata.CreatedFromBranch != "parent-piece-branch" {
		t.Errorf("expected CreatedFromBranch 'parent-piece-branch', got %q", readMetadata.CreatedFromBranch)
	}
}

func TestReadPieceMetadata_FileNotFound_ReturnsDefault(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	// No metadata file exists - should return default (parent=main)
	metadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	if metadata.Parent != "main" {
		t.Errorf("expected default Parent 'main', got %q", metadata.Parent)
	}
	if metadata.CreatedFromBranch != "" {
		t.Errorf("expected empty CreatedFromBranch for default, got %q", metadata.CreatedFromBranch)
	}
}

func TestReadPieceMetadata_InvalidJSON(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	// Create .monkeypuzzle directory
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	// Write invalid JSON
	metadataPath := filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json")
	_ = fs.WriteFile(metadataPath, []byte("not valid json"), 0644)

	_, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWritePieceMetadata_CreatesDirIfMissing(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	metadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}

	// Don't create .monkeypuzzle directory - WritePieceMetadata should create it
	if err := piece.WritePieceMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePieceMetadata failed: %v", err)
	}

	// Verify file exists
	metadataPath := filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json")
	data, err := fs.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	var readMetadata piece.PieceMetadata
	if err := json.Unmarshal(data, &readMetadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if readMetadata.Parent != "main" {
		t.Errorf("expected Parent 'main', got %q", readMetadata.Parent)
	}
}

func TestPieceMetadata_MainParentDefault(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	metadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}

	if err := piece.WritePieceMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePieceMetadata failed: %v", err)
	}

	readMetadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("ReadPieceMetadata failed: %v", err)
	}

	if readMetadata.Parent != "main" {
		t.Errorf("expected Parent 'main', got %q", readMetadata.Parent)
	}
}

func TestGetPieceChildren_NoChildren(t *testing.T) {
	fs := adapters.NewMemoryFS()

	// Create pieces directory with one piece that has no children
	piecesDir := "/pieces"
	piece1Path := filepath.Join(piecesDir, "piece-1")
	_ = fs.MkdirAll(piece1Path, 0755)

	// piece-1 has parent=main (no other pieces have parent=piece-1)
	metadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}
	_ = piece.WritePieceMetadata(piece1Path, metadata, fs)

	children, err := piece.GetPieceChildren("piece-1", piecesDir, fs)
	if err != nil {
		t.Fatalf("GetPieceChildren failed: %v", err)
	}

	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d: %v", len(children), children)
	}
}

func TestGetPieceChildren_WithChildren(t *testing.T) {
	fs := adapters.NewMemoryFS()

	// Create pieces directory structure
	piecesDir := "/pieces"
	parentPath := filepath.Join(piecesDir, "parent-piece")
	child1Path := filepath.Join(piecesDir, "child-1")
	child2Path := filepath.Join(piecesDir, "child-2")
	unrelatedPath := filepath.Join(piecesDir, "unrelated")

	_ = fs.MkdirAll(parentPath, 0755)
	_ = fs.MkdirAll(child1Path, 0755)
	_ = fs.MkdirAll(child2Path, 0755)
	_ = fs.MkdirAll(unrelatedPath, 0755)

	// Parent has main as parent
	_ = piece.WritePieceMetadata(parentPath, piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}, fs)

	// Child-1 has parent-piece as parent
	_ = piece.WritePieceMetadata(child1Path, piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}, fs)

	// Child-2 has parent-piece as parent
	_ = piece.WritePieceMetadata(child2Path, piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}, fs)

	// Unrelated has main as parent
	_ = piece.WritePieceMetadata(unrelatedPath, piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}, fs)

	children, err := piece.GetPieceChildren("parent-piece", piecesDir, fs)
	if err != nil {
		t.Fatalf("GetPieceChildren failed: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d: %v", len(children), children)
	}

	// Check both children are present (order may vary)
	foundChild1, foundChild2 := false, false
	for _, c := range children {
		if c == "child-1" {
			foundChild1 = true
		}
		if c == "child-2" {
			foundChild2 = true
		}
	}
	if !foundChild1 {
		t.Error("expected to find child-1 in children")
	}
	if !foundChild2 {
		t.Error("expected to find child-2 in children")
	}
}

func TestGetPieceChildren_NoPiecesDir(t *testing.T) {
	fs := adapters.NewMemoryFS()

	// Pieces directory doesn't exist
	children, err := piece.GetPieceChildren("parent", "/nonexistent", fs)
	if err != nil {
		t.Fatalf("expected no error for nonexistent dir, got: %v", err)
	}

	if len(children) != 0 {
		t.Errorf("expected 0 children for nonexistent dir, got %d", len(children))
	}
}

func TestHasChildren_True(t *testing.T) {
	fs := adapters.NewMemoryFS()

	piecesDir := "/pieces"
	parentPath := filepath.Join(piecesDir, "parent-piece")
	childPath := filepath.Join(piecesDir, "child-piece")

	_ = fs.MkdirAll(parentPath, 0755)
	_ = fs.MkdirAll(childPath, 0755)

	_ = piece.WritePieceMetadata(parentPath, piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}, fs)

	_ = piece.WritePieceMetadata(childPath, piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}, fs)

	hasChildren, err := piece.HasChildren("parent-piece", piecesDir, fs)
	if err != nil {
		t.Fatalf("HasChildren failed: %v", err)
	}

	if !hasChildren {
		t.Error("expected HasChildren to return true")
	}
}

func TestHasChildren_False(t *testing.T) {
	fs := adapters.NewMemoryFS()

	piecesDir := "/pieces"
	piecePath := filepath.Join(piecesDir, "lonely-piece")

	_ = fs.MkdirAll(piecePath, 0755)

	_ = piece.WritePieceMetadata(piecePath, piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}, fs)

	hasChildren, err := piece.HasChildren("lonely-piece", piecesDir, fs)
	if err != nil {
		t.Fatalf("HasChildren failed: %v", err)
	}

	if hasChildren {
		t.Error("expected HasChildren to return false")
	}
}

func TestGetPieceChildren_SkipsPiecesWithoutMetadata(t *testing.T) {
	fs := adapters.NewMemoryFS()

	piecesDir := "/pieces"
	parentPath := filepath.Join(piecesDir, "parent")
	noMetaPath := filepath.Join(piecesDir, "no-metadata")
	childPath := filepath.Join(piecesDir, "child")

	_ = fs.MkdirAll(parentPath, 0755)
	_ = fs.MkdirAll(noMetaPath, 0755) // No metadata file
	_ = fs.MkdirAll(childPath, 0755)

	_ = piece.WritePieceMetadata(parentPath, piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}, fs)

	_ = piece.WritePieceMetadata(childPath, piece.PieceMetadata{
		Parent:            "parent",
		CreatedFromBranch: "parent-branch",
	}, fs)

	// no-metadata piece has default parent=main, so won't be a child of parent

	children, err := piece.GetPieceChildren("parent", piecesDir, fs)
	if err != nil {
		t.Fatalf("GetPieceChildren failed: %v", err)
	}

	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d: %v", len(children), children)
	}

	if children[0] != "child" {
		t.Errorf("expected child 'child', got %q", children[0])
	}
}

func TestDefaultPieceMetadata(t *testing.T) {
	def := piece.DefaultPieceMetadata()
	if def.Parent != "main" {
		t.Errorf("expected default Parent 'main', got %q", def.Parent)
	}
	if def.CreatedFromBranch != "" {
		t.Errorf("expected empty default CreatedFromBranch, got %q", def.CreatedFromBranch)
	}
}

func TestNewPieceIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := piece.NewPieceID()
		if id == "" {
			t.Fatal("empty piece id")
		}
		if seen[id] {
			t.Fatalf("duplicate piece id %q", id)
		}
		seen[id] = true
	}
}

// A piece id has to survive being put in a branch name, a tag, or a URL by
// whatever external system records it.
func TestNewPieceIDIsWordSafe(t *testing.T) {
	id := piece.NewPieceID()
	for _, r := range id {
		isAlnum := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !isAlnum {
			t.Errorf("piece id %q contains non-alphanumeric %q", id, r)
		}
	}
}

func TestWritePieceMetadataBackfillsID(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	// Metadata written before ids existed.
	if err := piece.WritePieceMetadata(worktreePath, piece.PieceMetadata{Parent: "main"}, fs); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ID == "" {
		t.Fatal("write did not backfill an id")
	}
}

func TestWritePieceMetadataKeepsExistingID(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	const want = "KEEPTHISID00"
	if err := piece.WritePieceMetadata(worktreePath, piece.PieceMetadata{ID: want, Parent: "main"}, fs); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A later write for an unrelated reason must not re-mint it.
	metadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	metadata.Merged = true
	if err := piece.WritePieceMetadata(worktreePath, *metadata, fs); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got.ID != want {
		t.Errorf("id = %q, want %q", got.ID, want)
	}
}

func TestEnsurePieceIDIsStable(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	// Seed a piece with no id, the way one created before ids looks.
	raw := []byte(`{"parent":"main","created_from_branch":"main"}`)
	if err := fs.WriteFile(filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json"), raw, 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	first, err := piece.EnsurePieceID(worktreePath, fs)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first == "" {
		t.Fatal("ensure returned an empty id")
	}
	second, err := piece.EnsurePieceID(worktreePath, fs)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second != first {
		t.Errorf("id changed between calls: %q then %q", first, second)
	}

	// And the rest of the metadata survived the backfill.
	metadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if metadata.Parent != "main" || metadata.CreatedFromBranch != "main" {
		t.Errorf("backfill clobbered metadata: %+v", metadata)
	}
}

func TestPieceMetadataIDOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(piece.PieceMetadata{Parent: "main"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"parent":"main","created_from_branch":""}` {
		t.Errorf("empty id should be omitted, got %s", data)
	}
}

// ReadPieceMetadata hands back the default for a worktree with no metadata
// file. If that default carried an id, every read would report a different
// one — the opposite of a durable identifier.
func TestReadPieceMetadataWithoutFileHasNoID(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	first, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	second, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if first.ID != "" {
		t.Errorf("a missing metadata file reported id %q; reads must not mint", first.ID)
	}
	if first.ID != second.ID {
		t.Errorf("two reads gave different ids: %q then %q", first.ID, second.ID)
	}
}

func TestDefaultPieceMetadataHasNoID(t *testing.T) {
	if id := piece.DefaultPieceMetadata().ID; id != "" {
		t.Errorf("DefaultPieceMetadata minted id %q", id)
	}
}

// EnsurePieceID is the explicit materialisation path, so it has to work for
// the case it exists for: a worktree with no metadata file at all.
func TestEnsurePieceIDWithoutMetadataFile(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	first, err := piece.EnsurePieceID(worktreePath, fs)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first == "" {
		t.Fatal("ensure returned an empty id")
	}
	second, err := piece.EnsurePieceID(worktreePath, fs)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second != first {
		t.Errorf("id was not persisted: %q then %q", first, second)
	}
}
