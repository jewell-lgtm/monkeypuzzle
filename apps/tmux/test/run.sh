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
# fix-login carries draft PR metadata and dark-mode a ready PR, exercising the
# switch picker's status badges.
canned_json() {
	cat <<'JSON'
{
  "projects": [
    {
      "name": "alpha", "path": "/repos/alpha", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 2, "main_session": "mp/alpha",
      "pieces": [
        { "name": "fix-login", "worktree_path": "/wt/alpha/fix-login", "session_name": "mp/alpha/fix-login", "has_session": true, "branch": "fix-login", "pr_number": 483, "pr_draft": true },
        { "name": "dark-mode", "worktree_path": "/wt/alpha/dark-mode", "session_name": "mp/alpha/dark-mode", "has_session": false, "branch": "feat/dark-mode", "pr_number": 484 }
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
		$'alpha/(main)                       trunk\talpha\t\t/repos/alpha\t\t/repos/alpha' \
		$'alpha/fix-login                    ◆ draft\talpha\tfix-login\t/wt/alpha/fix-login\t\t/repos/alpha' \
		$'alpha/dark-mode  [feat/dark-mode]  ◆ in review\talpha\tdark-mode\t/wt/alpha/dark-mode\t\t/repos/alpha' \
		$'alpha/spike-idea                   branch\talpha\t\t/repos/alpha\tspike-idea\t/repos/alpha' \
		$'alpha/origin/review-fixes          branch\talpha\t\t/repos/alpha\torigin/review-fixes\t/repos/alpha' \
		$'beta/(main)                        trunk\tbeta\t\t/repos/beta\t\t/repos/beta')"
	assert_eq "switch build_rows: badge-aligned rows carrying the project path, non-project skipped" "$got" "$want"
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

	assert_eq "split_project_query: a known project prefix scopes the target" \
		"$(split_project_query "alpha/new-thing" "$rows")" $'alpha\tnew-thing'
	assert_eq "split_project_query: an unknown prefix stays in the target" \
		"$(split_project_query "feat/new-thing" "$rows")" $'\tfeat/new-thing'
	assert_eq "split_project_query: a bare name names no project" \
		"$(split_project_query "new-thing" "$rows")" $'\tnew-thing'

	target_verdict() { if valid_piece_target "$1"; then echo name; else echo filter; fi; }
	assert_eq "valid_piece_target: a plain name is a name" "$(target_verdict "new-thing")" "name"
	assert_eq "valid_piece_target: a branch path is a name" "$(target_verdict "feat/login-v2.1")" "name"
	assert_eq "valid_piece_target: an accented name is a name" "$(target_verdict "änderung-login")" "name"
	assert_eq "valid_piece_target: an fzf anchor is a filter" "$(target_verdict "^new-thing")" "filter"
	assert_eq "valid_piece_target: two terms are a filter" "$(target_verdict "alpha login")" "filter"
	assert_eq "valid_piece_target: an exact-match quote is a filter" "$(target_verdict "'\''fix")" "filter"
	assert_eq "valid_piece_target: a trailing slash is not a name" "$(target_verdict "alpha/")" "filter"
	# Names git itself refuses, which the guard exists to keep away from it.
	assert_eq "valid_piece_target: a trailing dot is not a name" "$(target_verdict "hotfix.")" "filter"
	assert_eq "valid_piece_target: a .lock suffix is not a name" "$(target_verdict "foo.lock")" "filter"
	assert_eq "valid_piece_target: a dot-led component is not a name" "$(target_verdict "feat/.hidden")" "filter"
	assert_eq "valid_piece_target: a reflog span is not a name" "$(target_verdict "ref@{0}")" "filter"

	assert_eq "project_path: a named project resolves to its path" \
		"$(project_path "beta" "$rows")" "/repos/beta"
	assert_eq "project_path: an unknown project resolves to nothing" \
		"$(project_path "gamma" "$rows")" ""
	assert_eq "project_path: no name resolves to nothing" "$(project_path "" "$rows")" ""
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

# ---- Integration: create a piece from the switch picker ----------------------------------------------
integration_switch_create() {
	if ! have jq || ! have fzf; then
		skip "switch create integration" "needs jq + fzf"
		return
	fi
	local tmp bin log err rc
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin" "$tmp/repos/alpha/src" "$tmp/repos/beta"

	# A real alpha path: project_for_cwd scopes from the cwd, and the create
	# flow cds into the project it picks.
	sed "s|/repos/|$tmp/repos/|g" >"$tmp/canned.json" < <(canned_json)
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  go) cat "$tmp/canned.json" ;;
  switch) printf '%s\n' "\$*" > "$log" ;;
  create) printf '%s :: %s\n' "\$*" "\$PWD" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"

	# A typed name that matches no row is created in the project it names.
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="alpha/new-thing" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: an unmatched project/name creates that piece" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --create -- new-thing"

	# Inside a project, a bare name needs no prefix.
	rm -f "$log"
	(cd "$tmp/repos/alpha/src" && PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="new-thing" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1)
	assert_eq "switch flow: an unmatched bare name creates it in the cwd's project" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --create -- new-thing"

	# The create key mints the query even when a row matches it.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="alpha/fix-login" MP_PLUGIN_KEY="ctrl-o" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: the create key wins over a matching row" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --create -- fix-login"

	# The create key with nothing typed falls through to the full create flow,
	# which names the piece at its own prompt.
	rm -f "$log"
	(cd "$tmp" && PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="" MP_PLUGIN_KEY="ctrl-o" \
		bash "$SCRIPTS/switch.sh" <<<"from-the-prompt" >/dev/null 2>&1)
	assert_eq "switch flow: the create key with no query hands off to create.sh" \
		"$(cat "$log" 2>/dev/null)" "create --name from-the-prompt :: $tmp/repos/alpha"

	# A query that reads as a search is refused, loudly, with nothing created.
	rm -f "$log"
	err="$(PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="^new-thing" \
		bash "$SCRIPTS/switch.sh" 2>&1 >/dev/null)"
	rc=$?
	assert_eq "switch flow: a search-syntax query creates nothing" "$(cat "$log" 2>/dev/null)" ""
	assert_eq "switch flow: a search-syntax query fails visibly" "$rc" "1"
	case "$err" in
		*"is a filter, not a name"*) ok "switch flow: says why it created nothing" ;;
		*) fail "switch flow: says why it created nothing" "$err" ;;
	esac

	# A project prefix with no name after it asks for a name instead of
	# creating a branch called "alpha/".
	rm -f "$log"
	(cd "$tmp/repos/beta" && PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="alpha/" MP_PLUGIN_KEY="ctrl-o" \
		bash "$SCRIPTS/switch.sh" <<<"from-the-prompt" >/dev/null 2>&1)
	assert_eq "switch flow: a trailing slash asks for a name" \
		"$(cat "$log" 2>/dev/null)" "create --name from-the-prompt :: $tmp/repos/alpha"

	# That handoff names the project, and create.sh must honour it over both
	# the cwd and its own picker — standing in alpha, told beta, it is beta.
	rm -f "$log"
	(cd "$tmp/repos/alpha" && PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="alpha" \
		bash "$SCRIPTS/create.sh" beta <<<"from-the-prompt" >/dev/null 2>&1)
	assert_eq "create flow: a named project wins over the cwd and the picker" \
		"$(cat "$log" 2>/dev/null)" "create --name from-the-prompt :: $tmp/repos/beta"

	rm -rf "$tmp"
}
integration_switch_create

