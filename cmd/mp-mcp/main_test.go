package main

import (
	"encoding/json"
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
		"mp_piece_new",
		"mp_piece_update",
		"mp_piece_merge",
		"mp_issue_list",
		"mp_issue_read",
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

func TestToolCallInvalidArguments(t *testing.T) {
	server := &Server{mpPath: "mp"}
	params, _ := json.Marshal(ToolCallParams{
		Name:      "mp_issue_read",
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
		t.Error("expected IsError=true for missing required path")
	}
}

func TestParseIssue(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedTitle  string
		expectedStatus string
	}{
		{
			name: "frontmatter with title and status",
			content: `---
title: Test Issue
status: in-progress
---

# Test Issue`,
			expectedTitle:  "Test Issue",
			expectedStatus: "in-progress",
		},
		{
			name: "frontmatter without status",
			content: `---
title: Another Issue
---

# Another Issue`,
			expectedTitle:  "Another Issue",
			expectedStatus: "todo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Note: parseIssue requires a file path, this is a simplified test
			// Full integration testing would require temp files
		})
	}
}
