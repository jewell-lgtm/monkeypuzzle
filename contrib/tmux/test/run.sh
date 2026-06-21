#!/usr/bin/env bash
# Dependency-free test runner for the monkeypuzzle tmux plugin.
#
# Outside-in: the first test is the happy-path integration of the switch flow
# (canned `mp go --json` -> picker -> `mp switch` with the right selectors),
# driven non-interactively via the MP_PLUGIN_FILTER seam and a stub `mp`. The
# remaining tests are unit coverage of the jq row-builders.
#
# Runnable anywhere with bash + jq + fzf; integration tests skip cleanly if a
# dependency is missing. No bats/shellcheck required.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="$DIR/../scripts"

PASS=0
FAIL=0
SKIP=0

ok() {
	PASS=$((PASS + 1))
	printf 'ok   - %s\n' "$1"
}
fail() {
	FAIL=$((FAIL + 1))
	printf 'FAIL - %s\n     %s\n' "$1" "$2"
}
skip() {
	SKIP=$((SKIP + 1))
	printf 'skip - %s (%s)\n' "$1" "$2"
}
assert_eq() {
	if [[ "$2" == "$3" ]]; then
		ok "$1"
	else
		fail "$1" "expected [$3] got [$2]"
	fi
}

have() { command -v "$1" >/dev/null 2>&1; }

# Canned `mp go --json` output: two real projects (one with pieces, one without)
# plus a non-project entry that must be filtered out.
canned_json() {
	cat <<'JSON'
{
  "projects": [
    {
      "name": "alpha", "path": "/repos/alpha", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 2, "main_session": "mp/alpha",
      "pieces": [
        { "name": "fix-login", "worktree_path": "/wt/alpha/fix-login", "session_name": "mp/alpha/fix-login", "has_session": true },
        { "name": "dark-mode", "worktree_path": "/wt/alpha/dark-mode", "session_name": "mp/alpha/dark-mode", "has_session": false }
      ]
    },
    {
      "name": "beta", "path": "/repos/beta", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 0, "main_session": "mp/beta", "pieces": []
    },
    {
      "name": "gamma", "path": "/repos/gamma", "exists": false, "is_project": false,
      "branch": "", "piece_count": 0, "main_session": "mp/gamma", "pieces": []
    }
  ]
}
JSON
}

# ---- Unit: switch.sh build_rows --------------------------------------------
# shellcheck source=../scripts/switch.sh
if source "$SCRIPTS/switch.sh" 2>/dev/null; then
	got="$(canned_json | build_rows)"
	want="$(printf '%s\n' \
		$'alpha/(main)\talpha\t\t/repos/alpha' \
		$'alpha/fix-login\talpha\tfix-login\t/wt/alpha/fix-login' \
		$'alpha/dark-mode\talpha\tdark-mode\t/wt/alpha/dark-mode' \
		$'beta/(main)\tbeta\t\t/repos/beta')"
	assert_eq "switch build_rows: pieces + main rows, non-project skipped" "$got" "$want"
else
	fail "source switch.sh" "could not source $SCRIPTS/switch.sh"
fi

# ---- Unit: create.sh build_project_rows ------------------------------------
# shellcheck source=../scripts/create.sh
if source "$SCRIPTS/create.sh" 2>/dev/null; then
	got="$(canned_json | build_project_rows)"
	want="$(printf '%s\n' \
		$'alpha\talpha\t/repos/alpha' \
		$'beta\tbeta\t/repos/beta')"
	assert_eq "create build_project_rows: real projects only" "$got" "$want"
else
	fail "source create.sh" "could not source $SCRIPTS/create.sh"
fi

# ---- Integration: switch happy path ----------------------------------------
integration_switch() {
	if ! have jq || ! have fzf; then
		skip "switch integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/switch.log"
	mkdir -p "$bin"

	# Stub mp: emit canned JSON for `go`, record args for `switch`.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  go) cat "$tmp/canned.json" ;;
  switch) printf '%s\n' "\$*" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"
	canned_json >"$tmp/canned.json"

	# Pick the piece row by fuzzy filter; assert mp switch got the selectors.
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="fix-login" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: piece selection calls mp switch --project --piece" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --piece fix-login"

	# Pick a project main row (beta has no pieces); assert no --piece.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="beta/" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: main-row selection calls mp switch --project only" \
		"$(cat "$log" 2>/dev/null)" "switch --project beta"

	rm -rf "$tmp"
}
integration_switch

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
