package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestListAll_SpansRegistryAndSkipsRemote(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MP_DATA_DIR", dataDir)
	registryJSON := `{"version":"1","projects":[
		{"name":"alpha","path":"/repoA","added_at":"2026-07-30T10:00:00Z"},
		{"name":"remote","path":"/on/host","host":"mattmini","added_at":"2026-07-30T10:00:00Z"}
	]}`
	if err := os.WriteFile(filepath.Join(dataDir, "projects.json"), []byte(registryJSON), 0644); err != nil {
		t.Fatal(err)
	}

	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repoA", "p1", map[string]string{"a1": piece.AgentBlocked})
	h := newWaitHandler(fs)

	items, err := h.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (remote project skipped), got %d: %+v", len(items), items)
	}
	if items[0].Project != "alpha" || items[0].Piece != "p1" {
		t.Errorf("unexpected item: %+v", items[0])
	}
}
