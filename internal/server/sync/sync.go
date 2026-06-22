// Package sync keeps the Postgres cache fresh from GitHub via Temporal. The
// SyncUserDataWorkflow fans out activities that fetch a user's repos and PRs
// from the GitHub API and persist them to the store; the web process triggers
// it (on login and on demand) and the worker executes it. This is also the
// durable substrate the future write path (rebase/retarget a stack) will use.
package sync

import (
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// TaskQueue is the Temporal task queue both the trigger (client) and worker use.
const TaskQueue = "mp-sync"

// SyncInput is the workflow argument.
type SyncInput struct {
	UserID int64
}

// PersistReposInput carries fetched repos to the persist activity.
type PersistReposInput struct {
	UserID int64
	Repos  []repoDTO
}

// FetchPRInput identifies a repo whose PRs to fetch (UserID supplies the token).
type FetchPRInput struct {
	UserID int64
	Owner  string
	Name   string
}

// PersistPRInput carries fetched PRs to the persist activity.
type PersistPRInput struct {
	Owner string
	Name  string
	PRs   []stackgraph.PRRef
}

// MarkSyncInput sets the terminal sync_state for a user.
type MarkSyncInput struct {
	UserID int64
	Status string
	Error  string
}

// repoDTO is the serializable repo shape passed through workflow history. It
// mirrors forge.Repo without importing it into the workflow's data plane.
type repoDTO struct {
	GitHubRepoID  int64
	Owner         string
	Name          string
	DefaultBranch string
	Private       bool
	HTMLURL       string
}

// toStorePRs converts stackgraph PRs to store rows for persistence.
func toStorePRs(prs []stackgraph.PRRef) []store.PullRequest {
	out := make([]store.PullRequest, len(prs))
	for i, p := range prs {
		out[i] = store.PullRequest{
			Number:  p.Number,
			Title:   p.Title,
			State:   p.State,
			IsDraft: p.Draft,
			HeadRef: p.HeadRef,
			BaseRef: p.BaseRef,
			Author:  p.Author,
			HTMLURL: p.URL,
		}
	}
	return out
}
