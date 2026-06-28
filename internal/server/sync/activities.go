package sync

import (
	"context"
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// Activities holds the dependencies the sync activities need. It is registered
// on the worker; the workflow references its methods by value.
type Activities struct {
	Store  store.Store
	Forge  forge.Registry
	Cipher crypto.TokenCipher
}

// clientFor loads the user, decrypts their forge token, and returns a
// token-scoped forge client for the user's provider plus that provider. Each
// forge-calling activity re-derives the token from the store so the plaintext
// token never enters workflow history.
func (a *Activities) clientFor(ctx context.Context, userID int64) (forge.Client, string, error) {
	u, err := a.Store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("sync: load user %d: %w", userID, err)
	}
	token, err := a.Cipher.Decrypt(u.AccessTokenEnc)
	if err != nil {
		return nil, "", fmt.Errorf("sync: decrypt token for user %d: %w", userID, err)
	}
	client, err := a.Forge.ForToken(u.Provider, string(token))
	if err != nil {
		return nil, "", fmt.Errorf("sync: forge client for user %d: %w", userID, err)
	}
	return client, u.Provider, nil
}

// FetchRepos lists every repo the user can access from the forge.
func (a *Activities) FetchRepos(ctx context.Context, userID int64) ([]repoDTO, error) {
	client, provider, err := a.clientFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	repos, err := client.ListAccessibleRepos(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]repoDTO, len(repos))
	for i, r := range repos {
		out[i] = repoDTO{
			Provider:      provider,
			ForgeRepoID:   r.ForgeRepoID,
			Owner:         r.Owner,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			HTMLURL:       r.HTMLURL,
		}
	}
	return out, nil
}

// PersistRepos upserts the repos and replaces the user's visibility set.
func (a *Activities) PersistRepos(ctx context.Context, in PersistReposInput) error {
	ids := make([]int64, 0, len(in.Repos))
	for _, r := range in.Repos {
		id, err := a.Store.UpsertRepo(ctx, store.Repo{
			Provider:      r.Provider,
			ForgeRepoID:   r.ForgeRepoID,
			Owner:         r.Owner,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			HTMLURL:       r.HTMLURL,
		})
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	return a.Store.SetUserRepos(ctx, in.UserID, ids)
}

// FetchPullRequests lists all PRs/MRs (state=all) for a repo from the forge.
func (a *Activities) FetchPullRequests(ctx context.Context, in FetchPRInput) ([]stackgraph.PRRef, error) {
	client, _, err := a.clientFor(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	return client.ListPullRequests(ctx, in.Owner, in.Name)
}

// PersistPullRequests replaces a repo's PR set in the store.
func (a *Activities) PersistPullRequests(ctx context.Context, in PersistPRInput) error {
	repo, err := a.Store.GetRepoByProviderOwnerName(ctx, in.Provider, in.Owner, in.Name)
	if err != nil {
		return fmt.Errorf("sync: resolve repo %s/%s/%s: %w", in.Provider, in.Owner, in.Name, err)
	}
	return a.Store.ReplacePullRequests(ctx, repo.ID, toStorePRs(in.PRs))
}

// MarkSync writes the terminal sync_state for a user.
func (a *Activities) MarkSync(ctx context.Context, in MarkSyncInput) error {
	return a.Store.SetSyncStatus(ctx, in.UserID, store.SyncStatus{
		Status: in.Status,
		Error:  in.Error,
	})
}
