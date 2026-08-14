#!/usr/bin/env bash
# Shared helpers for the monkeypuzzle tmux plugin scripts.
#
# Sourced by switch.sh and create.sh. This file defines functions and one
# export only — it performs no work at source time — so the test runner can
# source the scripts and call their build_* functions without tripping guards.

# The explicit signal that tells mp to manage the tmux session (switch-client /
# new-session) even though the plugin drives mp through the stateless API with
# no controlling TTY. Only the plugin sets this; agents never do. See AGENTS.md.
export MP_TMUX_PLUGIN=1

# mp_bin prints the mp binary to invoke: the MP_PLUGIN_BIN override that
# monkeypuzzle.tmux bakes in from @monkeypuzzle-bin, else "mp" from PATH.
mp_bin() {
	printf '%s' "${MP_PLUGIN_BIN:-mp}"
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
		command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
	done
}

# ensure_env verifies we are inside tmux and the tools we need are present.
ensure_env() {
	[[ -n "${TMUX:-}" ]] || die "not inside a tmux session"
	require_cmd "$(mp_bin)" jq fzf
}

# focus_agent moves the client to an agent: straight to its recorded pane when
# the piece session is alive, else through `mp switch` (which creates the
# session). A pane target resolves its window too, so focus lands exactly on
# the agent even in a split layout. The optional project arg scopes the
# fallback for cross-project rows.
focus_agent() {
	local session="$1" pane="$2" piece="$3" project="${4:-}"
	if [[ -n "$session" ]] && tmux has-session -t "=$session" 2>/dev/null; then
		tmux switch-client -t "=$session"
		if [[ -n "$pane" ]]; then
			tmux select-window -t "$pane" 2>/dev/null
			tmux select-pane -t "$pane" 2>/dev/null
		fi
		return 0
	fi
	if [[ -n "$project" ]]; then
		exec "$(mp_bin)" switch --project "$project" --piece "$piece"
	fi
	exec "$(mp_bin)" switch --piece "$piece"
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

# fzf_pick reads TAB-delimited rows on stdin and prints the chosen row verbatim
# (all fields, including hidden ones used to act on the selection). Extra args
# are forwarded to fzf for display/preview tuning. When MP_PLUGIN_FILTER is set
# (used by the test runner) it bypasses the interactive UI and selects the
# best fuzzy match for that query, so the flow is drivable without a TTY.
fzf_pick() {
	if [[ -n "${MP_PLUGIN_FILTER:-}" ]]; then
		fzf --delimiter=$'\t' --filter="$MP_PLUGIN_FILTER" | head -n1
	else
		fzf --delimiter=$'\t' "$@"
	fi
}
