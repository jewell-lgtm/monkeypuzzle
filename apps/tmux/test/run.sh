#!/usr/bin/env bash
# Dependency-free test runner for the monkeypuzzle tmux plugin.
#
# Outside-in: the integration tests drive the go.sh one-stop picker end to end
# (canned `mp go --json` -> picker -> `mp switch` / `mp create` with the right
# selectors), non-interactively via the MP_PLUGIN_FILTER / MP_PLUGIN_KEY /
# MP_PLUGIN_INPUT_* seams and a stub `mp`. The remaining tests are unit
# coverage of the jq row-builders.
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

# Canned `mp go --json` output exercising every row source: a current project
# with pieces + branches + an agent badge (alpha), a piece-less project (beta),
# a non-project entry that must be filtered (gamma), two more projects with
# pieces whose recency ordering must flip registry order (epsilon older than
# delta), and a remote project that gets a main row only (rho).
canned_json() {
	cat <<'JSON'
{
  "current": { "project": "alpha", "piece": "fix-login" },
  "projects": [
    {
      "name": "alpha", "path": "/repos/alpha", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 2, "main_session": "mp/alpha",
      "pieces": [
        { "name": "fix-login", "worktree_path": "/wt/alpha/fix-login", "session_name": "mp/alpha/fix-login", "has_session": true, "mod_time": "2026-07-01T10:00:00Z", "agent_status": "blocked" },
        { "name": "dark-mode", "worktree_path": "/wt/alpha/dark-mode", "session_name": "mp/alpha/dark-mode", "has_session": false, "mod_time": "2026-06-01T10:00:00Z" }
      ],
      "branches": [
        { "name": "feature/x" },
        { "name": "origin/hotfix", "remote": true }
      ]
    },
    {
      "name": "beta", "path": "/repos/beta", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 0, "main_session": "mp/beta", "pieces": []
    },
    {
      "name": "epsilon", "path": "/repos/epsilon", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 1, "main_session": "mp/epsilon",
      "pieces": [
        { "name": "old-work", "worktree_path": "/wt/epsilon/old-work", "session_name": "mp/epsilon/old-work", "has_session": false, "mod_time": "2026-05-01T10:00:00Z" }
      ]
    },
    {
      "name": "gamma", "path": "/repos/gamma", "exists": false, "is_project": false,
      "branch": "", "piece_count": 0, "main_session": "mp/gamma", "pieces": []
    },
    {
      "name": "delta", "path": "/repos/delta", "exists": true, "is_project": true,
      "branch": "main", "piece_count": 1, "main_session": "mp/delta",
      "pieces": [
        { "name": "ship-it", "worktree_path": "/wt/delta/ship-it", "session_name": "mp/delta/ship-it", "has_session": true, "mod_time": "2026-07-15T10:00:00Z" }
      ]
    },
    {
      "name": "rho", "path": "/repos/rho", "host": "devbox", "exists": true, "is_project": true,
      "branch": "", "piece_count": 0, "main_session": "mp/rho", "pieces": []
    }
  ]
}
JSON
}

# canned_json_at rewrites the fixture's paths under a real base dir (and
# creates the project dirs), so flows that cd into a project path work.
canned_json_at() {
	local base="$1"
	mkdir -p "$base/repos/alpha" "$base/repos/beta" "$base/repos/epsilon" "$base/repos/delta"
	canned_json | sed "s|/repos/|$base/repos/|g; s|/wt/|$base/wt/|g"
}

