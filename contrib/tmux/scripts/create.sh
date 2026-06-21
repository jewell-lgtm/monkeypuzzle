#!/usr/bin/env bash
# Create flow: pick a target project, name the piece, and hand off to
# `mp create`, which creates the worktree + tmux session and switches to it.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_project_rows reads `mp go --json` on stdin and writes TAB-separated rows:
#   <display>\t<project>\t<path>
# one per existing, initialised project. Missing / non-project entries skipped.
build_project_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| [ .name, .name, .path ]
		| @tsv
	'
}

# project_for_cwd prints the name of the project whose path is a prefix of the
# pane cwd (passed in $1), so the picker can pre-select it. Empty if none match.
project_for_cwd() {
	local cwd="$1" rows="$2" name path
	while IFS=$'\t' read -r _ name path; do
		[[ -n "$path" ]] || continue
		if [[ "$cwd" == "$path" || "$cwd" == "$path"/* ]]; then
			printf '%s' "$name"
			return
		fi
	done <<<"$rows"
}

main() {
	set -euo pipefail
	ensure_env

	local rows selection proj_path name prompt query
	rows="$("$(mp_bin)" go --json | build_project_rows)"
	[[ -n "$rows" ]] || die "no registered projects — run 'mp init' first"

	query="$(project_for_cwd "${PWD:-}" "$rows")"

	selection="$(printf '%s\n' "$rows" | fzf_pick \
		--with-nth=1 \
		--prompt='project> ' \
		--query="$query")" || exit 0
	[[ -n "$selection" ]] || exit 0
	proj_path="$(cut -f3 <<<"$selection")"

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
