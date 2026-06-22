package sync

import (
	"context"
	"testing"

	"go.temporal.io/sdk/testsuite"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

func key32() []byte {
	k := make([]byte, crypto.KeySize)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

// The sync workflow end-to-end against the in-memory store + stub GitHub +
// real cipher, executed in the Temporal test environment (no real Temporal).
func TestSyncUserDataWorkflow_HappyPath(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	cipher, err := crypto.NewAESGCMCipher(key32())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt([]byte("tok-123"))
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := mem.UpsertUser(ctx, store.User{GitHubUserID: 4242, GitHubLogin: "octo", AccessTokenEnc: enc})

	stub := forge.NewStubFactory()
	stub.SetUser("tok-123", forge.User{ID: 4242, Login: "octo"})
	stub.SetRepos("tok-123", forge.Repo{ForgeRepoID: 7, Owner: "o", Name: "r", DefaultBranch: "main"})
	stub.SetPullRequests("tok-123", "o", "r",
		stackgraph.PRRef{Number: 1, HeadRef: "feat-a", BaseRef: "main", State: stackgraph.StateOpen},
		stackgraph.PRRef{Number: 2, HeadRef: "feat-b", BaseRef: "feat-a", State: stackgraph.StateOpen},
		stackgraph.PRRef{Number: 3, HeadRef: "feat-c", BaseRef: "feat-b", State: stackgraph.StateOpen},
	)

	a := &Activities{Store: mem, Factory: stub, Cipher: cipher}

	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivity(a)
	env.ExecuteWorkflow(SyncUserDataWorkflow, SyncInput{UserID: uid})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	repos, _ := mem.ListReposForUser(ctx, uid)
	if len(repos) != 1 || repos[0].Name != "r" {
		t.Fatalf("repos not persisted: %+v", repos)
	}
	prs, _ := mem.ListPullRequestsForRepo(ctx, repos[0].ID)
	if len(prs) != 3 {
		t.Fatalf("want 3 PRs persisted, got %d: %+v", len(prs), prs)
	}
	if st, _ := mem.GetSyncStatus(ctx, uid); st.Status != store.SyncDone {
		t.Fatalf("want sync status done, got %+v", st)
	}
}
