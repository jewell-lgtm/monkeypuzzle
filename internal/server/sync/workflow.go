package sync

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// act is a nil typed pointer used only to name activities inside the workflow;
// the worker registers the real *Activities instance.
var act *Activities

// SyncUserDataWorkflow fetches a user's repos and PRs from GitHub and persists
// them, then records a terminal sync status. v1 is sequential for determinism;
// per-repo PR fetches can later fan out.
func SyncUserDataWorkflow(ctx workflow.Context, in SyncInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})

	err := syncUser(ctx, in)

	status, errMsg := store.SyncDone, ""
	if err != nil {
		status, errMsg = store.SyncFailed, err.Error()
	}
	// Best-effort terminal status; never mask the original error.
	_ = workflow.ExecuteActivity(ctx, act.MarkSync,
		MarkSyncInput{UserID: in.UserID, Status: status, Error: errMsg}).Get(ctx, nil)
	return err
}

func syncUser(ctx workflow.Context, in SyncInput) error {
	var repos []repoDTO
	if err := workflow.ExecuteActivity(ctx, act.FetchRepos, in.UserID).Get(ctx, &repos); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, act.PersistRepos,
		PersistReposInput{UserID: in.UserID, Repos: repos}).Get(ctx, nil); err != nil {
		return err
	}
	for _, r := range repos {
		var prs []stackgraph.PRRef
		if err := workflow.ExecuteActivity(ctx, act.FetchPullRequests,
			FetchPRInput{UserID: in.UserID, Owner: r.Owner, Name: r.Name}).Get(ctx, &prs); err != nil {
			return err
		}
		if err := workflow.ExecuteActivity(ctx, act.PersistPullRequests,
			PersistPRInput{Owner: r.Owner, Name: r.Name, PRs: prs}).Get(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}
