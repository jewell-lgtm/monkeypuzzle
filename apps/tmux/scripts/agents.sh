#!/usr/bin/env bash
# Agent picker: fzf over live agents across every registered project (blocked
# first, as `mp agent list` sorts them), preview the agent's pane, and hand
# focus off to `mp agent focus` on selection — the plugin only builds rows and
# picks; mp does all the resolving and focusing.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_agent_rows reads `mp agent list --all --json` on stdin and writes
# TAB-separated fzf rows:
#   <display>\t<id>\t<pane>
# Only <display> is shown; <pane> feeds the preview, <id> is the sole
# argument `mp agent focus` needs — it resolves session/pane/piece/project
# itself. The icon comes straight from mp's own JSON (single source: the same
# table `mp agent summary` uses), not a second jq lookup.
build_agent_rows() {
	jq -r '
		(.agents // [])[]
		| (.icon // "?") as $icon
		| ( if .project then .project + "/" + .piece else .piece end ) as $label
		| [ ($icon + " " + $label + " · " + (.kind // "agent") + " " + .id),
		    .id, (.pane // "") ]
		| @tsv
	'
}

main() {
	set -euo pipefail
	ensure_env

	local rows selection id
	rows="$("$(mp_bin)" agent list --all --json | build_agent_rows)"
	[[ -n "$rows" ]] || die "no live agents"

	selection="$(printf '%s\n' "$rows" | fzf_pick \
		--with-nth=1 \
		--prompt='agent> ' \
		--preview='tmux capture-pane -p -t {3} 2>/dev/null || echo "(no live pane)"' \
		--preview-window='right,60%')" || exit 0
	[[ -n "$selection" ]] || exit 0

	id="$(cut -f2 <<<"$selection")"
	exec "$(mp_bin)" agent focus "$id" --all
}

# Run main only when executed directly, so the test runner can source this file
# and call build_agent_rows in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