# ---- Integration: done / abandon from the picker ---------------------------
# finish.sh is the whole confirmation: the prompt, the mp call in the right
# repo, and the one escalation offered after mp's own gate refuses.
integration_finish() {
	local tmp bin log answers rc
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin" "$tmp/repos/alpha"

	# Stub mp: append each done/abandon call with the directory it ran in. It
	# fails until $tmp/allow exists, standing in for the merged/dirty gate.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  done|abandon)
    printf '%s :: %s\n' "\$*" "\$PWD" >> "$log"
    if [ -e "$tmp/allow" ] || [ "\$4" = "--force" ]; then exit 0; fi
    echo "piece is not merged into main" >&2
    exit 1 ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"

	run_finish_sh() { # <answers> <args...>
		answers="$1"
		shift
		printf '%s\n' "$answers" | PATH="$bin:$PATH" bash "$SCRIPTS/finish.sh" "$@" >/dev/null 2>&1
	}

	# Declining the prompt runs nothing at all.
	run_finish_sh "n" abandon alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: a declined prompt runs no mp command" "$(cat "$log" 2>/dev/null)" ""

	# Confirming runs the verb against the piece, in that project's repo.
	: >"$tmp/allow"
	run_finish_sh "y" abandon alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: y abandons the piece in its own project" \
		"$(cat "$log" 2>/dev/null)" "abandon --piece fix-login :: $tmp/repos/alpha"

	rm -f "$log"
	run_finish_sh "y" done alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: y finishes the piece with mp done" \
		"$(cat "$log" 2>/dev/null)" "done --piece fix-login :: $tmp/repos/alpha"

	# f forces in one keystroke, without waiting to be refused first.
	rm -f "$log"
	run_finish_sh "f" abandon alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: f forces without a second prompt" \
		"$(cat "$log" 2>/dev/null)" "abandon --piece fix-login --force :: $tmp/repos/alpha"

	# Refused: mp's gate says no, and the follow-up prompt escalates once.
	rm -f "$log" "$tmp/allow"
	run_finish_sh "$(printf 'y\ny')" done alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: a refused run offers force, and forcing retries it" \
		"$(cat "$log" 2>/dev/null)" \
		"$(printf 'done --piece fix-login :: %s\ndone --piece fix-login --force :: %s' "$tmp/repos/alpha" "$tmp/repos/alpha")"

	# Declining that escalation leaves the piece alone.
	rm -f "$log"
	run_finish_sh "$(printf 'y\nn')" done alpha fix-login "$tmp/repos/alpha"
	assert_eq "finish: declining the escalation stops at the refusal" \
		"$(cat "$log" 2>/dev/null)" "done --piece fix-login :: $tmp/repos/alpha"

	# A row with no piece (a project main, or a branch) has no lifecycle.
	rm -f "$log"
	out="$(printf 'y\n' | PATH="$bin:$PATH" bash "$SCRIPTS/finish.sh" abandon alpha "" "$tmp/repos/alpha" 2>&1)"
	assert_eq "finish: a non-piece row runs nothing" "$(cat "$log" 2>/dev/null)" ""
	case "$out" in
		*"not a piece"*) ok "finish: a non-piece row says why" ;;
		*) fail "finish: a non-piece row says why" "$out" ;;
	esac

	# An unknown action is a wiring bug, not something to guess at.
	printf 'y\n' | PATH="$bin:$PATH" bash "$SCRIPTS/finish.sh" merge alpha fix-login "$tmp/repos/alpha" >/dev/null 2>&1
	rc=$?
	assert_eq "finish: an unknown action fails loudly" "$rc" "1"
	assert_eq "finish: an unknown action runs nothing" "$(cat "$log" 2>/dev/null)" ""

	rm -rf "$tmp"
}
integration_finish

