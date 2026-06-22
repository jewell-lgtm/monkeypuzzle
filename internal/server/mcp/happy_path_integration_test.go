//go:build integration

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.temporal.io/sdk/testsuite"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	mpsync "github.com/jewell-lgtm/monkeypuzzle/internal/server/sync"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

type noopTrigger struct{}

func (noopTrigger) StartSync(context.Context, int64) (string, error) { return "", nil }
func (noopTrigger) SyncStatus(context.Context, int64) (store.SyncStatus, error) {
	return store.SyncStatus{Status: store.SyncIdle}, nil
}

// bearerRT injects an Authorization header on every request (or none, to test
// the unauthenticated path).
type bearerRT struct {
	token string
	base  http.RoundTripper
}

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.base.RoundTrip(r)
}

// TestMCP_HappyPath_SyncThenListStacks is the primary outside-in test: it drives
// the real MCP HTTP surface (bearer auth) end to end — stub GitHub -> sync
// workflow (testsuite) -> store -> service -> MCP tools — and asserts the 3-PR
// stack is reconstructed with correct nesting.
func TestMCP_HappyPath_SyncThenListStacks(t *testing.T) {
	ctx := context.Background()

	// --- store + encrypted token + user ---
	mem := store.NewMemoryStore()
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, err := crypto.NewAESGCMCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt([]byte("tok-123"))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := mem.UpsertUser(ctx, store.User{Provider: "github", ForgeUserID: 4242, ForgeLogin: "octo", AccessTokenEnc: enc})

	// --- stub GitHub: 1 repo, a 3-PR stack ---
	stub := forge.NewStubFactory()
	stub.SetUser("tok-123", forge.User{ID: 4242, Login: "octo"})
	stub.SetRepos("tok-123", forge.Repo{ForgeRepoID: 7, Owner: "o", Name: "r", DefaultBranch: "main"})
	stub.SetPullRequests("tok-123", "o", "r",
		stackgraph.PRRef{Number: 1, HeadRef: "feat-a", BaseRef: "main", State: stackgraph.StateOpen, Title: "A"},
		stackgraph.PRRef{Number: 2, HeadRef: "feat-b", BaseRef: "feat-a", State: stackgraph.StateOpen, Title: "B"},
		stackgraph.PRRef{Number: 3, HeadRef: "feat-c", BaseRef: "feat-b", State: stackgraph.StateOpen, Title: "C"},
	)

	// --- run the real sync workflow (Temporal test env) to populate the store ---
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivity(&mpsync.Activities{Store: mem, Factory: stub, Cipher: cipher})
	env.ExecuteWorkflow(mpsync.SyncUserDataWorkflow, mpsync.SyncInput{UserID: uid})
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("sync workflow failed: completed=%v err=%v", env.IsWorkflowCompleted(), env.GetWorkflowError())
	}

	// --- MCP server over the populated store, behind bearer auth ---
	svc := service.New(mem, noopTrigger{})
	srv := NewServer(svc)
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "test-token" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: strconv.FormatInt(uid, 10), Expiration: time.Now().Add(time.Hour)}, nil
	}
	httpSrv := httptest.NewServer(NewHTTPHandler(srv, verifier, "http://example/.well-known/oauth-protected-resource"))
	defer httpSrv.Close()

	// --- authenticated MCP client ---
	authedClient := &http.Client{Transport: bearerRT{token: "test-token", base: http.DefaultTransport}}
	session := connect(t, ctx, httpSrv.URL, authedClient)
	defer session.Close()

	// list_repos -> the one repo
	var repos struct {
		Repos []struct{ Owner, Name string } `json:"repos"`
	}
	callTool(t, ctx, session, "list_repos", nil, &repos)
	if len(repos.Repos) != 1 || repos.Repos[0].Owner != "o" || repos.Repos[0].Name != "r" {
		t.Fatalf("list_repos: %+v", repos.Repos)
	}

	// list_stacks -> 3-PR stack with correct nesting
	var stacks struct {
		Stacks []struct {
			RootNumber int `json:"root_number"`
			PRs        []struct {
				Number       int `json:"number"`
				ParentNumber int `json:"parent_number"`
				Depth        int `json:"depth"`
			} `json:"prs"`
		} `json:"stacks"`
	}
	callTool(t, ctx, session, "list_stacks", map[string]any{"owner": "o", "name": "r"}, &stacks)
	if len(stacks.Stacks) != 1 {
		t.Fatalf("want 1 stack, got %d: %+v", len(stacks.Stacks), stacks.Stacks)
	}
	st := stacks.Stacks[0]
	if st.RootNumber != 1 || len(st.PRs) != 3 {
		t.Fatalf("unexpected stack: %+v", st)
	}
	byNum := map[int]struct{ ParentNumber, Depth int }{}
	for _, p := range st.PRs {
		byNum[p.Number] = struct{ ParentNumber, Depth int }{p.ParentNumber, p.Depth}
	}
	if byNum[1] != (struct{ ParentNumber, Depth int }{0, 0}) ||
		byNum[2] != (struct{ ParentNumber, Depth int }{1, 1}) ||
		byNum[3] != (struct{ ParentNumber, Depth int }{2, 2}) {
		t.Fatalf("stack nesting wrong: %+v", byNum)
	}

	// --- unauthenticated client is rejected (401 / OAuth challenge) ---
	noAuth := &http.Client{Transport: bearerRT{token: "", base: http.DefaultTransport}}
	if _, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "0"}, nil).
		Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: httpSrv.URL, HTTPClient: noAuth}, nil); err == nil {
		t.Fatal("expected unauthenticated connect to fail")
	}
}

func connect(t *testing.T, ctx context.Context, url string, hc *http.Client) *mcpsdk.ClientSession {
	t.Helper()
	c := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := c.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: url, HTTPClient: hc}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return session
}

func callTool(t *testing.T, ctx context.Context, s *mcpsdk.ClientSession, name string, args any, out any) {
	t.Helper()
	res, err := s.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("call %s returned tool error: %+v", name, res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal %s output: %v", name, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal %s output: %v", name, err)
	}
}
