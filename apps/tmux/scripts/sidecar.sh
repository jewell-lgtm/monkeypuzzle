#!/usr/bin/env bash
# Sidecar shell toggle: a real shell split beside a keyboard-capturing agent
# TUI, in the same worktree. Press once to open (or jump to) the sidecar,
# press again from inside it to close it. Panes are tagged with the
# @mp_sidecar pane option so the toggle survives layout changes.

set -uo pipefail

main() {
	local pane="${1:?pane id required}" path="${2:-.}"

	# From inside the sidecar: close it.
	if [[ "$(tmux show-options -pqv -t "$pane" @mp_sidecar)" == "1" ]]; then
		tmux kill-pane -t "$pane"
		return
	fi

	# A sidecar already open in this window: jump to it.
	local existing
	existing="$(tmux list-panes -F '#{pane_id} #{@mp_sidecar}' | awk '$2 == "1" { print $1; exit }')"
	if [[ -n "$existing" ]]; then
		tmux select-pane -t "$existing"
		return
	fi

	# Open a fresh sidecar below, in the agent pane's cwd (the worktree).
	local new
	new="$(tmux split-window -v -l '30%' -c "$path" -P -F '#{pane_id}')"
	tmux set-option -p -t "$new" @mp_sidecar 1
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
