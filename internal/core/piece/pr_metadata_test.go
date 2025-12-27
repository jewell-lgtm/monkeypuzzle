package piece_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestWriteAndReadPRMetadata(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	// Create .monkeypuzzle directory
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	metadata := piece.PRMetadata{
		PRNumber:   123,
		PRURL:      "https://github.com/owner/repo/pull/123",
		Branch:     "feature-branch",
		BaseBranch: "main",
		CreatedAt:  time.Date(2025, 1, 27, 10, 0, 0, 0, time.UTC),
		IssuePath:  "issues/my-issue.md",
	}

	// Write metadata
	if err := piece.WritePRMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePRMetadata failed: %v", err)
	}

	// Read metadata back
	readMetadata, err := piece.ReadPRMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("ReadPRMetadata failed: %v", err)
	}

	// Verify fields
	if readMetadata.PRNumber != 123 {
		t.Errorf("expected PRNumber 123, got %d", readMetadata.PRNumber)
	}
	if readMetadata.PRURL != "https://github.com/owner/repo/pull/123" {
		t.Errorf("expected PRURL 'https://github.com/owner/repo/pull/123', got %q", readMetadata.PRURL)
	}
	if readMetadata.Branch != "feature-branch" {
		t.Errorf("expected Branch 'feature-branch', got %q", readMetadata.Branch)
	}
	if readMetadata.BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got %q", readMetadata.BaseBranch)
	}
	if readMetadata.IssuePath != "issues/my-issue.md" {
		t.Errorf("expected IssuePath 'issues/my-issue.md', got %q", readMetadata.IssuePath)
	}
}

func TestReadPRMetadata_FileNotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	_, err := piece.ReadPRMetadata(worktreePath, fs)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadPRMetadata_InvalidJSON(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	// Create .monkeypuzzle directory
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)

	// Write invalid JSON
	metadataPath := filepath.Join(worktreePath, ".monkeypuzzle", "pr-metadata.json")
	_ = fs.WriteFile(metadataPath, []byte("not valid json"), 0644)

	_, err := piece.ReadPRMetadata(worktreePath, fs)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWritePRMetadata_CreatesDirIfMissing(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	metadata := piece.PRMetadata{
		PRNumber:   456,
		PRURL:      "https://github.com/owner/repo/pull/456",
		Branch:     "test-branch",
		BaseBranch: "main",
		CreatedAt:  time.Now(),
	}

	// Don't create .monkeypuzzle directory - WritePRMetadata should create it
	if err := piece.WritePRMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePRMetadata failed: %v", err)
	}

	// Verify file exists
	metadataPath := filepath.Join(worktreePath, ".monkeypuzzle", "pr-metadata.json")
	data, err := fs.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	var readMetadata piece.PRMetadata
	if err := json.Unmarshal(data, &readMetadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if readMetadata.PRNumber != 456 {
		t.Errorf("expected PRNumber 456, got %d", readMetadata.PRNumber)
	}
}

func TestPRMetadata_WithoutIssuePath(t *testing.T) {
	fs := adapters.NewMemoryFS()
	worktreePath := "/workdir"

	metadata := piece.PRMetadata{
		PRNumber:   789,
		PRURL:      "https://github.com/owner/repo/pull/789",
		Branch:     "standalone-branch",
		BaseBranch: "develop",
		CreatedAt:  time.Now(),
		// IssuePath intentionally omitted
	}

	if err := piece.WritePRMetadata(worktreePath, metadata, fs); err != nil {
		t.Fatalf("WritePRMetadata failed: %v", err)
	}

	readMetadata, err := piece.ReadPRMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("ReadPRMetadata failed: %v", err)
	}

	if readMetadata.IssuePath != "" {
		t.Errorf("expected empty IssuePath, got %q", readMetadata.IssuePath)
	}
}
