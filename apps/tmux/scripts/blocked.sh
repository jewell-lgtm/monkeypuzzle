#!/usr/bin/env bash
# Jump straight to the first blocked agent across every registered project —
# the "answer whoever needs me" chord. No picker: bound to run-shell, so
# feedback goes through tmux display-message.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# pick_blocked reads `mp agent list --all --json` on stdin and prints the
# first blocked agent as <session>\t<pane>\t<piece>\t<project>, or nothing.
pick_blocked() {
	jq -r '
		first((.agents // [])[] | select(.status == "blocked")) // empty
		| [ .session_name, (.pane // ""), .piece, (.project // "") ]
		| @tsv
	'
}

main() {
	set -uo pipefail
	# $1 is the invoking pane's cwd — kept as a sane cwd for mp.
	cd "${1:-.}" 2>/dev/null || true

	local row
	row="$("$(mp_bin)" agent list --all --json 2>/dev/null | pick_blocked)"
	if [[ -z "$row" ]]; then
		tmux display-message "monkeypuzzle: no blocked agents"
		exit 0
	fi
	focus_agent "$(cut -f1 <<<"$row")" "$(cut -f2 <<<"$row")" "$(cut -f3 <<<"$row")" "$(cut -f4 <<<"$row")"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
