#!/usr/bin/env bash
# Open one of this plugin's picker panes as a herdr popup. Actions are the
# key-bindable unit in herdr; each picker action routes through here so a
# single `plugin_action` binding pops the corresponding pane.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./helpers.sh
source "$DIR/helpers.sh"

main() {
	set -euo pipefail
	[[ $# -eq 1 ]] || die "usage: show.sh <pane-id>"
	exec "$(herdr_bin)" plugin pane open "monkeypuzzle.$1"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	main "$@"
fi
