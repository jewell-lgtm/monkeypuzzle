package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
			Name:        "mp_piece_create",
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
					"all": {Type: "boolean", Description: "Span all registered projects instead of the current one"},
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
		{
			Name:        "mp_inbox",
			Description: "The global cross-repo inbox: every piece across all registered projects as one list, ordered by your manual rank then urgency (blocked > review > working > idle > merged). Returns {\"rows\":[…]}",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"sort":    {Type: "string", Description: "\"rank\" (your order, urgency breaks ties; default) or \"urgency\""},
					"refresh": {Type: "boolean", Description: "Re-fetch PR state instead of the 2-minute cache"},
					"cwd":     {Type: "string", Description: "Working directory (an init'd repo here is included even if unregistered)"},
				},
			},
		},
		{
			Name:        "mp_inbox_move",
			Description: "Re-rank a piece in the inbox; give exactly one placement. Returns {\"key\",\"rank\",\"from_rank\"}",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"piece":  {Type: "string", Description: "Piece selector: project/piece, or a bare piece name (this project's first, else must be unique)"},
					"top":    {Type: "boolean", Description: "Rank 1"},
					"bottom": {Type: "boolean", Description: "Last rank"},
					"up":     {Type: "integer", Description: "N ranks higher"},
					"down":   {Type: "integer", Description: "N ranks lower"},
					"before": {Type: "string", Description: "Place directly above this piece selector"},
					"after":  {Type: "string", Description: "Place directly below this piece selector"},
					"cwd":    {Type: "string", Description: "Working directory (scopes bare selectors)"},
				},
				Required: []string{"piece"},
			},
		},
		{
			Name:        "mp_inbox_note",
			Description: "Set a piece's inbox note; empty note or clear removes it. Returns {\"key\",\"note\"}",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"piece": {Type: "string", Description: "Piece selector: project/piece, or a bare piece name"},
					"note":  {Type: "string", Description: "Note text"},
					"clear": {Type: "boolean", Description: "Remove the note"},
					"cwd":   {Type: "string", Description: "Working directory (scopes bare selectors)"},
				},
				Required: []string{"piece"},
			},
		},
		{
			Name:        "mp_inbox_snooze",
			Description: "Park a piece at the bottom of the inbox (next/prev skip it) until a time; give one of for, until, clear. Returns {\"key\",\"snoozed_until\"}",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"piece": {Type: "string", Description: "Piece selector: project/piece, or a bare piece name"},
					"for":   {Type: "string", Description: "Duration, e.g. 90m, 2h, 2d"},
					"until": {Type: "string", Description: "RFC3339 time"},
					"clear": {Type: "boolean", Description: "Un-snooze"},
					"cwd":   {Type: "string", Description: "Working directory (scopes bare selectors)"},
				},
				Required: []string{"piece"},
			},
		},
		{
			Name:        "mp_inbox_next",
			Description: "Resolve the inbox row after the piece cwd stands in (wrapping, snoozed rows skipped; outside a piece: rank 1) and return its switch result. No multiplexer switch happens for a non-TTY caller, same as mp go",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"sort": {Type: "string", Description: "Order to step through: \"rank\" (default) or \"urgency\""},
					"cwd":  {Type: "string", Description: "Working directory (the piece you stand in)"},
				},
			},
		},
		{
			Name:        "mp_inbox_prev",
			Description: "Resolve the inbox row before the piece cwd stands in (wrapping, snoozed rows skipped; outside a piece: the last row) and return its switch result. No multiplexer switch happens for a non-TTY caller, same as mp go",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"sort": {Type: "string", Description: "Order to step through: \"rank\" (default) or \"urgency\""},
					"cwd":  {Type: "string", Description: "Working directory (the piece you stand in)"},
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

	// Decode loosely and stringify values: LLM clients mix types freely (a
	// boolean true for "all", a number for a count), and one wrong-typed field
	// must not fail the whole call the way a map[string]string decode would.
	var raw map[string]any
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &raw); err != nil {
			return errorResponse(req.ID, -32602, "Invalid arguments", err.Error())
		}
	}
	args := make(map[string]string, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			args[key] = v
		case bool:
			args[key] = fmt.Sprintf("%t", v)
		case float64:
			args[key] = strconv.FormatFloat(v, 'f', -1, 64)
		}
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

	case "mp_piece_create":
		cmdArgs = []string{"create"}
		if v := args["name"]; v != "" {
			cmdArgs = append(cmdArgs, "--name", v)
		}

	case "mp_piece_update":
		cmdArgs = []string{"update"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main", v)
		}

	case "mp_piece_merge":
		cmdArgs = []string{"merge"}
		if v := args["main_branch"]; v != "" {
			cmdArgs = append(cmdArgs, "--main", v)
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

	case "mp_inbox":
		cmdArgs = []string{"inbox", "--json"}
		if v := args["sort"]; v != "" {
			cmdArgs = append(cmdArgs, "--sort", v)
		}
		if args["refresh"] == "true" {
			cmdArgs = append(cmdArgs, "--refresh")
		}

	case "mp_inbox_move":
		cmdArgs = []string{"inbox", "move", "--json"}
		stdin = inboxStdin(args, []string{"before", "after"}, []string{"top", "bottom"}, []string{"up", "down"})

	case "mp_inbox_note":
		cmdArgs = []string{"inbox", "note", "--json"}
		stdin = inboxStdin(args, []string{"note"}, []string{"clear"}, nil)

	case "mp_inbox_snooze":
		cmdArgs = []string{"inbox", "snooze", "--json"}
		stdin = inboxStdin(args, []string{"for", "until"}, []string{"clear"}, nil)

	case "mp_inbox_next", "mp_inbox_prev":
		cmdArgs = []string{"inbox", strings.TrimPrefix(name, "mp_inbox_"), "--json"}
		if v := args["sort"]; v != "" {
			cmdArgs = append(cmdArgs, "--sort", v)
		}

	default:
		return fmt.Sprintf("Unknown tool: %s", name), true
	}

	return s.runMp(cwd, cmdArgs, stdin)
}

// inboxStdin builds the stdin JSON an `mp inbox` verb reads (its --schema
// shape): the piece selector plus whichever of the named string, bool and
// int fields the caller set. Types are restored from the stringified args.
func inboxStdin(args map[string]string, strs, bools, ints []string) string {
	in := map[string]any{"piece": args["piece"]}
	for _, k := range strs {
		if v := args[k]; v != "" {
			in[k] = v
		}
	}
	for _, k := range bools {
		if args[k] == "true" {
			in[k] = true
		}
	}
	for _, k := range ints {
		if n, err := strconv.Atoi(args[k]); err == nil && n != 0 {
			in[k] = n
		}
	}
	data, _ := json.Marshal(in)
	return string(data)
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
