#!/usr/bin/env bash
# monkeypuzzle tmux plugin entry point.
#
# TPM runs this file once at tmux start. It only binds keys — the actual work
# lives in scripts/, run inside a display-popup so it never disturbs the current
# pane layout. The popup talks to the `mp` CLI: reads via `--json`, and switch /
# create via the stateless API plus MP_TMUX_PLUGIN=1 (set in helpers.sh) so mp
# performs the tmux switch-client itself.
#
# All actions live in a "monkeypuzzle" key table entered via prefix + the
# table key (default m), so every chord is prefix m <letter> and the plugin
# claims exactly one key in the root prefix table.

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

# tmux_opt prints a user @option value, falling back to a default.
tmux_opt() {
	local value
	value="$(tmux show-option -gqv "$1")"
	if [[ -n "$value" ]]; then
		printf '%s' "$value"
	else
		printf '%s' "$2"
	fi
}

table_key="$(tmux_opt '@monkeypuzzle-key' 'm')"
mp_bin="$(tmux_opt '@monkeypuzzle-bin' 'mp')"
width="$(tmux_opt '@monkeypuzzle-popup-width' '80%')"
height="$(tmux_opt '@monkeypuzzle-popup-height' '70%')"

# prefix + table key enters the monkeypuzzle key table; the next key fires one
# binding below and drops back to the root table.
tmux bind-key "$table_key" switch-client -T monkeypuzzle

# popup binds a monkeypuzzle-table key to a script in a display-popup. -E
# closes the popup when the script exits; -d gives the script the active
# pane's cwd, which scopes project detection to where you're working.
popup() {
	tmux bind-key -T monkeypuzzle "$1" display-popup -E -w "$width" -h "$height" \
		-d '#{pane_current_path}' \
		"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/$2'"
}

popup p switch.sh # pick a piece / project main session
popup c create.sh # create a new piece
popup g branch.sh # paste a branch/piece name: switch, adopt, or create
popup a agents.sh # pick a live agent, focus its pane

# Jump straight to the first blocked agent — no picker, no popup.
tmux bind-key -T monkeypuzzle b run-shell \
	"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/blocked.sh' '#{pane_current_path}'"

# Toggle the sidecar shell split in the current piece.
tmux bind-key -T monkeypuzzle t run-shell \
	"'$CURRENT_DIR/scripts/sidecar.sh' '#{pane_id}' '#{pane_current_path}'"

# Cheat sheet: show this table's bindings.
tmux bind-key -T monkeypuzzle m list-keys -T monkeypuzzle
