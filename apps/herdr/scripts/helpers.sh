#!/usr/bin/env bash
# Shared helpers for the monkeypuzzle herdr plugin scripts.
#
# Sourced by every script in this directory. This file defines functions and
# one export only — it performs no work at source time — so the test runner
# can source the scripts and call their build_* functions without tripping
# guards.

# The explicit signal that tells mp to manage the herdr workspace (focus /
# create) even though the plugin drives mp through the stateless API with no
# controlling TTY. Only a companion plugin sets this; agents never do.
export MP_MUX_PLUGIN=1

# mp_bin prints the mp binary to invoke: the MP_PLUGIN_BIN override, else
# "mp" from PATH.
mp_bin() {
	printf '%s' "${MP_PLUGIN_BIN:-mp}"
}

# herdr_bin prints the herdr binary: herdr injects HERDR_BIN_PATH into every
# plugin process; "herdr" from PATH is the fallback for manual runs.
herdr_bin() {
	printf '%s' "${HERDR_BIN_PATH:-herdr}"
}

# die prints a message to stderr and exits non-zero.
die() {
	printf 'monkeypuzzle-herdr: %s\n' "$*" >&2
	exit 1
}

# require_cmd ensures every named command resolves on PATH.
require_cmd() {
	local cmd
	for cmd in "$@"; do
		command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
	done
}

# ensure_env verifies we run inside herdr and the tools we need are present.
ensure_env() {
	[[ "${HERDR_ENV:-}" == "1" ]] || die "not inside a herdr session"
	require_cmd "$(mp_bin)" jq fzf
}

# build_project_rows reads `mp go --json` on stdin and writes TAB-separated rows:
#   <display>\t<project>\t<path>
# one per existing, initialised project. Missing / non-project entries skipped.
# Shared by create.sh (picker) and open.sh (cwd pre-selection).
build_project_rows() {
	jq -r '
		.projects[]
		| select(.exists and .is_project)
		| [ .name, .name, .path ]
		| @tsv
	'
}

# project_for_cwd prints the name of the project whose path is a prefix of the
# cwd (passed in $1), given project rows in $2. Empty if none match.
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
