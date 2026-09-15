#!/usr/bin/env bash
# Shared helpers for the monkeypuzzle tmux plugin scripts.
#
# Sourced by every script in this directory. This file defines functions and
# one export only — it performs no work at source time — so the test runner
# can source the scripts and call their build_* functions without tripping
# guards.

# The explicit signal that tells mp to manage the tmux session (switch-client /
# new-session) even though the plugin drives mp through the stateless API with
# no controlling TTY. Only the plugin sets this; agents never do. See AGENTS.md.
export MP_TMUX_PLUGIN=1

# mp_bin prints the mp binary to invoke: the MP_PLUGIN_BIN override that
# monkeypuzzle.tmux bakes in from @monkeypuzzle-bin, else "mp" from PATH.
mp_bin() {
	printf '%s' "${MP_PLUGIN_BIN:-mp}"
}

# Keep the caller's command precedence, then search common user/package installs.
# GUI-launched tmux servers may inherit only the system PATH. Do not source
# shell startup files: they may print output or run interactive commands.
setup_path() {
	local dir
	for dir in "${HOME}/.local/bin" /opt/homebrew/bin /usr/local/bin; do
		case ":${PATH:-}:" in
			*":$dir:"*) ;;
			*) PATH="${PATH:+$PATH:}$dir" ;;
		esac
	done
	export PATH
}

# fzf uses 1 for no match and 130 for cancellation; other errors must surface.
picker_exit() {
	case "$1" in
		1|130) exit 0 ;;
		*) exit "$1" ;;
	esac
}

# die prints a message to stderr and exits non-zero.
die() {
	printf 'monkeypuzzle-tmux: %s\n' "$*" >&2
	exit 1
}

# require_cmd ensures every named command resolves on PATH.
require_cmd() {
	local cmd
	for cmd in "$@"; do
		command -v "$cmd" >/dev/null 2>&1 || die "Required command not found: $cmd
Install the missing command and add its directory to tmux's PATH.
For mp, set @monkeypuzzle-bin in ~/.tmux.conf to its absolute path.
Searched PATH: $PATH"
	done
}

# ensure_env verifies we are inside tmux and the tools we need are present.
ensure_env() {
	setup_path
	[[ -n "${TMUX:-}" ]] || die "not inside a tmux session"
	require_cmd "$(mp_bin)" jq fzf
}

# build_project_rows reads `mp go --json` on stdin and writes TAB-separated rows:
#   <display>\t<project>\t<path>
# one per existing, initialised project. Missing / non-project entries skipped.
# Shared by create.sh (picker) and branch.sh (cwd scoping + fallback picker).
build_project_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| [ .name, .name, .path ]
		| @tsv
	'
}

# project_for_cwd prints the name of the project whose path is a prefix of the
# cwd (passed in $1), given project rows in $2. Empty if none match. The popup
# runs with -d '#{pane_current_path}', so $PWD is the invoking pane's cwd.
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

# project_path prints the path of the project named in $1, given project rows
# in $2. Empty when $1 is empty or names no registered project.
project_path() {
	local want="$1" rows="$2" name path
	[[ -n "$want" ]] || return 0
	while IFS=$'\t' read -r _ name path; do
		if [[ "$name" == "$want" ]]; then
			printf '%s' "$path"
			return
		fi
	done <<<"$rows"
}

# seam_nth translates the caller's --with-nth into the matching --nth, so the
# non-interactive seam below scores the same columns the picker matches on.
# Without it a query could match a hidden field — a worktree or project path —
# that nobody typing into the real picker can see.
seam_nth() {
	local arg
	for arg in "$@"; do
		case "$arg" in
			--with-nth=*)
				printf '%s' "--nth=${arg#--with-nth=}"
				return
				;;
		esac
	done
}

# fzf_pick reads TAB-delimited rows on stdin and prints the chosen row verbatim
# (all fields, including hidden ones used to act on the selection). Extra args
# are forwarded to fzf for display/preview tuning. When MP_PLUGIN_FILTER is set
# (used by the test runner) — to an empty query too — it bypasses the
# interactive UI and selects the best fuzzy match for that query, so the flow
# is drivable without a TTY.
fzf_pick() {
	local nth
	if [[ -n "${MP_PLUGIN_FILTER+x}" ]]; then
		nth="$(seam_nth "$@")"
		# head closes the pipe on the first row, so fzf takes SIGPIPE and
		# pipefail would surface 141 as a picker failure.
		# shellcheck disable=SC2086 # one flag or nothing; never word-split
		fzf --delimiter=$'\t' $nth --filter="$MP_PLUGIN_FILTER" | head -n1 || true
	else
		fzf --delimiter=$'\t' "$@"
	fi
}

