package agent_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func newTestHandler() (*agent.Handler, *adapters.MemoryFS, *adapters.MockExec) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	h := agent.NewHandler(core.Deps{FS: fs, Output: out, Exec: mockExec})
	h.Alive = func(pid int) bool { return true }
	return h, fs, mockExec
}

var testLoc = agent.Location{
	PieceName:    "p1",
	WorktreePath: "/pieces/p1",
	RepoRoot:     "/repo",
}

func TestReport_UpsertAndAggregate(t *testing.T) {
	h, fs, _ := newTestHandler()
	ctx := context.Background()

	result, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "a1", Kind: "claude", Status: piece.AgentWorking, PID: 100})
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}
	if !result.Reported || result.Aggregate != piece.AgentWorking {
		t.Errorf("expected reported working aggregate, got %+v", result)
	}

	// A second, blocked agent wins the aggregate.
	result, err = h.Report(ctx, testLoc, agent.ReportInput{ID: "a2", Status: piece.AgentBlocked, PID: 200})
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}
	if result.Aggregate != piece.AgentBlocked {
		t.Errorf("expected blocked aggregate, got %q", result.Aggregate)
	}

	metadata, err := piece.ReadPieceMetadata(testLoc.WorktreePath, fs)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if len(metadata.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(metadata.Agents))
	}
	if metadata.Agents["a1"].Kind != "claude" {
		t.Errorf("expected kind claude, got %q", metadata.Agents["a1"].Kind)
	}
}

func TestReport_GoneRemovesRecord(t *testing.T) {
	h, fs, _ := newTestHandler()
	ctx := context.Background()

	if _, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "a1", Status: piece.AgentWorking, PID: 100}); err != nil {
		t.Fatalf("report failed: %v", err)
	}
	result, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "a1", Status: agent.StatusGone, PID: 100})
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}
	if result.Aggregate != "" {
		t.Errorf("expected empty aggregate, got %q", result.Aggregate)
	}

	metadata, _ := piece.ReadPieceMetadata(testLoc.WorktreePath, fs)
	if len(metadata.Agents) != 0 {
		t.Errorf("expected no agents, got %d", len(metadata.Agents))
	}
}

func TestReport_ReapsDeadPIDs(t *testing.T) {
	h, fs, _ := newTestHandler()
	ctx := context.Background()

	if _, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "dead", Status: piece.AgentWorking, PID: 100}); err != nil {
		t.Fatalf("report failed: %v", err)
	}

	h.Alive = func(pid int) bool { return pid != 100 }
	if _, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "live", Status: piece.AgentIdle, PID: 200}); err != nil {
		t.Fatalf("report failed: %v", err)
	}

	metadata, _ := piece.ReadPieceMetadata(testLoc.WorktreePath, fs)
	if _, exists := metadata.Agents["dead"]; exists {
		t.Error("expected dead agent to be reaped")
	}
	if _, exists := metadata.Agents["live"]; !exists {
		t.Error("expected live agent to remain")
	}
}

func TestReport_BlockedTransitionFiresDetachedHook(t *testing.T) {
	h, fs, mockExec := newTestHandler()
	ctx := context.Background()

	hookPath := filepath.Join("/repo/.monkeypuzzle/hooks", piece.HookAgentBlocked)
	_ = fs.MkdirAll("/repo/.monkeypuzzle/hooks", 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\n"), 0755)

	if _, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "a1", Kind: "claude", Status: piece.AgentBlocked, PID: 100, Pane: "%3"}); err != nil {
		t.Fatalf("report failed: %v", err)
	}

	calls := mockExec.GetDetachedCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 detached hook call, got %d", len(calls))
	}
	env := strings.Join(calls[0].Env, "\n")
	for _, want := range []string{"MP_AGENT_STATUS=blocked", "MP_AGENT_ID=a1", "MP_AGENT_KIND=claude", "MP_AGENT_PANE=%3", "MP_PIECE_NAME=p1"} {
		if !strings.Contains(env, want) {
			t.Errorf("hook env missing %s", want)
		}
	}

	// Re-reporting blocked is not a transition: no second fire.
	if _, err := h.Report(ctx, testLoc, agent.ReportInput{ID: "a1", Status: piece.AgentBlocked, PID: 100}); err != nil {
		t.Fatalf("report failed: %v", err)
	}
	if len(mockExec.GetDetachedCalls()) != 1 {
		t.Error("expected no hook fire without an aggregate transition")
	}
}

func TestReport_InvalidStatus(t *testing.T) {
	h, _, _ := newTestHandler()
	if _, err := h.Report(context.Background(), testLoc, agent.ReportInput{ID: "a1", Status: "napping"}); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestAggregateAgents(t *testing.T) {
	cases := []struct {
		statuses []string
		want     string
	}{
		{nil, ""},
		{[]string{piece.AgentIdle}, piece.AgentIdle},
		{[]string{piece.AgentIdle, piece.AgentDone}, piece.AgentDone},
		{[]string{piece.AgentDone, piece.AgentWorking}, piece.AgentWorking},
		{[]string{piece.AgentWorking, piece.AgentBlocked, piece.AgentIdle}, piece.AgentBlocked},
	}
	for _, tc := range cases {
		agents := make(map[string]piece.AgentRecord)
		for i, s := range tc.statuses {
			agents[string(rune('a'+i))] = piece.AgentRecord{Status: s}
		}
		if got := piece.AggregateAgents(agents); got != tc.want {
			t.Errorf("AggregateAgents(%v) = %q, want %q", tc.statuses, got, tc.want)
		}
	}
}

func TestFromClaudeHook(t *testing.T) {
	cases := []struct {
		event  string
		status string
		ok     bool
	}{
		{"SessionStart", piece.AgentIdle, true},
		{"UserPromptSubmit", piece.AgentWorking, true},
		{"Notification", piece.AgentBlocked, true},
		{"Stop", piece.AgentDone, true},
		{"SessionEnd", agent.StatusGone, true},
		{"SubagentStop", "", false},
	}
	for _, tc := range cases {
		payload := `{"session_id":"sess-1","hook_event_name":"` + tc.event + `"}`
		input, ok, err := agent.FromClaudeHook([]byte(payload))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.event, err)
		}
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.event, ok, tc.ok)
			continue
		}
		if ok && (input.Status != tc.status || input.ID != "sess-1" || input.Kind != "claude") {
			t.Errorf("%s: got %+v, want status %q", tc.event, input, tc.status)
		}
	}

	if _, _, err := agent.FromClaudeHook([]byte("not json")); err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestSummary(t *testing.T) {
	items := []agent.ListItem{
		{Status: piece.AgentWorking},
		{Status: piece.AgentWorking},
		{Status: piece.AgentBlocked},
	}
	if got := agent.Summary(items); got != "🔴1 ⚡2" {
		t.Errorf("Summary = %q, want %q", got, "🔴1 ⚡2")
	}
	if got := agent.Summary(nil); got != "" {
		t.Errorf("Summary(nil) = %q, want empty", got)
	}
}
