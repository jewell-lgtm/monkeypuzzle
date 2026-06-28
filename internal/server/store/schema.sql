-- mp server schema. Applied idempotently at boot by PgxStore.Migrate.
-- Baseline only: when schema evolution needs column drops / data backfills,
-- adopt a migration tool (e.g. goose) with this file as migration 0001.

-- The github_user_id / github_repo_id columns hold the forge-native numeric id
-- (GitHub user/repo id or GitLab user/project id); the provider column scopes
-- them so the same id space across forges cannot collide.
CREATE TABLE IF NOT EXISTS users (
    id               BIGSERIAL PRIMARY KEY,
    external_user_id   TEXT        NOT NULL UNIQUE,
    provider         TEXT        NOT NULL DEFAULT 'github',
    github_user_id   BIGINT      NOT NULL,
    github_login     TEXT        NOT NULL,
    avatar_url       TEXT        NOT NULL DEFAULT '',
    access_token_enc BYTEA       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS repos (
    id              BIGSERIAL PRIMARY KEY,
    provider        TEXT        NOT NULL DEFAULT 'github',
    github_repo_id  BIGINT      NOT NULL,
    owner           TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    default_branch  TEXT        NOT NULL DEFAULT 'main',
    private         BOOLEAN     NOT NULL DEFAULT false,
    html_url        TEXT        NOT NULL DEFAULT '',
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT now()
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

-- Provider migration for databases created before the provider column existed.
-- Additive + idempotent: ADD COLUMN IF NOT EXISTS, backfill, then drop the old
-- single-column UNIQUE constraints and replace them with provider-scoped unique
-- indexes so the GitHub and GitLab id spaces cannot collide.
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'github';
ALTER TABLE repos ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'github';
UPDATE users SET provider = 'github' WHERE provider IS NULL OR provider = '';
UPDATE repos SET provider = 'github' WHERE provider IS NULL OR provider = '';
-- Postgres auto-names a column UNIQUE constraint <table>_<column>_key and a
-- table-level UNIQUE(a, b) <table>_a_b_key. TODO verify these names against a
-- live database; a wrong name silently no-ops and leaves the old single-column
-- UNIQUE, which would block a colliding GitLab id.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_github_user_id_key;
ALTER TABLE repos DROP CONSTRAINT IF EXISTS repos_github_repo_id_key;
ALTER TABLE repos DROP CONSTRAINT IF EXISTS repos_owner_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS users_provider_forge_uid  ON users (provider, github_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS repos_provider_forge_rid  ON repos (provider, github_repo_id);
CREATE UNIQUE INDEX IF NOT EXISTS repos_provider_owner_name ON repos (provider, owner, name);
