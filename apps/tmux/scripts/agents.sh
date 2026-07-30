#!/usr/bin/env bash
# Agent picker: fzf over the current project's live agents (blocked first, as
# `mp agent list` sorts them), preview the agent's pane, and focus it on
# selection.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_agent_rows reads `mp agent list --json` on stdin and writes
# TAB-separated fzf rows:
#   <display>\t<session>\t<pane>\t<id>\t<piece>
# Only <display> is shown; the rest drive preview and focus.
build_agent_rows() {
	jq -r '
		(.agents // [])[]
		| ( { blocked: "🔴", working: "⚡", done: "✅", idle: "💤" }[.status] // "?" ) as $icon
		| [ ($icon + " " + .piece + " · " + (.kind // "agent") + " " + .id),
		    .session_name, (.pane // ""), .id, .piece ]
		| @tsv
	'
}

main() {
	set -euo pipefail
	ensure_env

	local rows selection session pane piece
	rows="$("$(mp_bin)" agent list --json | build_agent_rows)"
	[[ -n "$rows" ]] || die "no live agents in this project"

	selection="$(printf '%s\n' "$rows" | fzf_pick \
		--with-nth=1 \
		--prompt='agent> ' \
		--preview='if [[ -n {3} ]]; then tmux capture-pane -p -t {3} 2>/dev/null || echo "(pane gone)"; else echo "(no pane recorded)"; fi' \
		--preview-window='right,60%')" || exit 0
	[[ -n "$selection" ]] || exit 0

	session="$(cut -f2 <<<"$selection")"
	pane="$(cut -f3 <<<"$selection")"
	piece="$(cut -f5 <<<"$selection")"
	focus_agent "$session" "$pane" "$piece"
}

# Run main only when executed directly, so the test runner can source this file
# and call build_agent_rows in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
