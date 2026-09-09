#!/usr/bin/env bash
# Step through the inbox: `mp inbox next` / `mp inbox prev` from the piece
# the workspace stands in — the "what's in progress?" cycle without a
# picker. Runs as a plain herdr action: mp does the stepping and the
# workspace focus itself; stderr lands in the plugin's action log, and the
# exit code separates the soft "only piece in the inbox" case (0) from a
# genuine failure (1).

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

main() {
	set -uo pipefail
	local dir="${1:-}"
	[[ "$dir" == next || "$dir" == prev ]] || die "usage: step.sh next|prev [cwd]"
	# $2 is the invoking cwd (manual runs): mp resolves "the piece you stand
	# in" from it.
	cd "${2:-.}" 2>/dev/null || true

	local err rc
	err="$("$(mp_bin)" inbox "$dir" 2>&1 1>/dev/null)"
	rc=$?
	[[ -n "$err" ]] || return "$rc"
	printf 'monkeypuzzle: %s\n' "$err" >&2
	return "$rc"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