# fzf_pick_or_create is fzf_pick with a create affordance. It prints three
# lines: the typed query, the key that closed the picker (empty for Enter,
# otherwise the create key — ctrl-n is fzf's own "next match", so the create
# keys are ctrl-o and alt-enter), and the chosen row. Enter on a query that
# matches no row exits 1 with an empty row — the same create path, not a
# failure — while cancellation still exits 130.
fzf_pick_or_create() {
	local nth
	if [[ -n "${MP_PLUGIN_FILTER+x}" ]]; then
		nth="$(seam_nth "$@")"
		printf '%s\n%s\n' "$MP_PLUGIN_FILTER" "${MP_PLUGIN_KEY:-}"
		# shellcheck disable=SC2086 # one flag or nothing; never word-split
		fzf --delimiter=$'\t' $nth --filter="$MP_PLUGIN_FILTER" | head -n1 || true
		return 0
	fi
	fzf --delimiter=$'\t' --print-query --expect=ctrl-o,alt-enter "$@"
}

# split_project_query splits a picker query into "<project>\t<target>", given
# project rows in $2. The picker's rows read "project/piece", so a query whose
# prefix names a known project scopes to it; everything else keeps the query
# verbatim as the target, since branch names contain slashes too.
split_project_query() {
	local query="$1" rows="$2" prefix rest name
	prefix="${query%%/*}"
	rest="${query#*/}"
	if [[ "$query" == */* && -n "$rest" ]]; then
		while IFS=$'\t' read -r _ name _; do
			if [[ "$name" == "$prefix" ]]; then
				printf '%s\t%s' "$prefix" "$rest"
				return
			fi
		done <<<"$rows"
	fi
	printf '\t%s' "$query"
}

# valid_piece_target accepts what `git check-ref-format` will take as a branch
# name. fzf's search operators (^ $ ! ' | and spaces) are all in git's illegal
# set, so the one pass refuses a filter typed into the picker and lets through
# every name someone can actually have, accents included.
valid_piece_target() {
	case "${1:-}" in
		"" | @ | /* | */ | *//* | .* | */.* | *..* | *. | *.lock | *.lock/*) return 1 ;;
		*[[:space:][:cntrl:]]* | *"@{"*) return 1 ;;
		*"^"* | *'$'* | *"!"* | *"'"* | *'"'* | *"|"* | *"~"*) return 1 ;;
		*":"* | *"?"* | *"*"* | *"["* | *"\\"*) return 1 ;;
	esac
}

# create_from_query turns a picker query into a piece, given project rows in
# $2. The project comes from the query's prefix, else the cwd, else a picker;
# the rest goes to `mp switch --create`, which attaches to a piece or adopts a
# branch of that name rather than failing. An empty query hands off to the full
# create flow — carrying the named project — where a piece can also be
# described as a prompt; a query that is not a usable name is refused rather
# than handed to git.
create_from_query() {
	local query="$1" rows="$2" pair proj target selection dir
	# Nothing typed, or a project prefix with no name after it: no name to
	# create, so ask for one the long way round — carrying the project the
	# query named, so it is not asked for twice or answered differently.
	if [[ -z "$query" || "$query" == */ ]]; then
		dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
		exec bash "$dir/create.sh" "${query%/}"
	fi

	pair="$(split_project_query "$query" "$rows")"
	proj="${pair%%$'\t'*}"
	target="${pair#*$'\t'}"
	valid_piece_target "$target" || die "\"$query\" is a filter, not a name git can branch on — nothing created"
	[[ -n "$proj" ]] || proj="$(project_for_cwd "${PWD:-}" "$rows")"
	if [[ -z "$proj" ]]; then
		selection="$(printf '%s\n' "$rows" | fzf_pick \
			--with-nth=1 \
			--prompt='project> ')" || picker_exit "$?"
		[[ -n "$selection" ]] || exit 0
		proj="$(cut -f2 <<<"$selection")"
	fi

	exec "$(mp_bin)" switch --project "$proj" --create -- "$target"
}
