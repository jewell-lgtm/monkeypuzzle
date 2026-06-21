package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// PgxStore is the Postgres-backed Store, built on a pgx connection pool.
type PgxStore struct {
	pool *pgxpool.Pool
}

// NewPgxStore connects to Postgres at dsn and returns a Store. Call Close when done.
func NewPgxStore(ctx context.Context, dsn string) (*PgxStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	return &PgxStore{pool: pool}, nil
}

// Close releases the connection pool.
func (s *PgxStore) Close() { s.pool.Close() }

// Migrate applies schema.sql idempotently, one statement at a time (the
// extended protocol pgx uses forbids multiple statements per Exec).
func (s *PgxStore) Migrate(ctx context.Context) error {
	for _, stmt := range splitStatements(schemaSQL) {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("store: migrate: %w", err)
		}
	}
	return nil
}

// splitStatements splits a SQL file into individual statements on ';',
// dropping chunks that are blank or comment-only.
func splitStatements(sql string) []string {
	var out []string
	for _, chunk := range strings.Split(sql, ";") {
		hasSQL := false
		for _, line := range strings.Split(chunk, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") {
				hasSQL = true
				break
			}
		}
		if hasSQL {
			out = append(out, strings.TrimSpace(chunk))
		}
	}
	return out
}

func (s *PgxStore) UpsertUser(ctx context.Context, u User) (int64, error) {
	const q = `
INSERT INTO users (github_user_id, github_login, avatar_url, access_token_enc, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (github_user_id) DO UPDATE SET
  github_login = EXCLUDED.github_login,
  avatar_url = EXCLUDED.avatar_url,
  access_token_enc = EXCLUDED.access_token_enc,
  updated_at = now()
RETURNING id`
	var id int64
	err := s.pool.QueryRow(ctx, q, u.GitHubUserID, u.GitHubLogin, u.AvatarURL, u.AccessTokenEnc).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert user: %w", err)
	}
	return id, nil
}

func (s *PgxStore) GetUserByID(ctx context.Context, id int64) (User, error) {
	const q = `SELECT id, github_user_id, github_login, avatar_url, access_token_enc FROM users WHERE id = $1`
	var u User
	err := s.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.GitHubUserID, &u.GitHubLogin, &u.AvatarURL, &u.AccessTokenEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("store: get user: %w", err)
	}
	return u, nil
}

func (s *PgxStore) UpsertRepo(ctx context.Context, r Repo) (int64, error) {
	const q = `
INSERT INTO repos (github_repo_id, owner, name, default_branch, private, html_url, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (github_repo_id) DO UPDATE SET
  owner = EXCLUDED.owner,
  name = EXCLUDED.name,
  default_branch = EXCLUDED.default_branch,
  private = EXCLUDED.private,
  html_url = EXCLUDED.html_url,
  synced_at = now()
RETURNING id`
	var id int64
	err := s.pool.QueryRow(ctx, q, r.GitHubRepoID, r.Owner, r.Name, r.DefaultBranch, r.Private, r.HTMLURL).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert repo: %w", err)
	}
	return id, nil
}

func (s *PgxStore) SetUserRepos(ctx context.Context, userID int64, repoIDs []int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: set user repos: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
	if _, err := tx.Exec(ctx, `DELETE FROM user_repos WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("store: set user repos: delete: %w", err)
	}
	for _, repoID := range repoIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_repos (user_id, repo_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			userID, repoID); err != nil {
			return fmt.Errorf("store: set user repos: insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *PgxStore) ListReposForUser(ctx context.Context, userID int64) ([]Repo, error) {
	const q = `
SELECT r.id, r.github_repo_id, r.owner, r.name, r.default_branch, r.private, r.html_url
FROM repos r JOIN user_repos ur ON ur.repo_id = r.id
WHERE ur.user_id = $1
ORDER BY r.owner, r.name`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list repos: %w", err)
	}
	defer rows.Close()
	var out []Repo
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.GitHubRepoID, &r.Owner, &r.Name, &r.DefaultBranch, &r.Private, &r.HTMLURL); err != nil {
			return nil, fmt.Errorf("store: list repos: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PgxStore) GetRepoByOwnerName(ctx context.Context, owner, name string) (Repo, error) {
	const q = `SELECT id, github_repo_id, owner, name, default_branch, private, html_url FROM repos WHERE owner = $1 AND name = $2`
	var r Repo
	err := s.pool.QueryRow(ctx, q, owner, name).Scan(&r.ID, &r.GitHubRepoID, &r.Owner, &r.Name, &r.DefaultBranch, &r.Private, &r.HTMLURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return Repo{}, ErrNotFound
	}
	if err != nil {
		return Repo{}, fmt.Errorf("store: get repo: %w", err)
	}
	return r, nil
}

func (s *PgxStore) ReplacePullRequests(ctx context.Context, repoID int64, prs []PullRequest) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: replace PRs: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
	if _, err := tx.Exec(ctx, `DELETE FROM pull_requests WHERE repo_id = $1`, repoID); err != nil {
		return fmt.Errorf("store: replace PRs: delete: %w", err)
	}
	const ins = `
INSERT INTO pull_requests (repo_id, number, title, state, is_draft, head_ref, base_ref, author_login, html_url, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`
	for _, pr := range prs {
		if _, err := tx.Exec(ctx, ins,
			repoID, pr.Number, pr.Title, pr.State, pr.IsDraft, pr.HeadRef, pr.BaseRef, pr.Author, pr.HTMLURL); err != nil {
			return fmt.Errorf("store: replace PRs: insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *PgxStore) ListPullRequestsForRepo(ctx context.Context, repoID int64) ([]PullRequest, error) {
	const q = `
SELECT id, repo_id, number, title, state, is_draft, head_ref, base_ref, author_login, html_url
FROM pull_requests WHERE repo_id = $1 ORDER BY number`
	rows, err := s.pool.Query(ctx, q, repoID)
	if err != nil {
		return nil, fmt.Errorf("store: list PRs: %w", err)
	}
	defer rows.Close()
	var out []PullRequest
	for rows.Next() {
		var pr PullRequest
		if err := rows.Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.Title, &pr.State, &pr.IsDraft,
			&pr.HeadRef, &pr.BaseRef, &pr.Author, &pr.HTMLURL); err != nil {
			return nil, fmt.Errorf("store: list PRs: scan: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

func (s *PgxStore) SetSyncStatus(ctx context.Context, userID int64, st SyncStatus) error {
	const q = `
INSERT INTO sync_state (user_id, status, last_run_id, error, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (user_id) DO UPDATE SET
  status = EXCLUDED.status,
  last_run_id = EXCLUDED.last_run_id,
  error = EXCLUDED.error,
  updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, userID, st.Status, st.LastRunID, st.Error); err != nil {
		return fmt.Errorf("store: set sync status: %w", err)
	}
	return nil
}

func (s *PgxStore) GetSyncStatus(ctx context.Context, userID int64) (SyncStatus, error) {
	const q = `SELECT status, last_run_id, error FROM sync_state WHERE user_id = $1`
	var st SyncStatus
	err := s.pool.QueryRow(ctx, q, userID).Scan(&st.Status, &st.LastRunID, &st.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncStatus{Status: SyncIdle}, nil
	}
	if err != nil {
		return SyncStatus{}, fmt.Errorf("store: get sync status: %w", err)
	}
	return st, nil
}

// compile-time assertions that both implementations satisfy Store.
var (
	_ Store = (*PgxStore)(nil)
	_ Store = (*MemoryStore)(nil)
)
