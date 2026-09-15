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
	setup_path
	local dependency_error
	dependency_error="$(require_cmd "$(mp_bin)" 2>&1)" || {
		tmux display-message "monkeypuzzle: $dependency_error"
		return 1
	}
	# $1 is the invoking pane's cwd — kept as a sane cwd for mp.
	cd "${1:-.}" 2>/dev/null || true

	local err rc
	err="$("$(mp_bin)" agent focus --blocked --all 2>&1 1>/dev/null)"
	rc=$?
	[[ -n "$err" ]] || return "$rc"

	# The known soft case gets its own friendly wording; any other stderr
	# (a real failure — mp missing, registry unreadable, ...) is relayed
	# verbatim rather than vanishing silently, since a run-shell binding has
	# no other way to tell the user something went wrong.
	if [[ "$err" == *"no blocked agents"* ]]; then
		tmux display-message "monkeypuzzle: no blocked agents"
	else
		tmux display-message "monkeypuzzle: $err"
	fi
	return "$rc"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
