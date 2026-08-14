#!/usr/bin/env bash
# Branch jump: paste (or type) a branch or piece name and go there. The target
# resolves in the current pane's project — switching to it if it's already a
# piece, adopting it if it's an existing local/remote branch, and creating a
# new piece from it otherwise. All resolution lives in `mp switch`; this script
# only scopes the project and collects the target.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

main() {
	set -euo pipefail
	ensure_env

	local rows proj target selection
	rows="$("$(mp_bin)" go --json | build_project_rows)"
	[[ -n "$rows" ]] || die "no registered projects — run 'mp init' first"

	# Hard-scope to the pane's project (the popup runs in the pane's cwd);
	# outside any project, fall back to picking one.
	proj="$(project_for_cwd "${PWD:-}" "$rows")"
	if [[ -z "$proj" ]]; then
		selection="$(printf '%s\n' "$rows" | fzf_pick \
			--with-nth=1 \
			--prompt='project> ')" || exit 0
		[[ -n "$selection" ]] || exit 0
		proj="$(cut -f2 <<<"$selection")"
	fi

	# Positional arg is the test seam; interactively, read (paste-friendly,
	# with readline editing) from the popup.
	target="${1:-}"
	if [[ -z "$target" ]]; then
		read -r -e -p "branch or piece> " target || exit 0
	fi
	[[ -n "$target" ]] || exit 0

	# One mp invocation. --create makes a brand-new name mint a piece without a
	# TTY confirm — typing into this prompt is the confirmation.
	exec "$(mp_bin)" switch --project "$proj" --create -- "$target"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
