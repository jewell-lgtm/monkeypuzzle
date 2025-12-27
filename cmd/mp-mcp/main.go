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
					"name":           {Type: "string", Description: "Project name"},
					"issue_provider": {Type: "string", Description: "Issue provider (default: markdown)"},
					"pr_provider":    {Type: "string", Description: "PR provider (default: github)"},
					"cwd":            {Type: "string", Description: "Working directory"},
				},
			},
		},
		{
			Name:        "mp_piece_new",
			Description: "Create new piece (git worktree + tmux session)",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"name":  {Type: "string", Description: "Piece name"},
					"issue": {Type: "string", Description: "Path to issue file"},
					"cwd":   {Type: "string", Description: "Working directory"},
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
			Name:        "mp_issue_list",
			Description: "List issues in the issues directory",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"status": {Type: "string", Description: "Filter by status: todo, in-progress, done"},
					"cwd":    {Type: "string", Description: "Working directory"},
				},
			},
		},
		{
			Name:        "mp_issue_read",
			Description: "Read content of an issue file",
			InputSchema: JSONSchema{
				Type:       "object",
				Properties: map[string]Property{"path": {Type: "string", Description: "Path to issue file"}, "cwd": {Type: "string", Description: "Working directory"}},
				Required:   []string{"path"},
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
		if v := args["issue_provider"]; v != "" {
			input["issue_provider"] = v
		}
		if v := args["pr_provider"]; v != "" {
			input["pr_provider"] = v
		}
		if len(input) > 0 {
			data, _ := json.Marshal(input)
			stdin = string(data)
		}

	case "mp_piece_new":
		cmdArgs = []string{"piece", "new"}
		if v := args["name"]; v != "" {
			cmdArgs = append(cmdArgs, "--name", v)
		}
		if v := args["issue"]; v != "" {
			cmdArgs = append(cmdArgs, "--issue", v)
		}

	case "mp_piece_update":
		cmdArgs = []string{"piece", "update"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main-branch", v)
		}

	case "mp_piece_merge":
		cmdArgs = []string{"piece", "merge"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main-branch", v)
		}

	case "mp_issue_list":
		return s.listIssues(cwd, args["status"])

	case "mp_issue_read":
		if path := args["path"]; path != "" {
			return s.readIssue(cwd, path)
		}
		return "Error: path is required", true

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

func (s *Server) listIssues(cwd, statusFilter string) (string, bool) {
	issuesDir := filepath.Join(cwd, "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	type Issue struct {
		Path   string `json:"path"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	var issues []Issue

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join("issues", e.Name())
		title, status := parseIssue(filepath.Join(cwd, path))
		if statusFilter != "" && status != statusFilter {
			continue
		}
		issues = append(issues, Issue{Path: path, Title: title, Status: status})
	}

	data, _ := json.MarshalIndent(issues, "", "  ")
	return string(data), false
}

func (s *Server) readIssue(cwd, path string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(cwd, path))
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return string(content), false
}

func parseIssue(path string) (title, status string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(path), "todo"
	}
	text := string(content)
	status = "todo"

	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end > 0 {
			fm := text[4 : 4+end]
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "title:") {
					title = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "title:")), `"'`)
				}
				if strings.HasPrefix(line, "status:") {
					status = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "status:")), `"'`)
				}
			}
		}
	}

	if title == "" {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "# "))
				break
			}
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return
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
