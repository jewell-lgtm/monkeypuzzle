package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
)

// seedRemoteRegistry writes a registry with one remote and one stale local project
// and points MP_DATA_DIR at it.
func seedRemoteRegistry(t *testing.T) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("MP_DATA_DIR", dataDir)
	reg := `{"version":"1","projects":[
		{"name":"gone","path":"/definitely/not/a/real/path","added_at":"2026-01-01T00:00:00Z"},
		{"name":"api","path":"/home/u/api","host":"wire","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "projects.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneStaleKeepsRemoteProjects(t *testing.T) {
	seedRemoteRegistry(t)
	removed, err := PruneStale(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].Name != "gone" {
		t.Fatalf("removed = %+v, want only the stale local project", removed)
	}
	reg, _ := registry.Load()
	if len(reg.Projects) != 1 || reg.Projects[0].Host != "wire" {
		t.Errorf("remote project was pruned: %+v", reg.Projects)
	}
}

func TestEnrichRemoteSkipsLocalStat(t *testing.T) {
	seedRemoteRegistry(t)
	infos, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		if info.Host != "wire" {
			continue
		}
		if !info.Exists || !info.IsProject || info.Branch != "" || info.PieceCount != 0 {
			t.Errorf("remote enrich = %+v, want exists/is_project true with no local git state", info)
		}
		return
	}
	t.Fatal("remote project missing from List()")
}