# ---- Unit: go.sh build_rows -------------------------------------------------
# shellcheck source=../scripts/go.sh
if source "$SCRIPTS/go.sh" 2>/dev/null; then
	got="$(canned_json | build_rows)"
	want="$(printf '%s\n' \
		$'alpha/(main)\tmain\talpha\t\t/repos/alpha' \
		$'alpha/fix-login 🔴 *\tpiece\talpha\tfix-login\t/wt/alpha/fix-login' \
		$'alpha/dark-mode\tpiece\talpha\tdark-mode\t/wt/alpha/dark-mode' \
		$'alpha/(+ new piece)\tcreate\talpha\t\t/repos/alpha' \
		$'alpha/(branch: feature/x)\tbranch\talpha\tfeature/x\t/repos/alpha' \
		$'alpha/(branch: origin/hotfix)\tbranch\talpha\torigin/hotfix\t/repos/alpha' \
		$'delta/(main)\tmain\tdelta\t\t/repos/delta' \
		$'delta/ship-it\tpiece\tdelta\tship-it\t/wt/delta/ship-it' \
		$'delta/(+ new piece)\tcreate\tdelta\t\t/repos/delta' \
		$'epsilon/(main)\tmain\tepsilon\t\t/repos/epsilon' \
		$'epsilon/old-work\tpiece\tepsilon\told-work\t/wt/epsilon/old-work' \
		$'epsilon/(+ new piece)\tcreate\tepsilon\t\t/repos/epsilon' \
		$'beta/(main)\tmain\tbeta\t\t/repos/beta' \
		$'beta/(+ new piece)\tcreate\tbeta\t\t/repos/beta' \
		$'rho/(main)\tmain\trho\t\t/repos/rho')"
	assert_eq "go build_rows: full taxonomy, current first, recency order, remote main-only" "$got" "$want"

	# Without a current context: no markers, recency ordering still applies.
	got="$(canned_json | jq 'del(.current)' | build_rows)"
	if [[ "$got" == *' *'* ]]; then
		fail "go build_rows: no current -> no markers" "found a * marker in [$got]"
	else
		ok "go build_rows: no current -> no markers"
	fi
	assert_eq "go build_rows: no current -> most recent project first" \
		"$(head -n1 <<<"$got")" $'delta/(main)\tmain\tdelta\t\t/repos/delta'

	assert_eq "go current_label: project/piece" "$(canned_json | current_label)" "alpha/fix-login"
	assert_eq "go current_label: project only" \
		"$(canned_json | jq 'del(.current.piece)' | current_label)" "alpha"
	assert_eq "go current_label: empty without current" \
		"$(canned_json | jq 'del(.current)' | current_label)" ""
else
	fail "source go.sh" "could not source $SCRIPTS/go.sh"
fi

# Canned `mp agent list --all --json` output: blocked first, cross-project,
# one agent without a pane.
canned_agents_json() {
	cat <<'JSON'
{
  "agents": [
    { "project": "alpha", "piece": "fix-login", "session_name": "mp/alpha/fix-login", "id": "sess-1", "kind": "claude", "status": "blocked", "pane": "%7", "updated_at": "2026-07-30T10:00:00Z" },
    { "project": "beta", "piece": "dark-mode", "session_name": "mp/beta/dark-mode", "id": "codex-1", "kind": "codex", "status": "working", "updated_at": "2026-07-30T10:00:00Z" }
  ]
}
JSON
}

# ---- Unit: agents.sh build_agent_rows --------------------------------------
# shellcheck source=../scripts/agents.sh
if source "$SCRIPTS/agents.sh" 2>/dev/null; then
	got="$(canned_agents_json | build_agent_rows)"
	want="$(printf '%s\n' \
		$'🔴 alpha/fix-login · claude sess-1\tmp/alpha/fix-login\t%7\tsess-1\tfix-login\talpha' \
		$'⚡ beta/dark-mode · codex codex-1\tmp/beta/dark-mode\t\tcodex-1\tdark-mode\tbeta')"
	assert_eq "agents build_agent_rows: project labels, pane passthrough" "$got" "$want"
else
	fail "source agents.sh" "could not source $SCRIPTS/agents.sh"
fi

# ---- Unit: blocked.sh pick_blocked -----------------------------------------
# shellcheck source=../scripts/blocked.sh
if source "$SCRIPTS/blocked.sh" 2>/dev/null; then
	got="$(canned_agents_json | pick_blocked)"
	assert_eq "blocked pick_blocked: first blocked agent" \
		"$got" $'mp/alpha/fix-login\t%7\tfix-login\talpha'
	got="$(printf '{"agents": []}' | pick_blocked)"
	assert_eq "blocked pick_blocked: empty when none blocked" "$got" ""
else
	fail "source blocked.sh" "could not source $SCRIPTS/blocked.sh"
fi

# ---- Unit: monkeypuzzle.tmux chord indicator --------------------------------
# shellcheck source=../monkeypuzzle.tmux
if source "$DIR/../monkeypuzzle.tmux" 2>/dev/null; then
	assert_eq "chord_indicator: wraps text in a key-table conditional" \
		"$(chord_indicator '[mp]')" \
		'#{?#{==:#{client_key_table},monkeypuzzle},[mp],}'
	assert_eq "inject_indicator: replaces the placeholder in place" \
		"$(inject_indicator 'left #{monkeypuzzle_chord} | %H:%M' '[IND]')" \
		'left [IND] | %H:%M'
	assert_eq "inject_indicator: no placeholder -> unchanged" \
		"$(inject_indicator 'plain status | %H:%M' '[IND]')" \
		'plain status | %H:%M'
