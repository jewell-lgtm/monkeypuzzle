package registry

import (
	"strings"
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

	if _, err := r.Remove("/repo/a"); err != nil {
		t.Errorf("Remove by path failed: %v", err)
	}
	if _, err := r.Remove("b"); err != nil {
		t.Errorf("Remove by name failed: %v", err)
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected empty after removals, got %d", len(r.Projects))
	}
	if _, err := r.Remove("a"); err == nil {
		t.Error("Remove of already-removed should error")
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

func TestUpsertRemoteDedupeByHostAndPath(t *testing.T) {
	r := Registry{}
	r.Upsert("/repo/api", "api-local")
	if _, added := r.UpsertRemote("wire", "/repo/api", "api"); !added {
		t.Fatal("same path on a host must be a distinct project from the local one")
	}
	if _, added := r.UpsertRemote("wire", "/repo/api", "api-renamed"); added {
		t.Fatal("same (host, path) must dedupe")
	}
	if len(r.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(r.Projects))
	}
	p, ok := r.Find("api-renamed")
	if !ok || p.Host != "wire" {
		t.Errorf("Find(api-renamed) = %+v, %v", p, ok)
	}
}

func TestRemoteHostSurvivesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MP_DATA_DIR", dir)

	r := Registry{}
	r.UpsertRemote("wire", "/home/u/api", "api")
	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Projects[0].Host != "wire" {
		t.Errorf("host lost in round trip: %+v", loaded.Projects[0])
	}
}

func TestFindUniqueAmbiguity(t *testing.T) {
	r := Registry{Projects: []Project{
		{Name: "api", Path: "/local/api"},
		{Name: "api", Path: "/home/u/api", Host: "wire"},
	}}
	if _, err := r.FindUnique("api"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("FindUnique(api) = %v, want ambiguous error", err)
	}
	p, err := r.FindUnique("wire:/home/u/api")
	if err != nil || p.Host != "wire" {
		t.Errorf("FindUnique(host:path) = %+v, %v", p, err)
	}
	p, err = r.FindUnique("/local/api")
	if err != nil || p.Host != "" {
		t.Errorf("FindUnique(path) = %+v, %v", p, err)
	}
	if _, err := r.Remove("api"); err == nil {
		t.Error("Remove of ambiguous name must error, not delete an arbitrary entry")
	}
	if _, err := r.Remove("wire:/home/u/api"); err != nil {
		t.Errorf("Remove(host:path) = %v", err)
	}
	if len(r.Projects) != 1 || r.Projects[0].Host != "" {
		t.Errorf("wrong entry removed: %+v", r.Projects)
	}
}

func TestHiddenRows(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MP_DATA_DIR", dir)

	r := Registry{}
	r.Upsert("/local/api", "api")
	p, added := r.UpsertHidden("wire", "/home/u/.local/share/mp/api", "api@wire", "/local/api")
	if !added || !p.Hidden || p.LinkedFrom != "/local/api" {
		t.Fatalf("UpsertHidden = %+v added=%v", p, added)
	}
	// Re-upserting hidden keeps it hidden and is not a new row.
	if _, added := r.UpsertHidden("wire", "/home/u/.local/share/mp/api", "", "/local/api"); added {
		t.Error("second UpsertHidden added a row")
	}
	if got := r.Visible(); len(got) != 1 || got[0].Path != "/local/api" {
		t.Errorf("Visible = %+v, want only the local row", got)
	}
	// Hidden rows remain addressable.
	if _, err := r.FindUnique("api@wire"); err != nil {
		t.Errorf("FindUnique hidden: %v", err)
	}
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var hidden *Project
	for i := range loaded.Projects {
		if loaded.Projects[i].Host == "wire" {
			hidden = &loaded.Projects[i]
		}
	}
	if hidden == nil || !hidden.Hidden || hidden.LinkedFrom != "/local/api" {
		t.Fatalf("hidden row lost in round trip: %+v", loaded.Projects)
	}

	// An explicit registration promotes the row to visible.
	if _, added := r.UpsertRemote("wire", "/home/u/.local/share/mp/api", "api"); added {
		t.Error("UpsertRemote over hidden added a row")
	}
	if len(r.Visible()) != 2 {
		t.Errorf("after UpsertRemote, Visible = %d rows, want 2", len(r.Visible()))
	}
	// UpsertHidden over a visible row never hides it.
	r.UpsertHidden("wire", "/home/u/.local/share/mp/api", "", "/local/api")
	if len(r.Visible()) != 2 {
		t.Error("UpsertHidden hid a user-registered row")
	}
}
