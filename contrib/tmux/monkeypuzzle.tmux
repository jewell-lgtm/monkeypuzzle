#!/usr/bin/env bash
# monkeypuzzle tmux plugin entry point.
#
# TPM runs this file once at tmux start. It only binds keys — the actual work
# lives in scripts/, run inside a display-popup so it never disturbs the current
# pane layout. The popup talks to the `mp` CLI: reads via `--json`, and switch /
# create via the stateless API plus MP_TMUX_PLUGIN=1 (set in helpers.sh) so mp
# performs the tmux switch-client itself.

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

switch_key="$(tmux_opt '@monkeypuzzle-switch-key' 'p')"
create_key="$(tmux_opt '@monkeypuzzle-create-key' 'P')"
mp_bin="$(tmux_opt '@monkeypuzzle-bin' 'mp')"
width="$(tmux_opt '@monkeypuzzle-popup-width' '80%')"
height="$(tmux_opt '@monkeypuzzle-popup-height' '70%')"

# -E closes the popup when the script exits; -d gives the script the active
# pane's cwd, which the create flow uses to pre-select the current project.
tmux bind-key "$switch_key" display-popup -E -w "$width" -h "$height" \
	-d '#{pane_current_path}' \
	"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/switch.sh'"

tmux bind-key "$create_key" display-popup -E -w "$width" -h "$height" \
	-d '#{pane_current_path}' \
	"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/create.sh'"
