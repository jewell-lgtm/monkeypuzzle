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

# Canned `mp inbox --json` output, already in mp's order: a ranked blocked
# row with a PR and note, a working row with no PR, and a snoozed row (draft
# PR, no agent) that mp has put last.
canned_inbox_json() {
	cat <<'JSON'
{
  "rows": [
    { "key": "alpha/fix-login", "project": "alpha", "piece": "fix-login", "rank": 1, "branch": "fix-login", "parent": "main",
      "worktree_path": "/wt/alpha/fix-login", "session_name": "mp/alpha/fix-login", "has_session": true,
      "agent_status": "blocked", "agent_counts": { "blocked": 1 },
      "pr": { "number": 12, "url": "https://github.com/o/alpha/pull/12", "state": "open", "draft": false },
      "merged": false, "urgency": "blocked", "note": "waiting on review", "updated_at": "2026-09-09T11:42:00Z" },
    { "key": "beta/dark-mode", "project": "beta", "piece": "dark-mode", "rank": 2, "branch": "feat/dark-mode", "parent": "main",
      "worktree_path": "/wt/beta/dark-mode", "session_name": "mp/beta/dark-mode", "has_session": true,
      "agent_status": "working", "agent_counts": { "working": 1 },
      "merged": false, "urgency": "working", "updated_at": "2026-09-09T11:40:00Z" },
    { "key": "alpha/spike", "project": "alpha", "piece": "spike", "rank": 3, "branch": "spike", "parent": "main",
      "worktree_path": "/wt/alpha/spike", "session_name": "mp/alpha/spike", "has_session": false,
      "agent_status": "", "agent_counts": null,
      "pr": { "number": 7, "url": "https://github.com/o/alpha/pull/7", "state": "open", "draft": true },
      "merged": false, "urgency": "idle", "snoozed_until": "2099-01-01T09:00:00Z", "updated_at": "2026-09-09T11:00:00Z" }
  ]
}
JSON
}

# ---- Unit: inbox.sh build_inbox_rows ---------------------------------------
# shellcheck source=../scripts/inbox.sh
if source "$SCRIPTS/inbox.sh" 2>/dev/null; then
	got="$(canned_inbox_json | build_inbox_rows)"
	want="$(printf '%s\n' \
		$'1   ⚠ blocked  alpha/fix-login  blocked  #12 open    waiting on review\talpha/fix-login\talpha\tfix-login\t/wt/alpha/fix-login\twaiting on review\thttps://github.com/o/alpha/pull/12' \
		$'2     working  beta/dark-mode   working\tbeta/dark-mode\tbeta\tdark-mode\t/wt/beta/dark-mode\t\t' \
		$'\e[2mzz    idle     alpha/spike               #7 draft\e[0m\talpha/spike\talpha\tspike\t/wt/alpha/spike\t\thttps://github.com/o/alpha/pull/7')"
	assert_eq "inbox build_inbox_rows: rank/urgency/key/agent/PR/note cells, snoozed dimmed as zz" "$got" "$want"
	assert_eq "inbox build_inbox_rows: empty inbox yields no rows" \
		"$(echo '{"rows":[]}' | build_inbox_rows)" ""
else
	fail "source inbox.sh" "could not source $SCRIPTS/inbox.sh"
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

# ---- Integration: inbox picker hands off to `mp switch` --------------------
integration_inbox() {
	if ! have jq || ! have fzf; then
		skip "inbox integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin"

	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  inbox) cat "$tmp/inbox.json" ;;
  switch) printf '%s\n' "\$*" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"
	canned_inbox_json >"$tmp/inbox.json"

	# `inbox.sh rows` is the reload target of the key bindings.
	assert_eq "inbox rows mode: prints the row list for fzf reload" \
		"$(PATH="$bin:$PATH" bash "$SCRIPTS/inbox.sh" rows | cut -f2 | tr '\n' ' ')" \
		"alpha/fix-login beta/dark-mode alpha/spike "

	PATH="$bin:$PATH" HERDR_ENV=1 MP_PLUGIN_FILTER="dark-mode" \
		bash "$SCRIPTS/inbox.sh" >/dev/null 2>&1
	assert_eq "inbox flow: selection calls mp switch --project --piece" \
		"$(cat "$log" 2>/dev/null)" "switch --project beta --piece dark-mode"

	rm -f "$log"
	echo '{"rows":[]}' >"$tmp/inbox.json"
	PATH="$bin:$PATH" HERDR_ENV=1 MP_PLUGIN_FILTER="dark-mode" \
		bash "$SCRIPTS/inbox.sh" >/dev/null 2>&1
	assert_eq "inbox flow: empty inbox switches nothing" "$(cat "$log" 2>/dev/null)" ""

	rm -rf "$tmp"
}
integration_inbox

# ---- Integration: inbox next/prev --------------------------------------------
integration_step() {
	local tmp bin mplog err rc
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	mplog="$tmp/mp.log"
	mkdir -p "$bin"

	# mp switched: quiet, exit 0, run from the given cwd (mp resolves the
	# current piece from it).
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s %s\n' "\$*" "\$PWD" >"$mplog"
[[ "\$1 \$2" == "inbox next" || "\$1 \$2" == "inbox prev" ]] || exit 2
exit 0
EOF
	chmod +x "$bin/mp"

	err="$(PATH="$bin:$PATH" bash "$SCRIPTS/step.sh" next "$tmp" 2>&1 1>/dev/null)"
	rc=$?
	assert_eq "step flow: next invokes mp inbox next from the cwd" \
		"$(cat "$mplog" 2>/dev/null)" "inbox next $tmp"
	assert_eq "step flow: silent exit 0 when mp switched" "$rc:$err" "0:"

	rm -f "$mplog"
	PATH="$bin:$PATH" bash "$SCRIPTS/step.sh" prev "$tmp" >/dev/null 2>&1
	assert_eq "step flow: prev invokes mp inbox prev" \
		"$(cut -d' ' -f1-2 "$mplog" 2>/dev/null)" "inbox prev"

	# Only piece: soft — message on stderr, exit 0.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
echo "alpha/fix-login is the only piece in the inbox; staying put" >&2
exit 0
EOF
	err="$(PATH="$bin:$PATH" bash "$SCRIPTS/step.sh" next "$tmp" 2>&1 1>/dev/null)"
	rc=$?
	assert_eq "step flow: only-piece is soft (exit 0)" "$rc" "0"
	assert_eq "step flow: relays the only-piece message" \
		"$err" "monkeypuzzle: alpha/fix-login is the only piece in the inbox; staying put"

	# A genuine failure exits non-zero so herdr's action log shows it.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
echo "inbox: no pieces" >&2
exit 1
EOF
	err="$(PATH="$bin:$PATH" bash "$SCRIPTS/step.sh" next "$tmp" 2>&1 1>/dev/null)"
	rc=$?
	assert_eq "step flow: genuine failure exits non-zero" "$rc" "1"
	assert_eq "step flow: relays a genuine failure verbatim" "$err" "monkeypuzzle: inbox: no pieces"

	# A bad direction never reaches mp.
	rm -f "$mplog"
	PATH="$bin:$PATH" bash "$SCRIPTS/step.sh" sideways "$tmp" >/dev/null 2>&1
	assert_eq "step flow: rejects an unknown direction" "$(cat "$mplog" 2>/dev/null)" ""

	rm -rf "$tmp"
}
integration_step

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