# ---- Integration: the picker's lifecycle keys reach finish.sh --------------
integration_switch_finish_keys() {
	if ! have jq || ! have fzf; then
		skip "switch lifecycle keys" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin"

	sed "s|/repos/|$tmp/repos/|g" >"$tmp/canned.json" < <(canned_json)
	mkdir -p "$tmp/repos/alpha"
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  go) cat "$tmp/canned.json" ;;
  done|abandon) printf '%s :: %s\n' "\$*" "\$PWD" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"

	# The selected row carries the piece and the project path the verb needs.
	printf 'y\n' | PATH="$bin:$PATH" TMUX=fake MP_PLUGIN_FILTER="fix-login" MP_PLUGIN_KEY="ctrl-x" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: the abandon key abandons the selected piece" \
		"$(cat "$log" 2>/dev/null)" "abandon --piece fix-login :: $tmp/repos/alpha"

	rm -f "$log"
	printf 'y\n' | PATH="$bin:$PATH" TMUX=fake MP_PLUGIN_FILTER="fix-login" MP_PLUGIN_KEY="ctrl-d" \
		bash "$SCRIPTS/switch.sh" >/dev/null 2>&1
	assert_eq "switch flow: the done key finishes the selected piece" \
		"$(cat "$log" 2>/dev/null)" "done --piece fix-login :: $tmp/repos/alpha"

	# `switch.sh rows` is the reload target the keys hang off; it must print
	# the same rows the picker was built from.
	assert_eq "switch flow: the rows subcommand feeds the reload" \
		"$(PATH="$bin:$PATH" bash "$SCRIPTS/switch.sh" rows)" \
		"$(build_rows <"$tmp/canned.json")"

	rm -rf "$tmp"
}
integration_switch_finish_keys

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
	local tmp bin mplog
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	mplog="$tmp/mp.log"
	mkdir -p "$bin"

	# mp reports nothing blocked: blocked.sh must relay via tmux display-message.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$mplog"
