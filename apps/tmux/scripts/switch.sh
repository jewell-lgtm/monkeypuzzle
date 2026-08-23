#!/usr/bin/env bash
# Switch flow: pick a piece (or a project's main session) across all registered
# projects and hand off to `mp switch`, which performs the tmux switch-client.

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
# / missing entries are skipped. <display> carries an aligned status badge:
# "◆ draft" / "◆ in review" from the piece's locally-stored PR metadata (no
# forge round-trip), "trunk" for main rows, "branch" for adoptable branches.
build_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| .name as $proj
		| .path as $path
		| ( [ { label: "(main)", badge: "trunk", piece: "", worktree: $path, branch: "" } ]
		    + ( (.pieces // []) | map({
		          label: (if (.branch // "") != "" and .branch != .name then .name + "  [" + .branch + "]" else .name end),
		          badge: (if (.pr_draft // false) then "◆ draft"
		                  elif (.pr_number // 0) > 0 then "◆ in review"
		                  else "" end),
		          piece: .name, worktree: .worktree_path, branch: ""
		        }) )
		    + ( (.branches // []) | map({
		          label: .name, badge: "branch",
		          piece: "", worktree: $path, branch: .name
		        }) ) )
		| .[]
		| [ ($proj + "/" + .label), .badge, $proj, .piece, .worktree, .branch ]
		| @tsv
	' | align_rows
}

# align_rows pads the label column to a shared width and folds the badge into
# the display field, keeping the hidden selector fields in their positions.
align_rows() {
	awk -F'\t' '
		{ rows[NR] = $0; if (length($1) > max) max = length($1) }
		END {
			for (i = 1; i <= NR; i++) {
				split(rows[i], f, "\t")
				display = f[1]
				if (f[2] != "")
					display = f[1] sprintf("%" (max - length(f[1]) + 2) "s", "") f[2]
				printf "%s\t%s\t%s\t%s\t%s\n", display, f[3], f[4], f[5], f[6]
			}
		}
	'
}

main() {
	set -euo pipefail
	ensure_env

	local rows out key selection proj piece branch
	rows="$("$(mp_bin)" go --json | build_rows)"
	[[ -n "$rows" ]] || die "no pieces or projects found"

	out="$(printf '%s\n' "$rows" | fzf_pick \
		--with-nth=1 \
		--prompt='piece> ' \
		--header='↵ switch · ^P new piece' \
		--expect=ctrl-p \
		--preview='git -C {4} -c color.ui=always status -sb 2>/dev/null; echo; git -C {4} log --oneline -5 2>/dev/null' \
		--preview-window='right,50%')" || exit 0
	[[ -n "$out" ]] || exit 0

	if [[ -n "${MP_PLUGIN_FILTER:-}" ]]; then
		# --filter mode (test seam) has no --expect key line.
		selection="$out"
	else
		key="${out%%$'\n'*}"
		selection="${out#*$'\n'}"
		if [[ "$key" == "ctrl-p" ]]; then
			exec "$DIR/create.sh"
		fi
	fi
	[[ -n "$selection" ]] || exit 0

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
