package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

func testExec() *adapters.OSExec {
	return adapters.NewOSExec()
}

func writeUserConfig(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(paths.EnvConfigDir, dir)
}

// The gate's TTY half depends on the ambient stdin and is covered end-to-end,
// not here (go test inherits whatever stdin the runner has). Every case below
// is deterministic regardless of the stdin TTY state: either MP_TMUX_PLUGIN=1
// makes the first gate pass unconditionally, or InSession()=false forces no-op
// after it.

// A configured tmux multiplexer must still yield the no-op when the adapter
// does not report in-session ($TMUX unset) — a terminal outside tmux gets the
// worktree path, never an attach.
func TestChooseMultiplexer_NotInSessionIsNoop(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"tmux"}`)
	t.Setenv("TMUX", "")
	t.Setenv("MP_TMUX_PLUGIN", "1") // pass the TTY gate deterministically
	if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("outside a tmux session must yield no-op multiplexer, got %T", mux)
	}
}

// A configured zellij multiplexer outside a zellij session must also degrade
// to the no-op — the gate consults the adapter's own InSession, not $TMUX.
func TestChooseMultiplexer_ZellijNotInSessionIsNoop(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"zellij"}`)
	t.Setenv("ZELLIJ", "")
	t.Setenv("MP_TMUX_PLUGIN", "1") // pass the TTY gate deterministically
	if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("outside a zellij session must yield no-op multiplexer, got %T", mux)
	}
}

// Same for cmux: no $CMUX_WORKSPACE_ID means no-op.
func TestChooseMultiplexer_CmuxNotInSessionIsNoop(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"cmux"}`)
	t.Setenv("CMUX_WORKSPACE_ID", "")
	t.Setenv("MP_TMUX_PLUGIN", "1") // pass the TTY gate deterministically
	if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("outside a cmux workspace must yield no-op multiplexer, got %T", mux)
	}
}

// Same for herdr: no $HERDR_ENV means no-op.
func TestChooseMultiplexer_HerdrNotInSessionIsNoop(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"herdr"}`)
	t.Setenv("HERDR_ENV", "")
	t.Setenv("MP_MUX_PLUGIN", "1") // pass the TTY gate deterministically
	if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("outside a herdr workspace must yield no-op multiplexer, got %T", mux)
	}
}

// MP_TMUX_PLUGIN=1 substitutes for the stdin-TTY requirement: the tmux plugin
// drives mp through the stateless API (no controlling TTY) but still wants mp
// to manage the session. With $TMUX set and a valid tmux config, the real
// multiplexer is chosen.
func TestChooseMultiplexer_PluginOverrideUsesConfig(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"tmux"}`)
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	t.Setenv("MP_TMUX_PLUGIN", "1")
	if mux := chooseMultiplexer(testExec()); adapters.IsNoopMultiplexer(mux) {
		t.Fatal("plugin-driven with tmux config inside tmux should use the tmux multiplexer, got no-op")
	}
}

// The provider-neutral MP_MUX_PLUGIN=1 does the same for herdr's companion
// plugin — herdr is in pluginCapableProviders, so the override is honored.
func TestChooseMultiplexer_MuxPluginOverrideHerdr(t *testing.T) {
	writeUserConfig(t, `{"multiplexer":"herdr"}`)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("MP_TMUX_PLUGIN", "")
	t.Setenv("MP_MUX_PLUGIN", "1")
	if mux := chooseMultiplexer(testExec()); adapters.IsNoopMultiplexer(mux) {
		t.Fatal("plugin-driven with herdr config inside herdr should use the herdr multiplexer, got no-op")
	}
}

// The plugin override never enables providers without a companion plugin:
// zellij in-session but TTY-less stays no-op even with MP_MUX_PLUGIN=1.
func TestChooseMultiplexer_MuxPluginOverrideNotPluginCapable(t *testing.T) {
	if cli.IsTerminal() {
		t.Skip("stdin is a TTY; the plugin-capability gate is only reachable TTY-less")
	}
	writeUserConfig(t, `{"multiplexer":"zellij"}`)
	t.Setenv("ZELLIJ", "session-1")
	t.Setenv("MP_MUX_PLUGIN", "1")
	if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
		t.Fatalf("zellij is not plugin-capable; expected no-op, got %T", mux)
	}
}

// chooseMultiplexer must fall back to the no-op multiplexer — not recurse or
// panic — when the user config is corrupt or names an unknown multiplexer.
// Piece commands must keep working with a broken config.
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
			writeUserConfig(t, tc.content)
			t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
			t.Setenv("MP_TMUX_PLUGIN", "1") // get past the gate to the config fallbacks
			if mux := chooseMultiplexer(testExec()); !adapters.IsNoopMultiplexer(mux) {
				t.Fatalf("expected no-op multiplexer on bad config, got %T", mux)
			}
		})
	}
}
