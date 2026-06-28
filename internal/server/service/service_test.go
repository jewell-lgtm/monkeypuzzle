package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/mprunner"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

type fakeTrigger struct{ started []int64 }

func (f *fakeTrigger) StartSync(_ context.Context, userID int64) (string, error) {
	f.started = append(f.started, userID)
	return "run-id", nil
}

func (f *fakeTrigger) SyncStatus(context.Context, int64) (store.SyncStatus, error) {
	return store.SyncStatus{Status: store.SyncIdle}, nil
}

func seed(t *testing.T) (*Service, *store.MemoryStore, int64) {
	t.Helper()
	ctx := context.Background()
	mem := store.NewMemoryStore()
	uid, _ := mem.UpsertUser(ctx, store.User{Provider: "github", ForgeUserID: 1, ForgeLogin: "octo"})
	rid, _ := mem.UpsertRepo(ctx, store.Repo{Provider: "github", ForgeRepoID: 7, Owner: "o", Name: "r", DefaultBranch: "main"})
	other, _ := mem.UpsertRepo(ctx, store.Repo{Provider: "github", ForgeRepoID: 8, Owner: "x", Name: "hidden", DefaultBranch: "main"})
	_ = other
	if err := mem.SetUserRepos(ctx, uid, []int64{rid}); err != nil {
		t.Fatal(err)
	}
	_ = mem.ReplacePullRequests(ctx, rid, []store.PullRequest{
		{Number: 1, HeadRef: "feat-a", BaseRef: "main", State: "OPEN"},
		{Number: 2, HeadRef: "feat-b", BaseRef: "feat-a", State: "OPEN"},
		{Number: 3, HeadRef: "feat-c", BaseRef: "feat-b", State: "OPEN"},
	})
	return New(mem, &fakeTrigger{}, nil, nil, false), mem, uid
}

// testCryptoKey is a deterministic 32-byte AES key for tests, matching the
// crypto package's own test key.
func testCryptoKey() []byte {
	k := make([]byte, crypto.KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// seedWithToken builds a store with one visible repo (o/r), a 1->2->3 nested
// stack of open PRs, and a user whose encrypted GitHub token is "tok123".
func seedWithToken(t *testing.T) (*store.MemoryStore, crypto.TokenCipher, int64) {
	t.Helper()
	ctx := context.Background()
	cipher, err := crypto.NewAESGCMCipher(testCryptoKey())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt([]byte("tok123"))
	if err != nil {
		t.Fatal(err)
	}
	mem := store.NewMemoryStore()
	uid, _ := mem.UpsertUser(ctx, store.User{Provider: "github", ForgeUserID: 1, ForgeLogin: "octo", AccessTokenEnc: enc})
	rid, _ := mem.UpsertRepo(ctx, store.Repo{Provider: "github", ForgeRepoID: 7, Owner: "o", Name: "r", DefaultBranch: "main"})
	if err := mem.SetUserRepos(ctx, uid, []int64{rid}); err != nil {
		t.Fatal(err)
	}
	_ = mem.ReplacePullRequests(ctx, rid, []store.PullRequest{
		{Number: 1, HeadRef: "feat-a", BaseRef: "main", State: "OPEN"},
		{Number: 2, HeadRef: "feat-b", BaseRef: "feat-a", State: "OPEN"},
		{Number: 3, HeadRef: "feat-c", BaseRef: "feat-b", State: "OPEN"},
	})
	return mem, cipher, uid
}

func TestRepoStacks_UsesMpRunnerWhenEnabled(t *testing.T) {
	mem, cipher, uid := seedWithToken(t)
	canned := []stackgraph.Stack{{Root: &stackgraph.StackNode{
		PR: stackgraph.PRRef{Number: 42, HeadRef: "from-mp", BaseRef: "main", State: stackgraph.StateOpen},
	}}}
	fake := &mprunner.Fake{Stacks: canned}
	svc := New(mem, &fakeTrigger{}, fake, cipher, true)

	rs, err := svc.RepoStacks(context.Background(), uid, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	// The runner's forest is used, NOT the Go path's (which would be 1->2->3).
	if len(rs.Stacks) != 1 || rs.Stacks[0].Root.PR.Number != 42 {
		t.Fatalf("expected runner's canned forest, got %+v", rs.Stacks)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("want exactly 1 runner call, got %d", len(fake.Calls))
	}
	call := fake.Calls[0]
	if call.RepoSlug != "o/r" {
		t.Fatalf("RepoSlug = %q, want o/r", call.RepoSlug)
	}
	if call.Provider != "github" {
		t.Fatalf("Provider = %q, want github", call.Provider)
	}
	if call.Token == "" {
		t.Fatal("Token must be the non-empty decrypted token")
	}
	if call.Token != "tok123" {
		t.Fatalf("Token = %q, want decrypted tok123", call.Token)
	}
}

func TestRepoStacks_FallsBackToGoPathOnRunnerError(t *testing.T) {
	mem, cipher, uid := seedWithToken(t)
	fake := &mprunner.Fake{Err: errors.New("boom")}
	svc := New(mem, &fakeTrigger{}, fake, cipher, true)

	rs, err := svc.RepoStacks(context.Background(), uid, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	// Identical to the flag-off Go path: one nested stack 1 -> 2 -> 3.
	if len(rs.Stacks) != 1 {
		t.Fatalf("want 1 stack from Go path, got %d", len(rs.Stacks))
	}
	root := rs.Stacks[0].Root
	if root.PR.Number != 1 || len(root.Children) != 1 || root.Children[0].PR.Number != 2 {
		t.Fatalf("unexpected Go-path root/child: %+v", root)
	}
	if root.Children[0].Children[0].PR.Number != 3 {
		t.Fatalf("expected #3 nested under #2: %+v", root.Children[0])
	}
}

func TestService_RepoStacks_BuildsNestedStack(t *testing.T) {
	svc, _, uid := seed(t)
	rs, err := svc.RepoStacks(context.Background(), uid, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Stacks) != 1 {
		t.Fatalf("want 1 stack, got %d", len(rs.Stacks))
	}
	root := rs.Stacks[0].Root
	if root.PR.Number != 1 || len(root.Children) != 1 || root.Children[0].PR.Number != 2 {
		t.Fatalf("unexpected root/child: %+v", root)
	}
	if root.Children[0].Children[0].PR.Number != 3 {
		t.Fatalf("expected #3 nested under #2: %+v", root.Children[0])
	}
}

func TestService_RepoStacks_ScopedToUser(t *testing.T) {
	svc, _, uid := seed(t)
	// "x/hidden" exists in the store but is not in the user's visible set.
	if _, err := svc.RepoStacks(context.Background(), uid, "x", "hidden"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound for non-visible repo, got %v", err)
	}
}

func TestService_ListRepos(t *testing.T) {
	svc, _, uid := seed(t)
	repos, err := svc.ListRepos(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "r" {
		t.Fatalf("unexpected repos: %+v", repos)
	}
}
