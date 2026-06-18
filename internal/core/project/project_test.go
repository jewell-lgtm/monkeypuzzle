package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
)

// seedRegistry registers each name=>path pair and persists the registry.
func seedRegistry(t *testing.T, entries map[string]string) {
	t.Helper()
	var reg registry.Registry
	for name, path := range entries {
		reg.Upsert(path, name)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
}

func names(projects []registry.Project) map[string]bool {
	m := make(map[string]bool, len(projects))
	for _, p := range projects {
		m[p.Name] = true
	}
	return m
}

// TestPruneStale_HappyPath is the outside-in happy path: one project's
// directory was deleted, the other still exists. PruneStale removes only the
// deleted one and persists the change.
func TestPruneStale_HappyPath(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	live := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "gone") // never created

	seedRegistry(t, map[string]string{"live": live, "gone": gone})

	removed, err := PruneStale(false)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if len(removed) != 1 || removed[0].Name != "gone" {
		t.Fatalf("removed = %v, want [gone]", names(removed))
	}

	// The registry on disk must now contain only the live project.
	remaining, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Name != "live" {
		t.Fatalf("remaining = %v, want [live]", remaining)
	}
}

func TestPruneStale_DryRunLeavesRegistryUntouched(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	gone := filepath.Join(t.TempDir(), "gone")
	seedRegistry(t, map[string]string{"gone": gone})

	removed, err := PruneStale(true)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if len(removed) != 1 || removed[0].Name != "gone" {
		t.Fatalf("removed = %v, want [gone] reported", names(removed))
	}

	remaining, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("dry-run modified registry: remaining = %v", remaining)
	}
}

func TestPruneStale_NothingToPrune(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	live := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	seedRegistry(t, map[string]string{"live": live})

	removed, err := PruneStale(false)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", names(removed))
	}
}

func TestPruneStale_PathIsFileNotDir(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	// A registry path that points at a regular file is not a valid repo root
	// and should be pruned.
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRegistry(t, map[string]string{"afile": file})

	removed, err := PruneStale(false)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if len(removed) != 1 || removed[0].Name != "afile" {
		t.Fatalf("removed = %v, want [afile]", names(removed))
	}
}

func TestPruneStale_EmptyRegistry(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	removed, err := PruneStale(false)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", names(removed))
	}
}
