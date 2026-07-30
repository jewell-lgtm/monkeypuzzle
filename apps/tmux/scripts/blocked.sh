#!/usr/bin/env bash
# Jump straight to the first blocked agent in the current project — the
# "answer whoever needs me" chord. No picker: bound to run-shell, so feedback
# goes through tmux display-message.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# pick_blocked reads `mp agent list --json` on stdin and prints the first
# blocked agent as <session>\t<pane>\t<piece>, or nothing.
pick_blocked() {
	jq -r '
		first((.agents // [])[] | select(.status == "blocked")) // empty
		| [ .session_name, (.pane // ""), .piece ]
		| @tsv
	'
}

main() {
	set -uo pipefail
	# $1 is the invoking pane's cwd: scopes `mp agent list` to that project.
	cd "${1:-.}" 2>/dev/null || true

	local row
	row="$("$(mp_bin)" agent list --json 2>/dev/null | pick_blocked)"
	if [[ -z "$row" ]]; then
		tmux display-message "monkeypuzzle: no blocked agents"
		exit 0
	fi
	focus_agent "$(cut -f1 <<<"$row")" "$(cut -f2 <<<"$row")" "$(cut -f3 <<<"$row")"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
