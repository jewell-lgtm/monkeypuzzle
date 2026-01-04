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
