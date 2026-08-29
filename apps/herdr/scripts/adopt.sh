#!/usr/bin/env bash
# Adopt flow: pick a local or remote branch from any registered project and
# hand off to `mp switch --branch`, which adopts it as a piece (worktree +
# herdr workspace) on the fly.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_branch_rows reads `mp go --json` on stdin and writes TAB-separated rows:
#   <display>\t<project>\t<branch>\t<path>
# one per adoptable branch. The <path> column feeds the preview.
build_branch_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| .name as $proj
		| .path as $path
		| (.branches // [])[]
		| [ ($proj + "/" + .name), $proj, .name, $path ]
		| @tsv
	'
}

main() {
	set -euo pipefail
	ensure_env

	local rows selection proj branch
	rows="$("$(mp_bin)" go --json | build_branch_rows)"
	[[ -n "$rows" ]] || die "no adoptable branches found"

	selection="$(printf '%s\n' "$rows" | fzf_pick \
		--with-nth=1 \
		--prompt='branch> ' \
		--preview='git -C {4} -c color.ui=always log --oneline -8 {3} 2>/dev/null' \
		--preview-window='right,50%')" || exit 0
	[[ -n "$selection" ]] || exit 0

	proj="$(cut -f2 <<<"$selection")"
	branch="$(cut -f3 <<<"$selection")"
	exec "$(mp_bin)" switch --project "$proj" --branch "$branch"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
