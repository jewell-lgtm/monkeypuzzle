#!/usr/bin/env bash
# End a piece's lifecycle from a picker: `mp done` (finish a merged piece) or
# `mp abandon` (bin an unmerged one). Bound to a key in the open picker, which
# reloads its rows afterwards.
#
#   finish.sh done|abandon <project> <piece> <project-path>
#
# The confirmation lives here rather than in mp: `mp done` / `mp abandon` are
# unprompted by design, and a keystroke over a fuzzy list is not the same
# consent as typing the piece's name. mp still owns every rule — whether the
# piece is merged, whether the worktree is dirty, what --force overrides — so
# when it refuses we show its own words and offer the one escalation past it.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

# ask prints a prompt and reads one line of the answer. fzf's execute() hands
# the command the real terminal, so plain stdin is the prompt — and piping
# answers in drives the flow without one.
ask() {
	local reply=""
	read -r -p "$1" reply || true
	printf '%s' "$reply"
}

# pause holds the pane open so mp's output — a success line, or the reason it
# refused — is read before the picker paints over it. Off a terminal there is
# no pane to hold and nothing is waiting to read it.
pause() {
	[[ -t 0 ]] || return 0
	read -r -p "$(printf '\n[enter] ')" _ || true
}

# run_finish invokes the mp verb for $action in the project directory, so the
# piece name resolves against the right repo whichever project the picker was
# opened from. mp reads an input document from stdin when there is one, so it
# must not inherit the stream the prompts are read from: the next unread
# answer would reach it as a malformed document. The flags are the whole input.
run_finish() {
	local action="$1" piece="$2" project_path="$3" force="$4"
	if [[ "$force" == "force" ]]; then
		(cd "$project_path" && "$(mp_bin)" "$action" --piece "$piece" --force </dev/null)
	else
		(cd "$project_path" && "$(mp_bin)" "$action" --piece "$piece" </dev/null)
	fi
}

main() {
	set -uo pipefail
	setup_path
	local action="${1:-}" project="${2:-}" piece="${3:-}" project_path="${4:-}"
	local verb answer

	case "$action" in
		done) verb="finish" ;;
		abandon) verb="abandon" ;;
		*) die "unknown action: ${action:-<none>}" ;;
	esac

	# Main and branch rows have no piece to end. Say so rather than acting on
	# whatever the empty selector resolves to.
	if [[ -z "$piece" ]]; then
		printf 'Nothing to %s: that row is not a piece.\n' "$action"
		pause
		return 0
	fi
	[[ -n "$project_path" ]] || die "no project path for $project/$piece"

	answer="$(ask "$verb $project/$piece? [y/N/f=force] ")"
	case "$answer" in
		y | Y | yes) ;;
		f | F | force)
			run_finish "$action" "$piece" "$project_path" force
			pause
			return 0
			;;
		*) return 0 ;;
	esac

	if run_finish "$action" "$piece" "$project_path" ""; then
		pause
		return 0
	fi

	# mp's gate refused and printed why. Offer the single escalation past it
	# instead of making the user leave the picker to retry by hand.
	answer="$(ask "$(printf '\nforce? [y/N] ')")"
	case "$answer" in
		y | Y | yes | f | F | force) run_finish "$action" "$piece" "$project_path" force ;;
	esac
	pause
}

# Run main only when executed directly, so the test runner can source this
# file and exercise its helpers in isolation.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
