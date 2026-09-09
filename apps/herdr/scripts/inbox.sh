#!/usr/bin/env bash
# Inbox picker: fzf over `mp inbox --json` — every piece in every registered
# project, in mp's own order (your rank, then urgency; snoozed rows last).
# Enter hands off to `mp switch` like the open picker; the extra keys each
# run one `mp inbox …` verb and reload the list. The plugin only renders and
# binds keys — ordering, urgency, snoozing and switching are all mp's.
#
# `inbox.sh rows [mp inbox flags]` prints the fzf rows and exits; that is the
# reload command the key bindings use (a shell function can't be an fzf
# reload target).

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# build_inbox_rows reads `mp inbox --json` on stdin and writes TAB-separated
# fzf rows:
#   <display>\t<key>\t<project>\t<piece>\t<worktree>\t<note>\t<pr_url>
# Only <display> is shown: rank, urgency, project/piece, agent status, PR and
# note — the same cells as mp's own `mp inbox` table (snoozed rows show "zz"
# for rank there too). <key> is what every `mp inbox …` verb takes and what
# fzf tracks the cursor by across reloads; <project>/<piece> drive the mp
# switch call; the rest feed the preview. Rows arrive already ordered by mp
# with snoozed rows at the bottom; a snoozed row is only dimmed here.
build_inbox_rows() {
	jq -r '
		def pad($n): . + ((" " * ($n - length)) // "");
		def glyph:
		  if . == "blocked" then "⚠ blocked"
		  elif . == "review" then "• review"
		  elif . == "merged" then "✓ merged"
		  else "  " + . end;
		def pr_cell:
		  if .pr == null then ""
		  elif .merged then "merged"
		  elif .pr.draft then "#\(.pr.number) draft"
		  else "#\(.pr.number) \(.pr.state)" end;
		def note_cell: (.note // "") | if length > 40 then .[0:39] + "…" else . end;
		(.rows // []) as $rows
		| ([$rows[].key | length] | max // 0) as $kw
		| $rows[]
		| (.snoozed // (.snoozed_until != null)) as $snoozed
		| ( [ (if $snoozed then "zz" else (.rank | tostring) end | pad(2)),
		      (.urgency | glyph | pad(9)),
		      (.key | pad($kw)),
		      ((.agent_status // "") | pad(7)),
		      (pr_cell | pad(10)),
		      note_cell ]
		    | join("  ") | sub("\\s+$"; "") ) as $display
		| [ (if $snoozed then "\u001b[2m" + $display + "\u001b[0m" else $display end),
		    .key, .project, .piece, .worktree_path, (.note // ""), (.pr.url // "") ]
		| @tsv
	'
}

rows() {
	"$(mp_bin)" inbox --json "$@" | build_inbox_rows
}

main() {
	set -euo pipefail
	if [[ "${1:-}" == "rows" ]]; then
		shift
		rows "$@"
		return
	fi
	ensure_env

	# The fzf key bindings re-invoke mp and this script from a plain sh, so
	# the binary override must travel through the environment.
	MP_PLUGIN_BIN="$(mp_bin)"
	export MP_PLUGIN_BIN
	local list reload selection proj piece
	list="$(rows)"
	[[ -n "$list" ]] || die "inbox is empty"

	reload="bash \"$DIR/inbox.sh\" rows"
	selection="$(printf '%s\n' "$list" | fzf_pick \
		--ansi --with-nth=1 --layout=reverse --track --id-nth=2 \
		--prompt='inbox> ' \
		--header='enter switch · ^K/^J move · ^T top · ^S snooze 2h · ^U unsnooze · ^R refresh' \
		--bind "ctrl-k:execute-silent(\"\$MP_PLUGIN_BIN\" inbox move {2} --up)+reload-sync($reload)" \
		--bind "ctrl-j:execute-silent(\"\$MP_PLUGIN_BIN\" inbox move {2} --down)+reload-sync($reload)" \
		--bind "ctrl-t:execute-silent(\"\$MP_PLUGIN_BIN\" inbox move {2} --top)+reload-sync($reload)" \
		--bind "ctrl-s:execute-silent(\"\$MP_PLUGIN_BIN\" inbox snooze {2} --for 2h)+reload-sync($reload)" \
		--bind "ctrl-u:execute-silent(\"\$MP_PLUGIN_BIN\" inbox snooze {2} --clear)+reload-sync($reload)" \
		--bind "ctrl-r:reload-sync($reload --refresh)" \
		--preview='[ -n {6} ] && printf "note: %s\n\n" {6}; [ -n {7} ] && printf "%s\n\n" {7}; git -C {5} -c color.ui=always status -sb 2>/dev/null; echo; git -C {5} log --oneline -5 2>/dev/null' \
		--preview-window='right,50%')" || exit 0
	[[ -n "$selection" ]] || exit 0

	proj="$(cut -f3 <<<"$selection")"
	piece="$(cut -f4 <<<"$selection")"
	exec "$(mp_bin)" switch --project "$proj" --piece "$piece"
}

# Run main only when executed directly, so the test runner can source this
# file and call build_inbox_rows in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
