// Package mcp exposes mp server's read API to AI agents over the Model Context
// Protocol (Streamable HTTP transport). It is one of the two presentation
// adapters over internal/server/service (the other being the HTML UI); both
// call the same user-scoped read layer. Agents authenticate with an OAuth 2.1
// bearer token; the authenticated user id arrives on the request's TokenInfo.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// NewServer builds the MCP server with the read-only tools registered.
func NewServer(svc *service.Service) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "monkeypuzzle",
		Title:   "Monkeypuzzle",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repos",
		Description: "List the GitHub repositories the authenticated user can access.",
	}, listRepos(svc))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_stacks",
		Description: "List the stacks of pull requests in a repository. A stack is a chain of PRs where each PR's base branch is the head branch of the PR below it.",
	}, listStacks(svc))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_repo",
		Description: "Get a single repository's details and a summary of its open PR stacks.",
	}, getRepo(svc))

	return s
}

// --- tool argument and output types (schemas are inferred from these) ---

type noArgs struct{}

type repoArgs struct {
	Owner string `json:"owner" jsonschema:"repository owner login"`
	Name  string `json:"name" jsonschema:"repository name"`
}

type repoView struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	URL           string `json:"url"`
}

// stackView is a flat (non-recursive, so the JSON schema infers cleanly)
// representation of one stack: its PRs depth-first, each carrying the number of
// the PR it is stacked on (ParentNumber, 0 for the root) and its Depth.
type stackPRView struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	State        string `json:"state"`
	Draft        bool   `json:"draft"`
	HeadRef      string `json:"head_ref"`
	BaseRef      string `json:"base_ref"`
	Author       string `json:"author"`
	URL          string `json:"url"`
	ParentNumber int    `json:"parent_number"`
	Depth        int    `json:"depth"`
}

type stackView struct {
	RootNumber int           `json:"root_number"`
	PRs        []stackPRView `json:"prs"`
}

type reposOut struct {
	Repos []repoView `json:"repos"`
}

type stacksOut struct {
	Repo   repoView    `json:"repo"`
	Stacks []stackView `json:"stacks"`
}

type repoDetailOut struct {
	Repo       repoView `json:"repo"`
	StackCount int      `json:"stack_count"`
	OpenPRs    int      `json:"open_prs"`
}

// --- handlers ---

func listRepos(svc *service.Service) mcp.ToolHandlerFor[noArgs, reposOut] {
	return func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, reposOut, error) {
		uid, err := userID(req)
		if err != nil {
			return nil, reposOut{}, err
		}
		repos, err := svc.ListRepos(ctx, uid)
		if err != nil {
			return nil, reposOut{}, err
		}
		out := reposOut{Repos: make([]repoView, len(repos))}
		for i, r := range repos {
			out.Repos[i] = toRepoView(r)
		}
		return nil, out, nil
	}
}

func listStacks(svc *service.Service) mcp.ToolHandlerFor[repoArgs, stacksOut] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in repoArgs) (*mcp.CallToolResult, stacksOut, error) {
		uid, err := userID(req)
		if err != nil {
			return nil, stacksOut{}, err
		}
		rs, err := svc.RepoStacks(ctx, uid, in.Owner, in.Name)
		if err != nil {
			return nil, stacksOut{}, mapErr(err)
		}
		out := stacksOut{Repo: toRepoView(rs.Repo), Stacks: make([]stackView, len(rs.Stacks))}
		for i, st := range rs.Stacks {
			out.Stacks[i] = toStackView(st)
		}
		return nil, out, nil
	}
}

func getRepo(svc *service.Service) mcp.ToolHandlerFor[repoArgs, repoDetailOut] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in repoArgs) (*mcp.CallToolResult, repoDetailOut, error) {
		uid, err := userID(req)
		if err != nil {
			return nil, repoDetailOut{}, err
		}
		rs, err := svc.RepoStacks(ctx, uid, in.Owner, in.Name)
		if err != nil {
			return nil, repoDetailOut{}, mapErr(err)
		}
		open := 0
		for _, st := range rs.Stacks {
			open += countNodes(st.Root)
		}
		return nil, repoDetailOut{Repo: toRepoView(rs.Repo), StackCount: len(rs.Stacks), OpenPRs: open}, nil
	}
}

// userID extracts the authenticated user id from the request's bearer token.
func userID(req *mcp.CallToolRequest) (int64, error) {
	if req.Extra == nil || req.Extra.TokenInfo == nil {
		return 0, errors.New("mcp: unauthenticated request")
	}
	uid, err := strconv.ParseInt(req.Extra.TokenInfo.UserID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("mcp: invalid user id %q: %w", req.Extra.TokenInfo.UserID, err)
	}
	return uid, nil
}

// mapErr turns a not-found from the service into a clear tool error.
func mapErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("repository not found or not accessible")
	}
	return err
}

func toRepoView(r store.Repo) repoView {
	return repoView{
		Owner:         r.Owner,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		URL:           r.HTMLURL,
	}
}

// toStackView flattens a stack tree into a depth-first PR list, recording each
// PR's parent number and depth so the tree can be reconstructed by the caller.
func toStackView(st stackgraph.Stack) stackView {
	sv := stackView{RootNumber: st.Root.PR.Number}
	var walk func(n *stackgraph.StackNode, parent, depth int)
	walk = func(n *stackgraph.StackNode, parent, depth int) {
		sv.PRs = append(sv.PRs, stackPRView{
			Number:       n.PR.Number,
			Title:        n.PR.Title,
			State:        n.PR.State,
			Draft:        n.PR.Draft,
			HeadRef:      n.PR.HeadRef,
			BaseRef:      n.PR.BaseRef,
			Author:       n.PR.Author,
			URL:          n.PR.URL,
			ParentNumber: parent,
			Depth:        depth,
		})
		for _, c := range n.Children {
			walk(c, n.PR.Number, depth+1)
		}
	}
	walk(st.Root, 0, 0)
	return sv
}

func countNodes(n *stackgraph.StackNode) int {
	count := 1
	for _, c := range n.Children {
		count += countNodes(c)
	}
	return count
}
