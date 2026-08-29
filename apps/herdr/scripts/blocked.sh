#!/usr/bin/env bash
# Jump straight to the first blocked agent across every registered project —
# the "answer whoever needs me" action. No popup: one mp invocation does all
# the work (selection + herdr workspace/pane focus). Runs as a plain herdr
# action; stderr lands in the plugin's action log, and the exit code
# separates "nothing blocked" (0, soft) from a genuine failure (1).

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

main() {
	set -uo pipefail
	# $HERDR_PLUGIN_CONTEXT_JSON carries the invoking workspace; mp resolves
	# everything itself, so only a sane cwd matters ($1 for manual runs).
	cd "${1:-.}" 2>/dev/null || true

	local err
	err="$("$(mp_bin)" agent focus --blocked --all 2>&1 1>/dev/null)"
	[[ -n "$err" ]] || return 0

	if [[ "$err" == *"no blocked agents"* ]]; then
		printf 'monkeypuzzle: no blocked agents\n' >&2
		return 0
	fi
	printf 'monkeypuzzle: %s\n' "$err" >&2
	return 1
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
