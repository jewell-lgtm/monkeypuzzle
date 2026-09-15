#!/usr/bin/env bash
# Switch flow: pick a piece (or a project's main session) across all registered
# projects and hand off to `mp switch`, which performs the tmux switch-client.
# A name that matches nothing (or ctrl-o) creates the piece instead, and
# ctrl-d / ctrl-x end a piece's life through finish.sh.
#
# `switch.sh rows` prints the fzf rows and exits; that is the reload command
# the lifecycle keys use (a shell function can't be an fzf reload target).

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_rows reads `mp go --json` on stdin and writes TAB-separated fzf rows:
#   <display>\t<project>\t<piece>\t<worktree>\t<branch>\t<project-path>
# One "(main)" row per existing project, then one row per piece, then one row
# per adoptable branch (matching the `mp go` TUI's branch rows). Only the
# <display> column is shown by fzf; <project> plus <piece>/<branch> drive the
# mp switch call, <worktree> feeds the preview, and <project-path> is the repo
# the lifecycle keys run mp in. A piece checked out on a branch that differs
# from its name shows the branch in its label. Non-project / missing entries
# are skipped. <display> carries an aligned status badge: "◆ draft" / "◆ in
# review" from locally stored PR metadata, "trunk" for main rows, and "branch"
# for adoptable branches.
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
		| [ ($proj + "/" + .label), .badge, $proj, .piece, .worktree, .branch, $path ]
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
				printf "%s\t%s\t%s\t%s\t%s\t%s\n", display, f[3], f[4], f[5], f[6], f[7]
			}
		}
	'
}

rows() {
	"$(mp_bin)" go --json | build_rows
}

main() {
	set -euo pipefail
	if [[ "${1:-}" == "rows" ]]; then
		rows
		return
	fi
	ensure_env

	# The fzf key bindings re-invoke mp and finish.sh from a plain sh, so the
	# binary override must travel through the environment.
	MP_PLUGIN_BIN="$(mp_bin)"
	export MP_PLUGIN_BIN
	local go_json rows projects reload out rc=0 query key selection proj piece branch
	go_json="$("$(mp_bin)" go --json)"
	rows="$(build_rows <<<"$go_json")"
	projects="$(build_project_rows <<<"$go_json")"
	[[ -n "$rows" ]] || die "no pieces or projects found"

	reload="bash \"$DIR/switch.sh\" rows"
	out="$(printf '%s\n' "$rows" | fzf_pick_or_create \
		--with-nth=1 \
		--prompt='piece> ' \
		--header='enter switch · ^O create what you typed · ^D done · ^X abandon' \
		--bind "ctrl-d:execute(bash \"$DIR/finish.sh\" done {2} {3} {6})+reload-sync($reload)" \
		--bind "ctrl-x:execute(bash \"$DIR/finish.sh\" abandon {2} {3} {6})+reload-sync($reload)" \
		--preview='git -C {4} -c color.ui=always status -sb 2>/dev/null; echo; git -C {4} log --oneline -5 2>/dev/null' \
		--preview-window='right,50%')" || rc=$?
	# 1 is "typed a name that matches nothing" — a create, not a failure.
	[[ "$rc" -eq 0 || "$rc" -eq 1 ]] || picker_exit "$rc"

	query="$(sed -n 1p <<<"$out")"
	key="$(sed -n 2p <<<"$out")"
	selection="$(sed -n 3p <<<"$out")"

	# The lifecycle keys never reach here under fzf — they act and reload
	# inside the picker — but the non-interactive seam replays them so the
	# wiring is testable.
	case "$key" in
		ctrl-d) exec bash "$DIR/finish.sh" "done" "$(cut -f2 <<<"$selection")" "$(cut -f3 <<<"$selection")" "$(cut -f6 <<<"$selection")" ;;
		ctrl-x) exec bash "$DIR/finish.sh" abandon "$(cut -f2 <<<"$selection")" "$(cut -f3 <<<"$selection")" "$(cut -f6 <<<"$selection")" ;;
	esac

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
