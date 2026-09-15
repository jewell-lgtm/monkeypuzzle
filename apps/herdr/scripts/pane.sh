#!/usr/bin/env bash
# Keep failed picker output visible until dismissed. Running the picker as a
# child also catches failures from exec mp, set -e, and dependency checks.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
case "${1:-}" in
	open|create|adopt|inbox) ;;
	*) printf 'Unknown picker: %s\n' "${1:-}" >&2; exit 2 ;;
esac
bash "$DIR/$1.sh"
rc=$?
if [[ "$rc" -ne 0 ]]; then
	printf '\nMonkeypuzzle picker failed (exit %s). See the error above.\n' "$rc" >&2
	if [[ -t 0 ]]; then
		printf 'Press Enter to close.\n' >&2
		read -r _ || true
	fi
fi
exit "$rc"
