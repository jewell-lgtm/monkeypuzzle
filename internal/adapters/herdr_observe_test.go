package adapters

import (
	"context"
	"testing"
)

// herdrAgentsJSON builds a `herdr agent list` payload.
func herdrAgentsJSON(entries ...string) []byte {
	out := `{"id":"cli:agent:list","result":{"type":"agent_list","agents":[`
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return []byte(out + `]}}`)
}

func TestHerdrMultiplexer_ObserveAgents_ScopesByWorkspace(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list"},
		herdrListJSON(`{"workspace_id":"w2","label":"mp/proj/piece"}`), nil)
	// The agent list is server-wide: entries from other workspaces (w1) and
	// prefix-similar ids (w20) must be filtered out by the "w2:" pane prefix.
	exec.AddResponse("herdr", []string{"agent", "list"},
		herdrAgentsJSON(
			`{"pane_id":"w1:p1","agent":"claude","agent_status":"working"}`,
			`{"pane_id":"w2:p1","agent":"Claude","agent_status":"blocked"}`,
			`{"pane_id":"w2:p2","agent":"opencode","agent_status":"done"}`,
			`{"pane_id":"w20:p1","agent":"codex","agent_status":"working"}`,
		), nil)

	mux := NewHerdrMultiplexer(exec)
	obs, err := mux.ObserveAgents(context.Background(), "mp/proj/piece")
	if err != nil {
		t.Fatalf("ObserveAgents() error = %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations scoped to w2, got %d: %+v", len(obs), obs)
	}
	// Kind is lowercased; states pass through; kinds beyond mp's own
	// claude/codex allowlist (opencode) pass through too.
	if obs[0].Pane != "w2:p1" || obs[0].Kind != "claude" || obs[0].Status != "blocked" || obs[0].PID != 0 {
		t.Errorf("unexpected first observation: %+v", obs[0])
	}
	if obs[1].Kind != "opencode" || obs[1].Status != "done" {
		t.Errorf("unexpected second observation: %+v", obs[1])
	}
}

// herdr's "unknown" (and any future state) maps to idle, never to a false
// blocked/working.
func TestHerdrMultiplexer_ObserveAgents_UnknownIsIdle(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list"},
		herdrListJSON(`{"workspace_id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"agent", "list"},
		herdrAgentsJSON(
			`{"pane_id":"w2:p1","agent":"claude","agent_status":"unknown"}`,
			`{"pane_id":"w2:p2","agent":"claude","agent_status":"hibernating"}`,
		), nil)

	mux := NewHerdrMultiplexer(exec)
	obs, err := mux.ObserveAgents(context.Background(), "mp/proj/piece")
	if err != nil {
		t.Fatalf("ObserveAgents() error = %v", err)
	}
	for _, o := range obs {
		if o.Status != "idle" {
			t.Errorf("state for %s = %q, want idle", o.Pane, o.Status)
		}
	}
}

// A missing workspace observes as empty (the piece has no live session), and
// must not even hit the agent list.
func TestHerdrMultiplexer_ObserveAgents_MissingWorkspace(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list"}, herdrListJSON(), nil)

	mux := NewHerdrMultiplexer(exec)
	obs, err := mux.ObserveAgents(context.Background(), "mp/proj/piece")
	if err != nil || obs != nil {
		t.Errorf("ObserveAgents() = %+v, %v; want nil, nil", obs, err)
	}
	if exec.WasCalled("herdr", "agent", "list") {
		t.Error("must not list agents when the workspace doesn't exist")
	}
}
