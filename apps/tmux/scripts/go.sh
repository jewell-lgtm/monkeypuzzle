#!/usr/bin/env bash
# The one-stop picker: every context switch behind a single chord. One fzf list
# covers switching to any piece or project main session, adopting a branch as a
# piece, and creating a new piece — visible "(+ new piece)" rows or ctrl-n. The
# single `mp go --json` call is the entire data model: rows, the "current:"
# header, the * you-are-here marker, and ordering all derive from it.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_rows reads `mp go --json` on stdin and writes TAB-separated fzf rows:
#   <display>\t<type>\t<project>\t<arg>\t<path>
# type is one of main|piece|create|branch; arg is the piece or branch name the
# switch/adopt call needs; path feeds the preview (and, for create rows, the cd
# before `mp create`). Only <display> is shown by fzf.
#
# Blocks are ordered: current project first, then projects with pieces by most
# recent piece activity, then piece-less projects in registry order. Within a
# block: (main), pieces (newest first, as mp emits them), (+ new piece), then
# adoptable branches. The current piece (or main) carries a " *" marker; pieces
# with live agents carry the same status icons as the agent picker. Remote
# (host:) projects get their main row only — their pieces live elsewhere.
build_rows() {
	jq -r '
		(.current.project // "") as $cp
		| (.current.piece // "") as $cpc
		| { blocked: "🔴", working: "⚡", done: "✅", idle: "💤" } as $icons
		| [ .projects[] | select(.exists and .is_project) ]
		| ( map(select(.name == $cp))
		    + ( map(select(.name != $cp and ((.pieces // []) | length) > 0))
		        | sort_by((.pieces // []) | map(.mod_time // "") | max) | reverse )
		    + map(select(.name != $cp and ((.pieces // []) | length) == 0)) )
		| .[]
		| .name as $proj
		| .path as $ppath
		| ((.host // "") == "") as $local
		| (
		    [ { d: ($proj + "/(main)" + (if $proj == $cp and $cpc == "" then " *" else "" end)),
		        t: "main", a: "", p: $ppath } ]
		    + ( (.pieces // []) | map(
		        { d: ($proj + "/" + .name
		              + (if (.agent_status // "") != "" then " " + ($icons[.agent_status] // "?") else "" end)
		              + (if $proj == $cp and .name == $cpc then " *" else "" end)),
		          t: "piece", a: .name, p: .worktree_path } ) )
		    + ( if $local
		        then [ { d: ($proj + "/(+ new piece)"), t: "create", a: "", p: $ppath } ]
		        else [] end )
		    + ( if $local
		        then ( (.branches // []) | map(
		            { d: ($proj + "/(branch: " + .name + ")"), t: "branch", a: .name, p: $ppath } ) )
		        else [] end )
		  )[]
		| [ .d, .t, $proj, .a, .p ]
		| @tsv
	'
}

# current_label reads `mp go --json` on stdin and prints "project/piece",
# "project", or nothing, for the picker header.
current_label() {
	jq -r '.current // empty | .project + (if .piece then "/" + .piece else "" end)'
}

# project_path_for prints the local path of the named project from the JSON in
# $2, empty when unknown or remote (a create can only cd into a local path).
project_path_for() {
	jq -r --arg p "$1" '
		[ .projects[] | select(.name == $p and ((.host // "") == "")) | .path ]
		| first // empty
	' <<<"$2"
}

# create_flow prompts for a name (blank falls through to a free-form prompt for
# `mp create --prompt`) and hands off to mp. The cd is load-bearing: mp create
# resolves the target project from the cwd.
create_flow() {
	local proj_path="$1" prefill="$2" name prompt
	name="$(prompt_input MP_PLUGIN_INPUT_NAME 'New piece name (blank to describe instead): ' "$prefill")" || exit 0
	cd "$proj_path" || die "cannot cd to $proj_path"
	if [[ -n "$name" ]]; then
		exec "$(mp_bin)" create --name "$name"
	fi
	prompt="$(prompt_input MP_PLUGIN_INPUT_PROMPT 'Describe the piece: ')" || exit 0
	[[ -n "$prompt" ]] || exit 0
	exec "$(mp_bin)" create --prompt "$prompt"
}

main() {
	set -euo pipefail
	ensure_env

	local json rows cur header out query key row
	json="$("$(mp_bin)" go --json)"
	rows="$(build_rows <<<"$json")"
	[[ -n "$rows" ]] || die "no pieces or projects found"

	header='enter: switch · ctrl-n: new piece · esc: quit'
	cur="$(current_label <<<"$json")"
	[[ -n "$cur" ]] && header="current: $cur │ $header"

	# Esc still prints the query line (exit 130), so don't die on non-zero here.
	out="$(printf '%s\n' "$rows" | fzf_pick_expect \
		--with-nth=1 \
		--prompt='mp> ' \
		--header="$header" \
		--print-query \
		--expect=ctrl-n \
		--preview="'$DIR/preview.sh' {2} {5} {4}" \
		--preview-window='right,50%')" || true

	local -a lines=()
	mapfile -t lines <<<"$out"
	query="${lines[0]:-}"
	key="${lines[1]:-}"
	row="${lines[2]:-}"

	# ctrl-n creates in the highlighted row's project (any row type names one),
	# falling back to the current project; the typed query prefills the name.
	if [[ "$key" == "ctrl-n" ]]; then
		local proj path
		if [[ -n "$row" ]]; then
			proj="$(cut -f3 <<<"$row")"
		else
			proj="$(jq -r '.current.project // ""' <<<"$json")"
			query=""
		fi
		[[ -n "$proj" ]] || exit 0
		path="$(project_path_for "$proj" "$json")"
		[[ -n "$path" ]] || die "cannot create a piece in remote project $proj"
		create_flow "$path" "$query"
	fi

	[[ -n "$row" ]] || exit 0
	local type proj arg path
	type="$(cut -f2 <<<"$row")"
	proj="$(cut -f3 <<<"$row")"
	arg="$(cut -f4 <<<"$row")"
	path="$(cut -f5 <<<"$row")"

	case "$type" in
	piece) exec "$(mp_bin)" switch --project "$proj" --piece "$arg" ;;
	branch) exec "$(mp_bin)" switch --project "$proj" --branch "$arg" ;;
	create) create_flow "$path" "" ;;
	*) exec "$(mp_bin)" switch --project "$proj" ;;
	esac
}

# Run main only when executed directly, so the test runner can source this file
# and call build_rows / current_label in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
