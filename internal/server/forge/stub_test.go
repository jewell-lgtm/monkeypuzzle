package forge

import (
	"context"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

func TestStubFactory(t *testing.T) {
	ctx := context.Background()
	f := NewStubFactory()
	f.SetUser("tok", User{ID: 7, Login: "octo"})
	f.SetRepos("tok", Repo{ForgeRepoID: 1, Owner: "o", Name: "r", DefaultBranch: "main"})
	f.SetPullRequests("tok", "o", "r",
		stackgraph.PRRef{Number: 1, HeadRef: "a", BaseRef: "main", State: stackgraph.StateOpen})

	c := f.ForToken("tok")

	u, err := c.GetAuthenticatedUser(ctx)
	if err != nil || u.Login != "octo" {
		t.Fatalf("user: %+v err=%v", u, err)
	}
	repos, err := c.ListAccessibleRepos(ctx)
	if err != nil || len(repos) != 1 || repos[0].Name != "r" {
		t.Fatalf("repos: %+v err=%v", repos, err)
	}
	prs, err := c.ListPullRequests(ctx, "o", "r")
	if err != nil || len(prs) != 1 || prs[0].Number != 1 {
		t.Fatalf("prs: %+v err=%v", prs, err)
	}

	// Unknown token errors on user lookup.
	if _, err := f.ForToken("nope").GetAuthenticatedUser(ctx); err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestRegistry_ForToken(t *testing.T) {
	stub := NewStubFactory()
	stub.SetUser("tok", User{ID: 1, Login: "u"})
	reg := Registry{"github": stub}

	c, err := reg.ForToken("github", "tok")
	if err != nil {
		t.Fatalf("ForToken: %v", err)
	}
	if u, err := c.GetAuthenticatedUser(context.Background()); err != nil || u.Login != "u" {
		t.Fatalf("user: %+v err=%v", u, err)
	}

	if _, err := reg.ForToken("gitlab", "tok"); err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}
