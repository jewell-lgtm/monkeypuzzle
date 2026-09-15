#!/usr/bin/env bash
# Step through the inbox: `mp inbox next` / `mp inbox prev` from the piece
# the pane stands in — the "what's in progress?" cycle without a picker.
# Bound to run-shell, so feedback goes through tmux display-message: mp does
# the stepping and the switch-client itself; this script only relays its
# stderr ("… is the only piece in the inbox; staying put", or a failure).

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
	local dir="${1:-}"
	[[ "$dir" == next || "$dir" == prev ]] || die "usage: step.sh next|prev [cwd]"
	# $2 is the invoking pane's cwd: mp resolves "the piece you stand in"
	# from it, so it must be the pane's directory, not the tmux server's.
	cd "${2:-.}" 2>/dev/null || true

	local err rc
	err="$("$(mp_bin)" inbox "$dir" 2>&1 1>/dev/null)"
	rc=$?
	[[ -n "$err" ]] || return "$rc"
	tmux display-message "monkeypuzzle: $err"
	return "$rc"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
