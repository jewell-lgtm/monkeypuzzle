package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
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
	return New(mem, &fakeTrigger{}), mem, uid
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
