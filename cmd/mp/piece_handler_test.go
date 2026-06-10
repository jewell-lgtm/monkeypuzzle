package mp

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

func testDeps() core.Deps {
	return core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(io.Discard),
		adapters.NewOSExec(),
		nil,
		nil,
	)
}

// newPieceHandler must fall back to a usable handler — not recurse — when the
// user config is corrupt or names an unknown multiplexer.
func TestNewPieceHandler_FallsBackOnBadConfig(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"corrupt json", `{not json`},
		{"unknown multiplexer", `{"multiplexer":"screen"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			t.Setenv(paths.EnvConfigDir, dir)

			h := newPieceHandler(testDeps())
			if h == nil {
				t.Fatal("expected a handler, got nil")
			}
		})
	}
}
