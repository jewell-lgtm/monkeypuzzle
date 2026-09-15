package piece_test

import (
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

func TestParseMergeStrategy(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    piece.MergeStrategy
		wantErr bool
	}{
		{"", "", false}, // unset: defer to the next layer
		{"local", piece.MergeLocal, false},
		{"forge", piece.MergeForge, false},
		{"  FORGE  ", piece.MergeForge, false},
		{"github", "", true},
		{"remote", "", true},
	} {
		got, err := piece.ParseMergeStrategy(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseMergeStrategy(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMergeStrategy(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// writeProjectConfig lays down a monkeypuzzle.json declaring the given strategy
// ("" writes a config with none, exercising the fall-through).
func writeProjectConfig(t *testing.T, fs core.FS, repoRoot, strategy string) {
	t.Helper()
	if err := fs.MkdirAll(projectdir.Dir(repoRoot), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{"version":"1","project":{"name":"alpha"},"pr":{"provider":"github"}}`
	if strategy != "" {
		body = `{"version":"1","project":{"name":"alpha"},"pr":{"provider":"github"},"merge":{"strategy":"` + strategy + `"}}`
	}
	if err := fs.WriteFile(projectdir.ConfigFilePath(repoRoot), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestResolveMergeStrategy_Precedence(t *testing.T) {
	const repoRoot = "/repos/alpha"

	for _, tc := range []struct {
		name        string
		project     string // "" = project declares none
		userDefault string
		override    string
		want        piece.MergeStrategy
	}{
		{"nothing configured falls back to local", "", "", "", piece.MergeLocal},
		{"user default applies when the project is silent", "", "forge", "", piece.MergeForge},
		{"project wins over the user default", "local", "forge", "", piece.MergeLocal},
		{"project applies with no user default", "forge", "", "", piece.MergeForge},
		{"the per-call override wins over both", "forge", "forge", "local", piece.MergeLocal},
		{"the override wins the other way too", "local", "local", "forge", piece.MergeForge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := adapters.NewMemoryFS()
			writeProjectConfig(t, fs, repoRoot, tc.project)
			got, err := piece.ResolveMergeStrategy(repoRoot, fs, tc.override, tc.userDefault)
			if err != nil {
				t.Fatalf("ResolveMergeStrategy: %v", err)
			}
			if got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// A project with no config at all (not yet initialised, or unreadable) must not
// break merging — it falls through to the user default like a silent project.
func TestResolveMergeStrategy_MissingProjectConfig(t *testing.T) {
	fs := adapters.NewMemoryFS()
	got, err := piece.ResolveMergeStrategy("/repos/nowhere", fs, "", "forge")
	if err != nil {
		t.Fatalf("ResolveMergeStrategy: %v", err)
	}
	if got != piece.MergeForge {
		t.Errorf("= %q, want %q", got, piece.MergeForge)
	}
}

func TestResolveMergeStrategy_RejectsGarbage(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := filepath.Clean("/repos/alpha")
	writeProjectConfig(t, fs, repoRoot, "sideways")

	if _, err := piece.ResolveMergeStrategy(repoRoot, fs, "", ""); err == nil {
		t.Error("a project declaring an unknown strategy must fail loudly, not silently merge locally")
	}
	writeProjectConfig(t, fs, repoRoot, "")
	if _, err := piece.ResolveMergeStrategy(repoRoot, fs, "", "sideways"); err == nil {
		t.Error("an unknown user-level strategy must fail loudly")
	}
	if _, err := piece.ResolveMergeStrategy(repoRoot, fs, "sideways", ""); err == nil {
		t.Error("an unknown per-call override must fail loudly")
	}
}
