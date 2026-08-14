#!/usr/bin/env bash
# Jump straight to the first blocked agent across every registered project —
# the "answer whoever needs me" chord. No picker: bound to run-shell, so
# feedback goes through tmux display-message. One mp invocation does all the
# work (selection + focus/switch); this script only relays its "nothing
# blocked" case to a visible message, since a run-shell binding has no popup.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

main() {
	set -uo pipefail
	# $1 is the invoking pane's cwd — kept as a sane cwd for mp.
	cd "${1:-.}" 2>/dev/null || true

	local err
	err="$("$(mp_bin)" agent focus --blocked --all 2>&1 1>/dev/null)"
	if [[ "$err" == *"no blocked agents"* ]]; then
		tmux display-message "monkeypuzzle: no blocked agents"
	fi
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
