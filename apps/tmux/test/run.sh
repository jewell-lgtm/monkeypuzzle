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

# ---- Unit: switch.sh build_rows --------------------------------------------
# shellcheck source=../scripts/switch.sh
if source "$SCRIPTS/switch.sh" 2>/dev/null; then
	got="$(canned_json | build_rows)"
	want="$(printf '%s\n' \
		$'alpha/(main)\talpha\t\t/repos/alpha\t' \
		$'alpha/fix-login\talpha\tfix-login\t/wt/alpha/fix-login\t' \
		$'alpha/dark-mode  [feat/dark-mode]\talpha\tdark-mode\t/wt/alpha/dark-mode\t' \
		$'alpha/spike-idea  (branch)\talpha\t\t/repos/alpha\tspike-idea' \
		$'alpha/origin/review-fixes  (branch)\talpha\t\t/repos/alpha\torigin/review-fixes' \
		$'beta/(main)\tbeta\t\t/repos/beta\t')"
	assert_eq "switch build_rows: main + piece + branch rows, non-project skipped" "$got" "$want"
else
	fail "source switch.sh" "could not source $SCRIPTS/switch.sh"
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
	assert_eq "project_for_cwd: nested path matches" \
		"$(project_for_cwd "/repos/beta/src/deep" "$rows")" "beta"
	assert_eq "project_for_cwd: sibling prefix does not match" \
		"$(project_for_cwd "/repos/alphabet" "$rows")" ""
	assert_eq "project_for_cwd: outside any project is empty" \
		"$(project_for_cwd "/home/nobody" "$rows")" ""
else
	fail "source helpers.sh" "could not source $SCRIPTS/helpers.sh"
fi

# Canned `mp agent list --all --json` output: blocked first, cross-project,
# one agent without a pane. icon comes straight from mp's JSON (the fixture
# mirrors what internal/core/agent's statusIcons table produces).
canned_agents_json() {
	cat <<'JSON'
{
  "agents": [
    { "project": "alpha", "piece": "fix-login", "session_name": "mp/alpha/fix-login", "id": "sess-1", "kind": "claude", "status": "blocked", "icon": "🔴", "pane": "%7", "updated_at": "2026-07-30T10:00:00Z" },
    { "project": "beta", "piece": "dark-mode", "session_name": "mp/beta/dark-mode", "id": "codex-1", "kind": "codex", "status": "working", "icon": "⚡", "updated_at": "2026-07-30T10:00:00Z" }
  ]
}
JSON
}

# ---- Unit: agents.sh build_agent_rows --------------------------------------
# shellcheck source=../scripts/agents.sh
if source "$SCRIPTS/agents.sh" 2>/dev/null; then
	got="$(canned_agents_json | build_agent_rows)"
	want="$(printf '%s\n' \
		$'🔴 alpha/fix-login · claude sess-1\tsess-1\t%7' \
		$'⚡ beta/dark-mode · codex codex-1\tcodex-1\t')"
	assert_eq "agents build_agent_rows: icon from JSON, id+pane only" "$got" "$want"
else
	fail "source agents.sh" "could not source $SCRIPTS/agents.sh"
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

	# Pick an adoptable-branch row; assert dispatch through --branch.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="spike" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: branch-row selection calls mp switch --branch" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --branch spike-idea"

	rm -rf "$tmp"
}
integration_switch

# ---- Integration: branch jump (paste a target) ------------------------------
integration_branch() {
	if ! have jq || ! have fzf; then
		skip "branch integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/switch.log"
	mkdir -p "$bin" "$tmp/repos/alpha/src"

	# Stub mp with canned JSON whose alpha path is a real directory, so
	# project_for_cwd can hard-scope from the cwd.
	sed "s|/repos/alpha|$tmp/repos/alpha|g" >"$tmp/canned.json" < <(canned_json)
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  go) cat "$tmp/canned.json" ;;
  switch) printf '%s\n' "\$*" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"

	# Inside alpha (nested dir): the target goes straight to mp switch --create,
	# scoped to alpha with no project picker.
	(cd "$tmp/repos/alpha/src" && PATH="$bin:$PATH" TMUX="fake,1,0" \
		bash "$SCRIPTS/branch.sh" "feat/my-spike" >/dev/null 2>&1)
	assert_eq "branch flow: in-repo target hard-scopes to the pane's project" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --create -- feat/my-spike"

	# Outside any project: falls back to the project picker (via the filter
	# seam), then passes the target through.
	rm -f "$log"
	(cd "$tmp" && PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="beta" \
		bash "$SCRIPTS/branch.sh" "my-spike" >/dev/null 2>&1)
	assert_eq "branch flow: outside a project falls back to the picker" \
		"$(cat "$log" 2>/dev/null)" "switch --project beta --create -- my-spike"

	rm -rf "$tmp"
}
integration_branch

# ---- Integration: agents picker hands off to `mp agent focus` -------------
integration_agents() {
	if ! have jq || ! have fzf; then
		skip "agents integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin"

	# Stub mp: canned agent list; record the `focus` call's args.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1 \$2" in
  "agent list") cat "$tmp/agents.json" ;;
  "agent focus") printf '%s\n' "\$*" > "$log" ;;
esac
EOF
	chmod +x "$bin/mp"
	canned_agents_json >"$tmp/agents.json"

	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="fix-login" \
		bash "$SCRIPTS/agents.sh" >/dev/null 2>&1
	assert_eq "agents flow: selection hands off to mp agent focus <id> --all" \
		"$(cat "$log" 2>/dev/null)" "agent focus sess-1 --all"

	rm -rf "$tmp"
}
integration_agents

# ---- Integration: blocked jump relays "nothing blocked" -------------------
integration_blocked() {
	local tmp bin
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	mkdir -p "$bin"

	# mp reports nothing blocked: blocked.sh must relay via tmux display-message.
	cat >"$bin/mp" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2" == "agent focus" ]] || exit 2
echo "monkeypuzzle: ⚠ no blocked agents" >&2
exit 0
EOF
	cat >"$bin/tmux" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$tmp/tmux.log"
EOF
	chmod +x "$bin/mp" "$bin/tmux"

	PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" >/dev/null 2>&1
	assert_eq "blocked flow: relays the no-blocked-agents message" \
		"$(cat "$tmp/tmux.log" 2>/dev/null)" "display-message monkeypuzzle: no blocked agents"

	# mp finds and focuses one: blocked.sh must NOT show any message.
	cat >"$bin/mp" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2" == "agent focus" ]] || exit 2
exit 0
EOF
	rm -f "$tmp/tmux.log"
	PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" >/dev/null 2>&1
	assert_eq "blocked flow: silent when an agent was focused" \
		"$(cat "$tmp/tmux.log" 2>/dev/null)" ""

	rm -rf "$tmp"
}
integration_blocked

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
