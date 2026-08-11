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

# chord_indicator prints the status-line format that renders $1 while the
# monkeypuzzle key table is waiting for its second key. Entering a key table
# releases prefix mode, so #{client_prefix} indicators go dark mid-chord —
# this is the missing feedback.
chord_indicator() {
	printf '#{?#{==:#{client_key_table},monkeypuzzle},%s,}' "$1"
}

# inject_indicator replaces every #{monkeypuzzle_chord} placeholder in $1
# with the rendered indicator $2 (tmux has no user-defined format variables,
# so the swap happens once at load — the tmux-prefix-highlight approach).
inject_indicator() {
	printf '%s' "${1//'#{monkeypuzzle_chord}'/$2}"
}

main() {
	local table_key mp_bin width height indicator side value
	table_key="$(tmux_opt '@monkeypuzzle-key' 'm')"
	mp_bin="$(tmux_opt '@monkeypuzzle-bin' 'mp')"
	width="$(tmux_opt '@monkeypuzzle-popup-width' '80%')"
	height="$(tmux_opt '@monkeypuzzle-popup-height' '70%')"

	# prefix + table key enters the monkeypuzzle key table; the next key fires
	# one binding below and drops back to the root table.
	tmux bind-key "$table_key" switch-client -T monkeypuzzle

	# popup binds a monkeypuzzle-table key to a script in a display-popup. -E
	# closes the popup when the script exits; -d gives the script the active
	# pane's cwd, which is what makes `mp go --json` report the current
	# project/piece for the you-are-here header and marker.
	popup() {
		tmux bind-key -T monkeypuzzle "$1" display-popup -E -w "$width" -h "$height" \
			-d '#{pane_current_path}' \
			"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/$2'"
	}

	# p is the one-stop context switch: pieces, project mains, branch adoption,
	# and piece creation all live in one picker (ctrl-n / "(+ new piece)" rows).
	popup p go.sh
	popup a agents.sh # pick a live agent, focus its pane

	# Jump straight to the first blocked agent — no picker, no popup.
	tmux bind-key -T monkeypuzzle b run-shell \
		"MP_PLUGIN_BIN='$mp_bin' '$CURRENT_DIR/scripts/blocked.sh' '#{pane_current_path}'"

	# Toggle the sidecar shell split in the current piece.
	tmux bind-key -T monkeypuzzle t run-shell \
		"'$CURRENT_DIR/scripts/sidecar.sh' '#{pane_id}' '#{pane_current_path}'"

	# Cheat sheet: show this table's bindings.
	tmux bind-key -T monkeypuzzle m list-keys -T monkeypuzzle

	# Mid-chord feedback: swap any #{monkeypuzzle_chord} placeholder the user
	# put in status-left/right for the live indicator. Idempotent — a config
	# reload restores the placeholder before this runs again.
	indicator="$(chord_indicator "$(tmux_opt '@monkeypuzzle-indicator' '#[reverse] mp… #[noreverse]')")"
	for side in status-left status-right; do
		value="$(tmux show-option -gv "$side" 2>/dev/null)"
		if [[ "$value" == *'#{monkeypuzzle_chord}'* ]]; then
			tmux set-option -g "$side" "$(inject_indicator "$value" "$indicator")"
		fi
	done
}

# Run main only when executed directly (TPM / run-shell), so the test runner
# can source this file and unit-test the indicator helpers.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
