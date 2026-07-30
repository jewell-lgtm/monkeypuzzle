package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// JSON-RPC 2.0 types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCP protocol types
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct{}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"inputSchema"`
}

type JSONSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Server struct {
	mpPath string
}

func main() {
	server := &Server{mpPath: findMpBinary()}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeResponse(errorResponse(nil, -32700, "Parse error", err.Error()))
			continue
		}

		resp := server.handleRequest(&req)
		if resp != nil {
			writeResponse(resp)
		}
	}
}

func findMpBinary() string {
	if exe, err := os.Executable(); err == nil {
		mpPath := filepath.Join(filepath.Dir(exe), "mp")
		if _, err := os.Stat(mpPath); err == nil {
			return mpPath
		}
	}
	if mpPath, err := exec.LookPath("mp"); err == nil {
		return mpPath
	}
	return "mp"
}

func (s *Server) handleRequest(req *Request) *Response {
	switch req.Method {
	case "initialize":
		return successResponse(req.ID, InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    Capabilities{Tools: &ToolsCapability{}},
			ServerInfo:      ServerInfo{Name: "monkeypuzzle-mcp", Version: "0.1.0"},
		})
	case "initialized":
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return errorResponse(req.ID, -32601, "Method not found", nil)
	}
}

func (s *Server) handleToolsList(req *Request) *Response {
	tools := []Tool{
		{
			Name:        "mp_init",
			Description: "Initialize monkeypuzzle in a directory",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"name":        {Type: "string", Description: "Project name"},
					"pr_provider": {Type: "string", Description: "PR provider (default: github)"},
					"cwd":         {Type: "string", Description: "Working directory"},
				},
			},
		},
		{
			Name:        "mp_piece_new",
			Description: "Create new piece (git worktree + tmux session)",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"name": {Type: "string", Description: "Piece name"},
					"cwd":  {Type: "string", Description: "Working directory"},
				},
			},
		},
		{
			Name:        "mp_piece_update",
			Description: "Update piece with latest from main branch",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"main_branch": {Type: "string", Description: "Main branch name (default: main)"},
					"cwd":         {Type: "string", Description: "Working directory (piece worktree)"},
				},
			},
		},
		{
			Name:        "mp_piece_merge",
			Description: "Merge piece back into main branch",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"main_branch": {Type: "string", Description: "Main branch name (default: main)"},
					"cwd":         {Type: "string", Description: "Working directory (piece worktree)"},
				},
			},
		},
		{
			Name:        "mp_agent_list",
			Description: "List live agents across pieces (blocked first): status working/blocked/done/idle per agent, aggregated per piece",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"all": {Type: "string", Description: "\"true\" to span all registered projects instead of the current one"},
					"cwd": {Type: "string", Description: "Working directory (scopes the project)"},
				},
			},
		},
		{
			Name:        "mp_wait",
			Description: "Block until agents settle (no agent working) in the given pieces, or everywhere. Returns per-piece aggregates; blocked pieces need human input",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"pieces":  {Type: "string", Description: "Space-separated piece names (empty = all pieces with agents)"},
					"timeout": {Type: "string", Description: "Give up after this long, e.g. \"10m\" (default: \"5m\")"},
					"cwd":     {Type: "string", Description: "Working directory (scopes the project)"},
				},
			},
		},
	}
	return successResponse(req.ID, ToolsListResult{Tools: tools})
}

func (s *Server) handleToolsCall(req *Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	var args map[string]string
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errorResponse(req.ID, -32602, "Invalid arguments", err.Error())
		}
	}
	if args == nil {
		args = make(map[string]string)
	}

	result, isError := s.executeTool(params.Name, args)
	return successResponse(req.ID, ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: result}},
		IsError: isError,
	})
}

func (s *Server) executeTool(name string, args map[string]string) (string, bool) {
	cwd := args["cwd"]
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	var cmdArgs []string
	var stdin string

	switch name {
	case "mp_init":
		cmdArgs = []string{"init", "--yes"}
		input := map[string]string{}
		if v := args["name"]; v != "" {
			input["name"] = v
		}
		if v := args["pr_provider"]; v != "" {
			input["pr_provider"] = v
		}
		if len(input) > 0 {
			data, _ := json.Marshal(input)
			stdin = string(data)
		}

	case "mp_piece_new":
		cmdArgs = []string{"create"}
		if v := args["name"]; v != "" {
			cmdArgs = append(cmdArgs, "--name", v)
		}

	case "mp_piece_update":
		cmdArgs = []string{"update"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main-branch", v)
		}

	case "mp_piece_merge":
		cmdArgs = []string{"merge"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main-branch", v)
		}

	case "mp_agent_list":
		cmdArgs = []string{"agent", "list", "--json"}
		if args["all"] == "true" {
			cmdArgs = append(cmdArgs, "--all")
		}

	case "mp_wait":
		timeout := args["timeout"]
		if timeout == "" {
			// An MCP call should always come back; forever-wait is a CLI luxury.
			timeout = "5m"
		}
		cmdArgs = []string{"wait", "--timeout", timeout}
		if v := args["pieces"]; v != "" {
			cmdArgs = append(cmdArgs, strings.Fields(v)...)
		}

	default:
		return fmt.Sprintf("Unknown tool: %s", name), true
	}

	return s.runMp(cwd, cmdArgs, stdin)
}

func (s *Server) runMp(cwd string, args []string, stdin string) (string, bool) {
	cmd := exec.Command(s.mpPath, args...)
	cmd.Dir = cwd
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			return err.Error(), true
		}
		return string(output), true
	}
	return string(output), false
}

func successResponse(id any, result any) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id any, code int, message string, data any) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: message, Data: data}}
}

func writeResponse(resp *Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
