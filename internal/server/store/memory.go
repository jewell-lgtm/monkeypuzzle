package store

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for hermetic tests (no Postgres). It mirrors
// adapters.MemoryFS: a concurrency-safe map-backed fake with the same contract
// as the real implementation.
type MemoryStore struct {
	mu        sync.RWMutex
	nextUser  int64
	nextRepo  int64
	nextPR    int64
	users     map[int64]User           // id -> user
	usersByGH map[int64]int64          // github_user_id -> id
	repos     map[int64]Repo           // id -> repo
	reposByGH map[int64]int64          // github_repo_id -> id
	userRepos map[int64]map[int64]bool // user id -> set of repo ids
	prs       map[int64][]PullRequest  // repo id -> PRs
	sync      map[int64]SyncStatus     // user id -> status
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:     map[int64]User{},
		usersByGH: map[int64]int64{},
		repos:     map[int64]Repo{},
		reposByGH: map[int64]int64{},
		userRepos: map[int64]map[int64]bool{},
		prs:       map[int64][]PullRequest{},
		sync:      map[int64]SyncStatus{},
	}
}

// Migrate is a no-op for the in-memory store.
func (m *MemoryStore) Migrate(context.Context) error { return nil }

func (m *MemoryStore) UpsertUser(_ context.Context, u User) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.usersByGH[u.GitHubUserID]; ok {
		u.ID = id
		m.users[id] = u
		return id, nil
	}
	m.nextUser++
	u.ID = m.nextUser
	m.users[u.ID] = u
	m.usersByGH[u.GitHubUserID] = u.ID
	return u.ID, nil
}

func (m *MemoryStore) GetUserByID(_ context.Context, id int64) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (m *MemoryStore) UpsertRepo(_ context.Context, r Repo) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.reposByGH[r.GitHubRepoID]; ok {
		r.ID = id
		m.repos[id] = r
		return id, nil
	}
	m.nextRepo++
	r.ID = m.nextRepo
	m.repos[r.ID] = r
	m.reposByGH[r.GitHubRepoID] = r.ID
	return r.ID, nil
}

func (m *MemoryStore) SetUserRepos(_ context.Context, userID int64, repoIDs []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := make(map[int64]bool, len(repoIDs))
	for _, id := range repoIDs {
		set[id] = true
	}
	m.userRepos[userID] = set
	return nil
}

func (m *MemoryStore) ListReposForUser(_ context.Context, userID int64) ([]Repo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Repo
	for id := range m.userRepos[userID] {
		if r, ok := m.repos[id]; ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Owner != out[j].Owner {
			return out[i].Owner < out[j].Owner
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *MemoryStore) GetRepoByOwnerName(_ context.Context, owner, name string) (Repo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.repos {
		if r.Owner == owner && r.Name == name {
			return r, nil
		}
	}
	return Repo{}, ErrNotFound
}

func (m *MemoryStore) ReplacePullRequests(_ context.Context, repoID int64, prs []PullRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := make([]PullRequest, len(prs))
	for i, pr := range prs {
		m.nextPR++
		pr.ID = m.nextPR
		pr.RepoID = repoID
		stored[i] = pr
	}
	sort.Slice(stored, func(i, j int) bool { return stored[i].Number < stored[j].Number })
	m.prs[repoID] = stored
	return nil
}

func (m *MemoryStore) ListPullRequestsForRepo(_ context.Context, repoID int64) ([]PullRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.prs[repoID]
	out := make([]PullRequest, len(src))
	copy(out, src)
	return out, nil
}

func (m *MemoryStore) SetSyncStatus(_ context.Context, userID int64, s SyncStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sync[userID] = s
	return nil
}

func (m *MemoryStore) GetSyncStatus(_ context.Context, userID int64) (SyncStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sync[userID]
	if !ok {
		return SyncStatus{Status: SyncIdle}, nil
	}
	return s, nil
}
