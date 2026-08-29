#!/usr/bin/env bash
# Dependency-free test runner for the monkeypuzzle herdr plugin.
#
# Outside-in: integration of the open flow (canned `mp go --json` -> picker ->
# `mp switch` with the right selectors), driven non-interactively via the
# MP_PLUGIN_FILTER seam and a stub `mp`, plus unit coverage of the jq
# row-builders. Runnable anywhere with bash + jq + fzf; integration tests skip
# cleanly if a dependency is missing.
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

# Canned `mp go --json` output: two real projects (one with pieces and
# adoptable branches, one bare) plus a non-project entry that must be filtered
# out. dark-mode is an adopted branch whose name differs from the piece name.
canned_json() {
	cat <<'JSON'
{
  "projects": [
    {
      "name": "alpha", "path": "/repos/alpha", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 2, "main_session": "mp/alpha",
      "pieces": [
        { "name": "fix-login", "worktree_path": "/wt/alpha/fix-login", "session_name": "mp/alpha/fix-login", "has_session": true, "branch": "fix-login" },
        { "name": "dark-mode", "worktree_path": "/wt/alpha/dark-mode", "session_name": "mp/alpha/dark-mode", "has_session": false, "branch": "feat/dark-mode" }
      ],
      "branches": [
        { "name": "spike-idea" },
        { "name": "origin/review-fixes", "remote": true }
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

# ---- Unit: open.sh build_rows ----------------------------------------------
# shellcheck source=../scripts/open.sh
if source "$SCRIPTS/open.sh" 2>/dev/null; then
	got="$(canned_json | build_rows)"
	want="$(printf '%s\n' \
		$'alpha/(main)\talpha\t\t/repos/alpha' \
		$'alpha/fix-login\talpha\tfix-login\t/wt/alpha/fix-login' \
		$'alpha/dark-mode  [feat/dark-mode]\talpha\tdark-mode\t/wt/alpha/dark-mode' \
		$'beta/(main)\tbeta\t\t/repos/beta')"
	assert_eq "open build_rows: main + piece rows, no branch rows, non-project skipped" "$got" "$want"
else
	fail "source open.sh" "could not source $SCRIPTS/open.sh"
fi

# ---- Unit: adopt.sh build_branch_rows --------------------------------------
# shellcheck source=../scripts/adopt.sh
if source "$SCRIPTS/adopt.sh" 2>/dev/null; then
	got="$(canned_json | build_branch_rows)"
	want="$(printf '%s\n' \
		$'alpha/spike-idea\talpha\tspike-idea\t/repos/alpha' \
		$'alpha/origin/review-fixes\talpha\torigin/review-fixes\t/repos/alpha')"
	assert_eq "adopt build_branch_rows: adoptable branches only" "$got" "$want"
else
	fail "source adopt.sh" "could not source $SCRIPTS/adopt.sh"
fi

# ---- Unit: helpers.sh build_project_rows + project_for_cwd ------------------
# shellcheck source=../scripts/helpers.sh
if source "$SCRIPTS/helpers.sh" 2>/dev/null; then
	rows="$(canned_json | build_project_rows)"
	want="$(printf '%s\n' \
		$'alpha\talpha\t/repos/alpha' \
		$'beta\tbeta\t/repos/beta')"
	assert_eq "helpers build_project_rows: real projects only" "$rows" "$want"

	assert_eq "project_for_cwd: repo root matches" \
		"$(project_for_cwd "/repos/alpha" "$rows")" "alpha"
	assert_eq "project_for_cwd: sibling prefix does not match" \
		"$(project_for_cwd "/repos/alphabet" "$rows")" ""
else
	fail "source helpers.sh" "could not source $SCRIPTS/helpers.sh"
fi

# ---- Integration: open happy path ------------------------------------------
integration_open() {
	if ! have jq || ! have fzf; then
		skip "open integration" "needs jq + fzf"
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
	PATH="$bin:$PATH" HERDR_ENV=1 MP_PLUGIN_FILTER="fix-login" \
		bash "$SCRIPTS/open.sh" >/dev/null 2>&1
	assert_eq "open flow: piece selection calls mp switch --project --piece" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --piece fix-login"

	# Pick a project main row (beta has no pieces); assert no --piece.
	rm -f "$log"
	PATH="$bin:$PATH" HERDR_ENV=1 MP_PLUGIN_FILTER="beta/" \
		bash "$SCRIPTS/open.sh" >/dev/null 2>&1
	assert_eq "open flow: main-row selection calls mp switch --project only" \
		"$(cat "$log" 2>/dev/null)" "switch --project beta"

	rm -rf "$tmp"
}
integration_open

# ---- Integration: adopt hands off through --branch --------------------------
integration_adopt() {
	if ! have jq || ! have fzf; then
		skip "adopt integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/switch.log"
	mkdir -p "$bin"

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

	PATH="$bin:$PATH" HERDR_ENV=1 MP_PLUGIN_FILTER="spike" \
		bash "$SCRIPTS/adopt.sh" >/dev/null 2>&1
	assert_eq "adopt flow: branch selection calls mp switch --branch" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --branch spike-idea"

	rm -rf "$tmp"
}
integration_adopt

# ---- Integration: blocked jump ---------------------------------------------
integration_blocked() {
	local tmp bin mplog
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	mplog="$tmp/mp.log"
	mkdir -p "$bin"

	# mp reports nothing blocked: soft case — message on stderr, exit 0.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$mplog"
[[ "\$1 \$2" == "agent focus" ]] || exit 2
echo "monkeypuzzle: ⚠ no blocked agents" >&2
exit 0
EOF
	chmod +x "$bin/mp"

	err="$(PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" 2>&1 1>/dev/null)"
	rc=$?
	assert_eq "blocked flow: invokes mp agent focus --blocked --all" \
		"$(cat "$mplog" 2>/dev/null)" "agent focus --blocked --all"
	assert_eq "blocked flow: nothing-blocked is soft (exit 0)" "$rc" "0"
	assert_eq "blocked flow: relays the no-blocked-agents message" \
		"$err" "monkeypuzzle: no blocked agents"

	# A genuine failure must surface with a non-zero exit so herdr's action
	# log shows it.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$mplog"
echo "registry unreadable" >&2
exit 1
EOF
	rm -f "$mplog"
	err="$(PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" 2>&1 1>/dev/null)"
	rc=$?
	assert_eq "blocked flow: genuine failure exits non-zero" "$rc" "1"
	assert_eq "blocked flow: relays a genuine failure verbatim" \
		"$err" "monkeypuzzle: registry unreadable"

	rm -rf "$tmp"
}
integration_blocked

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
