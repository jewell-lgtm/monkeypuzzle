#!/usr/bin/env bash
# Create flow: pick a target project, name the piece, and hand off to
# `mp create`, which creates the worktree + tmux session and switches to it.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_project_rows and project_for_cwd live in helpers.sh (shared with
# branch.sh).
#
# $1, when given, is the project to create in — the switch picker passes the
# one its query named, so the choice is not made twice.

main() {
	set -euo pipefail
	ensure_env

	local rows selection proj_path name prompt query
	rows="$("$(mp_bin)" go --json | build_project_rows)"
	[[ -n "$rows" ]] || die "no registered projects — run 'mp init' first"

	proj_path="$(project_path "${1:-}" "$rows")"
	if [[ -z "$proj_path" ]]; then
		query="$(project_for_cwd "${PWD:-}" "$rows")"

		selection="$(printf '%s\n' "$rows" | fzf_pick \
			--with-nth=1 \
			--prompt='project> ' \
			--query="$query")" || picker_exit "$?"
		[[ -n "$selection" ]] || exit 0
		proj_path="$(cut -f3 <<<"$selection")"
	fi

	read -r -p "New piece name (blank to describe instead): " name || exit 0
	cd "$proj_path"
	if [[ -n "$name" ]]; then
		exec "$(mp_bin)" create --name "$name"
	fi

	read -r -p "Describe the piece: " prompt || exit 0
	[[ -n "$prompt" ]] || exit 0
	exec "$(mp_bin)" create --prompt "$prompt"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
