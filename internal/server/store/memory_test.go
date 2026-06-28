package store

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryStore_UserUpsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	id, err := s.UpsertUser(ctx, User{ExternalUserID: "wos_1", Provider: "github", ForgeUserID: 42, ForgeLogin: "octo", AccessTokenEnc: []byte("enc1")})
	if err != nil {
		t.Fatal(err)
	}
	// Upsert same (provider, forge_user_id) updates in place (same id, new fields).
	id2, err := s.UpsertUser(ctx, User{ExternalUserID: "wos_1", Provider: "github", ForgeUserID: 42, ForgeLogin: "octocat", AccessTokenEnc: []byte("enc2")})
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Fatalf("upsert created a new id: %d != %d", id, id2)
	}
	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if u.ForgeLogin != "octocat" || string(u.AccessTokenEnc) != "enc2" {
		t.Fatalf("update not applied: %+v", u)
	}

	// Resolve by WorkOS id (the MCP verifier path).
	byWO, err := s.GetUserByExternalID(ctx, "wos_1")
	if err != nil || byWO.ID != id {
		t.Fatalf("GetUserByExternalID: %+v err=%v", byWO, err)
	}
	if _, err := s.GetUserByExternalID(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for unknown workos id, got %v", err)
	}

	if _, err := s.GetUserByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Ping(t *testing.T) {
	if err := NewMemoryStore().Ping(context.Background()); err != nil {
		t.Fatalf("memory store Ping should always succeed, got %v", err)
	}
}

func TestMemoryStore_ReposAndVisibility(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	uid, _ := s.UpsertUser(ctx, User{Provider: "github", ForgeUserID: 1, ForgeLogin: "u"})

	r1, _ := s.UpsertRepo(ctx, Repo{Provider: "github", ForgeRepoID: 10, Owner: "o", Name: "beta", DefaultBranch: "main"})
	r2, _ := s.UpsertRepo(ctx, Repo{Provider: "github", ForgeRepoID: 11, Owner: "o", Name: "alpha", DefaultBranch: "main"})

	if err := s.SetUserRepos(ctx, uid, []int64{r1, r2}); err != nil {
		t.Fatal(err)
	}
	repos, err := s.ListReposForUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	// ordered by owner, name -> alpha before beta
	if len(repos) != 2 || repos[0].Name != "alpha" || repos[1].Name != "beta" {
		t.Fatalf("unexpected repos: %+v", repos)
	}

	got, err := s.GetRepoByProviderOwnerName(ctx, "github", "o", "alpha")
	if err != nil || got.ForgeRepoID != 11 {
		t.Fatalf("get repo: %+v err=%v", got, err)
	}
	if _, err := s.GetRepoByProviderOwnerName(ctx, "github", "o", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	// Re-setting visibility replaces the set.
	if err := s.SetUserRepos(ctx, uid, []int64{r2}); err != nil {
		t.Fatal(err)
	}
	repos, _ = s.ListReposForUser(ctx, uid)
	if len(repos) != 1 || repos[0].ID != r2 {
		t.Fatalf("visibility not replaced: %+v", repos)
	}
}

// Provider scoping: a GitHub id and a GitLab id that happen to share the same
// numeric value are distinct rows, not an upsert collision.
func TestMemoryStore_ProviderScopesForgeIDs(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	gh, _ := s.UpsertUser(ctx, User{ExternalUserID: "ext-gh", Provider: "github", ForgeUserID: 1, ForgeLogin: "gh"})
	gl, _ := s.UpsertUser(ctx, User{ExternalUserID: "ext-gl", Provider: "gitlab", ForgeUserID: 1, ForgeLogin: "gl"})
	if gh == gl {
		t.Fatalf("github and gitlab users with the same forge id collided: %d == %d", gh, gl)
	}

	r1, _ := s.UpsertRepo(ctx, Repo{Provider: "github", ForgeRepoID: 5, Owner: "o", Name: "r", DefaultBranch: "main"})
	r2, _ := s.UpsertRepo(ctx, Repo{Provider: "gitlab", ForgeRepoID: 5, Owner: "o", Name: "r", DefaultBranch: "main"})
	if r1 == r2 {
		t.Fatalf("github and gitlab repos with the same forge id collided: %d == %d", r1, r2)
	}
}

func TestMemoryStore_ReplaceAndListPullRequests(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	rid, _ := s.UpsertRepo(ctx, Repo{Provider: "github", ForgeRepoID: 10, Owner: "o", Name: "r", DefaultBranch: "main"})

	prs := []PullRequest{
		{Number: 3, HeadRef: "c", BaseRef: "b", State: "OPEN"},
		{Number: 1, HeadRef: "a", BaseRef: "main", State: "OPEN"},
	}
	if err := s.ReplacePullRequests(ctx, rid, prs); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListPullRequestsForRepo(ctx, rid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 3 {
		t.Fatalf("PRs not ordered by number: %+v", got)
	}
	if got[0].RepoID != rid {
		t.Fatalf("RepoID not set: %+v", got[0])
	}

	// Replace fully swaps the set.
	if err := s.ReplacePullRequests(ctx, rid, []PullRequest{{Number: 9, HeadRef: "x", BaseRef: "main", State: "OPEN"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListPullRequestsForRepo(ctx, rid)
	if len(got) != 1 || got[0].Number != 9 {
		t.Fatalf("PRs not replaced: %+v", got)
	}
}

func TestMemoryStore_SyncStatus(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	uid, _ := s.UpsertUser(ctx, User{Provider: "github", ForgeUserID: 1, ForgeLogin: "u"})

	// Default is idle when never set.
	st, err := s.GetSyncStatus(ctx, uid)
	if err != nil || st.Status != SyncIdle {
		t.Fatalf("default status: %+v err=%v", st, err)
	}
	if err := s.SetSyncStatus(ctx, uid, SyncStatus{Status: SyncRunning, LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.GetSyncStatus(ctx, uid)
	if st.Status != SyncRunning || st.LastRunID != "run-1" {
		t.Fatalf("status not persisted: %+v", st)
	}
}
