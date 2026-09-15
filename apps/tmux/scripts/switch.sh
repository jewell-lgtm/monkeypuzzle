#!/usr/bin/env bash
# Switch flow: pick a piece (or a project's main session) across all registered
# projects and hand off to `mp switch`, which performs the tmux switch-client.
# A name that matches nothing (or ctrl-o) creates the piece instead.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_rows reads `mp go --json` on stdin and writes TAB-separated fzf rows:
#   <display>\t<project>\t<piece>\t<worktree>\t<branch>
# One "(main)" row per existing project, then one row per piece, then one row
# per adoptable branch (matching the `mp go` TUI's branch rows). Only the
# <display> column is shown by fzf; <project> plus <piece>/<branch> drive the
# mp switch call and <worktree> feeds the preview. A piece checked out on a
# branch that differs from its name shows the branch in its label. Non-project
# / missing entries are skipped.
build_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| .name as $proj
		| .path as $path
		| ( [ { label: "(main)", piece: "", worktree: $path, branch: "" } ]
		    + ( (.pieces // []) | map({
		          label: (if (.branch // "") != "" and .branch != .name then .name + "  [" + .branch + "]" else .name end),
		          piece: .name, worktree: .worktree_path, branch: ""
		        }) )
		    + ( (.branches // []) | map({
		          label: (.name + "  (branch)"),
		          piece: "", worktree: $path, branch: .name
		        }) ) )
		| .[]
		| [ ($proj + "/" + .label), $proj, .piece, .worktree, .branch ]
		| @tsv
	'
}

main() {
	set -euo pipefail
	ensure_env

	local go_json rows projects out rc=0 query key selection proj piece branch
	go_json="$("$(mp_bin)" go --json)"
	rows="$(build_rows <<<"$go_json")"
	projects="$(build_project_rows <<<"$go_json")"
	[[ -n "$rows" ]] || die "no pieces or projects found"

	out="$(printf '%s\n' "$rows" | fzf_pick_or_create \
		--with-nth=1 \
		--prompt='piece> ' \
		--header='enter: switch — a name nothing matches creates it  ctrl-o: create what you typed' \
		--preview='git -C {4} -c color.ui=always status -sb 2>/dev/null; echo; git -C {4} log --oneline -5 2>/dev/null' \
		--preview-window='right,50%')" || rc=$?
	# 1 is "typed a name that matches nothing" — a create, not a failure.
	[[ "$rc" -eq 0 || "$rc" -eq 1 ]] || picker_exit "$rc"

	query="$(sed -n 1p <<<"$out")"
	key="$(sed -n 2p <<<"$out")"
	selection="$(sed -n 3p <<<"$out")"

	# A create key, or Enter with nothing matched: mint what was typed.
	if [[ -n "$key" || -z "$selection" ]]; then
		create_from_query "$query" "$projects"
	fi

	proj="$(cut -f2 <<<"$selection")"
	piece="$(cut -f3 <<<"$selection")"
	branch="$(cut -f5 <<<"$selection")"

	if [[ -n "$branch" ]]; then
		exec "$(mp_bin)" switch --project "$proj" --branch "$branch"
	fi
	if [[ -n "$piece" ]]; then
		exec "$(mp_bin)" switch --project "$proj" --piece "$piece"
	fi
	exec "$(mp_bin)" switch --project "$proj"
}

# Run main only when executed directly, so the test runner can source this file
# and call build_rows in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
