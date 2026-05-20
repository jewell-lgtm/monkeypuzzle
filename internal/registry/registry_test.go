package registry

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	r, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected empty registry, got %d", len(r.Projects))
	}
	if r.Version == "" {
		t.Errorf("expected version to be set")
	}
}

func TestUpsertDedupeAndRename(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	r := Registry{}
	if _, added := r.Upsert("/repo/a", "a"); !added {
		t.Fatal("first upsert should add")
	}
	if _, added := r.Upsert("/repo/a", "a-renamed"); added {
		t.Fatal("re-upsert same path should not add")
	}
	if len(r.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(r.Projects))
	}
	if r.Projects[0].Name != "a-renamed" {
		t.Errorf("name = %q, want a-renamed", r.Projects[0].Name)
	}
}

func TestFindAndRemove(t *testing.T) {
	r := Registry{Projects: []Project{
		{Name: "a", Path: "/repo/a"},
		{Name: "b", Path: "/repo/b"},
	}}

	if _, ok := r.Find("a"); !ok {
		t.Error("Find by name failed")
	}
	if _, ok := r.Find("/repo/b"); !ok {
		t.Error("Find by path failed")
	}
	if _, ok := r.Find("nope"); ok {
		t.Error("Find should fail for unknown")
	}

	if _, ok := r.Remove("/repo/a"); !ok {
		t.Error("Remove by path failed")
	}
	if _, ok := r.Remove("b"); !ok {
		t.Error("Remove by name failed")
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected empty after removals, got %d", len(r.Projects))
	}
	if _, ok := r.Remove("a"); ok {
		t.Error("Remove of already-removed should report false")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	r := Registry{}
	r.Upsert("/repo/z", "z")
	r.Upsert("/repo/a", "a")
	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Projects) != 2 {
		t.Fatalf("expected 2, got %d", len(loaded.Projects))
	}
	// Save sorts by name.
	if loaded.Projects[0].Name != "a" || loaded.Projects[1].Name != "z" {
		t.Errorf("unexpected order: %+v", loaded.Projects)
	}
}
