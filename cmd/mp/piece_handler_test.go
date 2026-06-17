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

// interactiveSessionContext gates all tmux session management on stdin being a
// TTY AND $TMUX being set. The $TMUX half is deterministic: with $TMUX unset it
// must report non-interactive regardless of the stdin TTY state — a terminal
// outside tmux still gets the worktree path, never an attach. (The stdin-TTY
// half depends on the ambient stdin and is covered end-to-end, not here, since
// go test inherits whatever stdin the runner has.)
func TestInteractiveSessionContext_FalseWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if interactiveSessionContext() {
		t.Fatal("expected non-interactive when $TMUX is unset")
	}
}

// A non-interactive context (agents, scripts) must always get the no-op
// multiplexer, regardless of a valid tmux user config.
func TestChooseMultiplexer_NonInteractiveIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"multiplexer":"tmux"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(paths.EnvConfigDir, dir)

	if mux := chooseMultiplexer(testDeps(), false); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("non-interactive must yield no-op multiplexer, got %T", mux)
	}
}

// An interactive context with a valid tmux config uses the real multiplexer.
func TestChooseMultiplexer_InteractiveUsesConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"multiplexer":"tmux"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(paths.EnvConfigDir, dir)

	if mux := chooseMultiplexer(testDeps(), true); adapters.IsNoopMultiplexer(mux) {
		t.Fatal("interactive with tmux config should use the tmux multiplexer, got no-op")
	}
}

// chooseMultiplexer must fall back to the no-op multiplexer — not recurse or
// panic — when the user config is corrupt or names an unknown multiplexer, even
// when interactive. Piece commands must keep working with a broken config.
func TestChooseMultiplexer_FallsBackOnBadConfig(t *testing.T) {
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

			// interactive=true so we get past the gate and exercise the
			// config-resolution fallbacks.
			if mux := chooseMultiplexer(testDeps(), true); !adapters.IsNoopMultiplexer(mux) {
				t.Fatalf("expected no-op multiplexer on bad config, got %T", mux)
			}
		})
	}
}
