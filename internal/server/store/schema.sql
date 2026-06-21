-- mp server schema. Applied idempotently at boot by PgxStore.Migrate.
-- Baseline only: when schema evolution needs column drops / data backfills,
-- adopt a migration tool (e.g. goose) with this file as migration 0001.

CREATE TABLE IF NOT EXISTS users (
    id               BIGSERIAL PRIMARY KEY,
    github_user_id   BIGINT      NOT NULL UNIQUE,
    github_login     TEXT        NOT NULL,
    avatar_url       TEXT        NOT NULL DEFAULT '',
    access_token_enc BYTEA       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS repos (
    id              BIGSERIAL PRIMARY KEY,
    github_repo_id  BIGINT      NOT NULL UNIQUE,
    owner           TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    default_branch  TEXT        NOT NULL DEFAULT 'main',
    private         BOOLEAN     NOT NULL DEFAULT false,
    html_url        TEXT        NOT NULL DEFAULT '',
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner, name)
);

CREATE TABLE IF NOT EXISTS user_repos (
    user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id  BIGINT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, repo_id)
);

CREATE TABLE IF NOT EXISTS pull_requests (
    id           BIGSERIAL PRIMARY KEY,
    repo_id      BIGINT      NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    number       INTEGER     NOT NULL,
    title        TEXT        NOT NULL DEFAULT '',
    state        TEXT        NOT NULL,
    is_draft     BOOLEAN     NOT NULL DEFAULT false,
    head_ref     TEXT        NOT NULL,
    base_ref     TEXT        NOT NULL,
    author_login TEXT        NOT NULL DEFAULT '',
    html_url     TEXT        NOT NULL DEFAULT '',
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, number)
);

CREATE TABLE IF NOT EXISTS sync_state (
    user_id     BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'idle',
    last_run_id TEXT        NOT NULL DEFAULT '',
    error       TEXT        NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_repos_user          ON user_repos (user_id);
CREATE INDEX IF NOT EXISTS idx_pull_requests_repo       ON pull_requests (repo_id);
CREATE INDEX IF NOT EXISTS idx_pull_requests_repo_state ON pull_requests (repo_id, state);