[[ "\$1 \$2" == "agent focus" ]] || exit 2
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
	assert_eq "blocked flow: invokes mp agent focus --blocked --all" \
		"$(cat "$mplog" 2>/dev/null)" "agent focus --blocked --all"

	# mp finds and focuses one: blocked.sh must NOT show any message. The stub
	# still records its own invocation so a bug dropping --blocked/--all from
	# the exec call can't pass this vacuously.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$mplog"
[[ "\$1 \$2" == "agent focus" ]] || exit 2
exit 0
EOF
	rm -f "$tmp/tmux.log" "$mplog"
	PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" >/dev/null 2>&1
	assert_eq "blocked flow: still invokes mp agent focus --blocked --all" \
		"$(cat "$mplog" 2>/dev/null)" "agent focus --blocked --all"
	assert_eq "blocked flow: silent when an agent was focused" \
		"$(cat "$tmp/tmux.log" 2>/dev/null)" ""

	# A genuine failure (anything other than the known "no blocked agents"
	# case) must still surface — not vanish silently — since a run-shell
	# binding has no other way to signal something went wrong.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$mplog"
echo "registry unreadable" >&2
exit 1
EOF
	rm -f "$tmp/tmux.log" "$mplog"
	PATH="$bin:$PATH" bash "$SCRIPTS/blocked.sh" "$tmp" >/dev/null 2>&1
	assert_eq "blocked flow: relays a genuine failure verbatim" \
		"$(cat "$tmp/tmux.log" 2>/dev/null)" "display-message monkeypuzzle: registry unreadable"

	rm -rf "$tmp"
}
integration_blocked

# ---- Startup environment and failure propagation ----------------------------
integration_startup() {
	local tmp err rc
	tmp="$(mktemp -d)"
	mkdir -p "$tmp/.local/bin" "$tmp/first"
	printf '#!/bin/sh\nexit 0\n' >"$tmp/.local/bin/mp"
	chmod +x "$tmp/.local/bin/mp"
	assert_eq "PATH: finds a user install with a system-only inherited PATH" \
		"$(HOME="$tmp" PATH=/usr/bin:/bin bash -c 'source "$1"; setup_path; command -v mp' _ "$SCRIPTS/helpers.sh")" \
		"$tmp/.local/bin/mp"
	cp "$tmp/.local/bin/mp" "$tmp/first/mp"
	assert_eq "PATH: preserves an existing command override" \
		"$(HOME="$tmp" PATH="$tmp/first:/usr/bin:/bin" bash -c 'source "$1"; setup_path; command -v mp' _ "$SCRIPTS/helpers.sh")" \
		"$tmp/first/mp"
	err="$(TMUX=test MP_PLUGIN_BIN="$tmp/missing-mp" bash "$SCRIPTS/pane.sh" switch.sh </dev/null 2>&1)"
	rc=$?
	assert_eq "pane: missing dependency retains failure status" "$rc" 1
	case "$err" in
		*"Required command not found: $tmp/missing-mp"*"@monkeypuzzle-bin"*"Searched PATH:"*"picker failed"*) ok "pane: actionable dependency error" ;;
		*) fail "pane: actionable dependency error" "$err" ;;
	esac
	for rc in 1 130 2; do
		bash -c 'source "$1"; picker_exit "$2"' _ "$SCRIPTS/helpers.sh" "$rc"
		got=$?
		if [[ "$rc" == 2 ]]; then
			assert_eq "picker: fzf errors remain failures" "$got" 2
		else
			assert_eq "picker: fzf cancellation/no-match ($rc) closes normally" "$got" 0
		fi
	done
	rm -rf "$tmp"
}
integration_startup

if have python3; then
	if python3 "$SCRIPTS/../test/pane_test.py"; then
		ok "all picker panes: terminal errors wait for dismissal and preserve exit code"
	else
		fail "pane: terminal error waits for dismissal" "see Python assertion above"
	fi
else
	skip "pane terminal test" "needs python3"
fi

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
