// Package store is the persistence layer for mp server. It is the cache layer
// between the app and GitHub: Temporal workers fetch repos/PRs from the GitHub
// API and persist them here, and every read path (HTML UI, MCP tools) reads
// only from the store — never from GitHub directly.
//
// Following the repo's ports/adapters convention, Store is an interface with a
// real Postgres implementation (PgxStore) and an in-memory implementation
// (MemoryStore) for hermetic tests, mirroring adapters.OSFS / adapters.MemoryFS.
package store

import (
	"context"
	"errors"
)

// ErrNotFound is returned by the Get* methods when no row matches.
var ErrNotFound = errors.New("store: not found")

// Sync status values stored in sync_state.status.
const (
	SyncIdle    = "idle"
	SyncRunning = "running"
	SyncDone    = "done"
	SyncFailed  = "failed"
)

// User is an authenticated account. Identity comes from WorkOS (ExternalUserID is
// the WorkOS `sub`, shared by the human session and the agent's MCP token); the
// GitHub fields are derived from the GitHub token WorkOS passes through.
// AccessTokenEnc holds that GitHub token encrypted at rest; the store treats it
// as opaque bytes (callers encrypt/decrypt) and it is only decrypted in the worker.
type User struct {
	ID             int64
	ExternalUserID string
	GitHubUserID   int64
	GitHubLogin    string
	AvatarURL      string
	AccessTokenEnc []byte
}

// Repo is a GitHub repository the user can access.
type Repo struct {
	ID            int64
	GitHubRepoID  int64
	Owner         string
	Name          string
	DefaultBranch string
	Private       bool
	HTMLURL       string
}

// PullRequest is one PR of a repo. State is OPEN | MERGED | CLOSED, mirroring
// stackgraph.PRRef / adapters.PRInfo semantics.
type PullRequest struct {
	ID      int64
	RepoID  int64
	Number  int
	Title   string
	State   string
	IsDraft bool
	HeadRef string
	BaseRef string
	Author  string
	HTMLURL string
}

// SyncStatus is the per-user bookkeeping for the "Sync now" button and its poll.
type SyncStatus struct {
	Status    string
	LastRunID string
	Error     string
}

// Store is the read/write persistence contract. All methods take a context and
// must be safe for concurrent use across different users; per-repo write
// serialization (when needed) is the caller's responsibility.
type Store interface {
	// Migrate applies the schema idempotently. Safe to call on every boot.
	Migrate(ctx context.Context) error
	// Ping verifies the backing store is reachable; used by readiness checks.
	Ping(ctx context.Context) error

	// UpsertUser inserts or updates a user keyed by GitHubUserID and returns its
	// id. AccessTokenEnc is persisted as-is (already encrypted).
	UpsertUser(ctx context.Context, u User) (int64, error)
	// GetUserByID returns the user (including AccessTokenEnc) or ErrNotFound.
	GetUserByID(ctx context.Context, id int64) (User, error)
	// GetUserByExternalID resolves a WorkOS `sub` to a user, or ErrNotFound. Used
	// by the MCP resource server to map a validated token to a local user.
	GetUserByExternalID(ctx context.Context, externalUserID string) (User, error)

	// UpsertRepo inserts or updates a repo keyed by GitHubRepoID and returns its id.
	UpsertRepo(ctx context.Context, r Repo) (int64, error)
	// SetUserRepos replaces the set of repos visible to a user.
	SetUserRepos(ctx context.Context, userID int64, repoIDs []int64) error
	// ListReposForUser returns the user's repos ordered by owner/name.
	ListReposForUser(ctx context.Context, userID int64) ([]Repo, error)
	// GetRepoByOwnerName returns a repo or ErrNotFound.
	GetRepoByOwnerName(ctx context.Context, owner, name string) (Repo, error)

	// ReplacePullRequests atomically replaces all PRs of a repo with prs.
	ReplacePullRequests(ctx context.Context, repoID int64, prs []PullRequest) error
	// ListPullRequestsForRepo returns a repo's PRs ordered by number.
	ListPullRequestsForRepo(ctx context.Context, repoID int64) ([]PullRequest, error)

	// SetSyncStatus upserts the per-user sync bookkeeping row.
	SetSyncStatus(ctx context.Context, userID int64, s SyncStatus) error
	// GetSyncStatus returns the user's sync status, or {Status: SyncIdle} if none.
	GetSyncStatus(ctx context.Context, userID int64) (SyncStatus, error)
}