else
	fail "source monkeypuzzle.tmux" "could not source $DIR/../monkeypuzzle.tmux"
fi

# ---- Integration: the go.sh one-stop picker ---------------------------------
integration_go() {
	if ! have jq || ! have fzf; then
		skip "go picker integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/mp.log"
	mkdir -p "$bin"

	# Stub mp: emit canned JSON for `go`, record everything else. `create`
	# also records $PWD — the flow selects the project by cd'ing into it.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
case "\$1" in
  go) cat "$tmp/canned.json" ;;
  switch) printf '%s\n' "\$*" > "$log" ;;
  create) printf '%s|%s\n' "\$PWD" "\$*" > "$log" ;;
  *) exit 2 ;;
esac
EOF
	chmod +x "$bin/mp"
	canned_json_at "$tmp" >"$tmp/canned.json"

	# Piece row (marker + badge in the display must not break filtering).
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="fix-login" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: piece row -> mp switch --project --piece" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --piece fix-login"

	# Project main row.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="beta/(main)" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: main row -> mp switch --project only" \
		"$(cat "$log" 2>/dev/null)" "switch --project beta"

	# Branch adoption row.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="feature/x" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: branch row -> mp switch --branch" \
		"$(cat "$log" 2>/dev/null)" "switch --project alpha --branch feature/x"

	# Create row + typed name: cd into the project, mp create --name.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="beta/(+ new" \
		MP_PLUGIN_INPUT_NAME="my-piece" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: create row + name -> mp create --name in the project" \
		"$(cat "$log" 2>/dev/null)" "$tmp/repos/beta|create --name my-piece"

	# Create row + blank name: falls through to the free-form prompt.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="beta/(+ new" \
		MP_PLUGIN_INPUT_NAME="" MP_PLUGIN_INPUT_PROMPT="do the thing" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: create row + description -> mp create --prompt" \
		"$(cat "$log" 2>/dev/null)" "$tmp/repos/beta|create --prompt do the thing"

	# ctrl-n with no matching row: creates in the current project.
	rm -f "$log"
	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="zzz-nomatch" \
		MP_PLUGIN_KEY="ctrl-n" MP_PLUGIN_INPUT_NAME="quick" \
		bash "$SCRIPTS/go.sh" >/dev/null 2>&1
	assert_eq "go flow: ctrl-n falls back to the current project" \
		"$(cat "$log" 2>/dev/null)" "$tmp/repos/alpha|create --name quick"

	rm -rf "$tmp"
}
integration_go

# ---- Integration: agents picker focuses the selected pane ------------------
integration_agents() {
	if ! have jq || ! have fzf; then
		skip "agents integration" "needs jq + fzf"
		return
	fi
	local tmp bin log
	tmp="$(mktemp -d)"
	bin="$tmp/bin"
	log="$tmp/tmux.log"
	mkdir -p "$bin"

	# Stub mp: canned agent list. Stub tmux: record every call; has-session ok.
	cat >"$bin/mp" <<EOF
#!/usr/bin/env bash
[[ "\$1 \$2" == "agent list" ]] && cat "$tmp/agents.json"
EOF
	cat >"$bin/tmux" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$log"
exit 0
EOF
	chmod +x "$bin/mp" "$bin/tmux"
	canned_agents_json >"$tmp/agents.json"

	PATH="$bin:$PATH" TMUX="fake,1,0" MP_PLUGIN_FILTER="fix-login" \
		bash "$SCRIPTS/agents.sh" >/dev/null 2>&1
	if grep -q -- 'switch-client -t =mp/alpha/fix-login' "$log" 2>/dev/null &&
		grep -q -- 'select-pane -t %7' "$log" 2>/dev/null; then
		ok "agents flow: selection switches session and focuses pane"
	else
		fail "agents flow" "tmux calls: $(tr '\n' '; ' <"$log" 2>/dev/null)"
	fi

	rm -rf "$tmp"
}
integration_agents

printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[[ "$FAIL" -eq 0 ]]
