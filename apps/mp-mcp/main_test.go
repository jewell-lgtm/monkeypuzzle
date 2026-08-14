package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleInitialize(t *testing.T) {
	server := &Server{mpPath: "mp"}
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := server.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if result.ServerInfo.Name != "monkeypuzzle-mcp" {
		t.Errorf("expected server name 'monkeypuzzle-mcp', got %q", result.ServerInfo.Name)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version '2024-11-05', got %q", result.ProtocolVersion)
	}
}

func TestHandleToolsList(t *testing.T) {
	server := &Server{mpPath: "mp"}
	req := &Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := server.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(ToolsListResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}

	expectedTools := []string{
		"mp_init",
		"mp_piece_create",
		"mp_piece_update",
		"mp_piece_merge",
		"mp_agent_list",
		"mp_wait",
	}

	if len(result.Tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing expected tool: %s", name)
		}
	}
}

// stubMp writes an executable that echoes its argv, for asserting the exact
// mp invocation a tool builds.
func stubMp(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mp")
	script := "#!/bin/sh\necho \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecuteTool_AgentList(t *testing.T) {
	server := &Server{mpPath: stubMp(t)}
	out, isError := server.executeTool("mp_agent_list", map[string]string{"all": "true"})
	if isError {
		t.Fatalf("unexpected error: %s", out)
	}
	if strings.TrimSpace(out) != "agent list --json --all" {
		t.Errorf("unexpected argv: %q", out)
	}
}

func TestExecuteTool_Wait(t *testing.T) {
	server := &Server{mpPath: stubMp(t)}
	out, isError := server.executeTool("mp_wait", map[string]string{"pieces": "a b", "timeout": "1m"})
	if isError {
		t.Fatalf("unexpected error: %s", out)
	}
	if strings.TrimSpace(out) != "wait --timeout 1m a b" {
		t.Errorf("unexpected argv: %q", out)
	}

	// Default timeout keeps MCP calls bounded.
	out, _ = server.executeTool("mp_wait", map[string]string{})
	if strings.TrimSpace(out) != "wait --timeout 5m" {
		t.Errorf("unexpected default argv: %q", out)
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	server := &Server{mpPath: "mp"}
	req := &Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "unknown/method",
	}

	resp := server.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestHandleInitializedNotification(t *testing.T) {
	server := &Server{mpPath: "mp"}
	req := &Request{
		JSONRPC: "2.0",
		Method:  "initialized",
	}

	resp := server.handleRequest(req)
	if resp != nil {
		t.Error("expected nil response for notification")
	}
}

func TestToolCallUnknownTool(t *testing.T) {
	server := &Server{mpPath: "mp"}
	params, _ := json.Marshal(ToolCallParams{
		Name:      "mp_does_not_exist",
		Arguments: json.RawMessage(`{}`),
	})
	req := &Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  params,
	}

	resp := server.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response")
	}

	result, ok := resp.Result.(ToolCallResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if !result.IsError {
		t.Error("expected IsError=true for unknown tool")
	}
}
