#!/usr/bin/env bash
# Preview pane for the go.sh picker: preview.sh <type> <path> <arg>.
# main/piece rows show the worktree's status + recent commits, branch rows the
# branch's recent commits, create rows a one-line description.

type="${1:-}"
path="${2:-}"
arg="${3:-}"

case "$type" in
create)
	printf 'Create a new piece in %s\n' "$path"
	;;
branch)
	git -C "$path" -c color.ui=always log --oneline -5 "$arg" 2>/dev/null ||
		printf '%s: remote branch (not fetched yet — adopting it will fetch)\n' "$arg"
	;;
*)
	git -C "$path" -c color.ui=always status -sb 2>/dev/null
	echo
	git -C "$path" log --oneline -5 2>/dev/null
	;;
esac
