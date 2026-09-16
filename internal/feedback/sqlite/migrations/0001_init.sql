-- 0001_init: the MVP feedback schema, verbatim from
-- .task/model-feedback-plan/plan.md section 5.1 plus one addition (the
-- final index below) documented at its own definition. (contract.md's own
-- section 9 explicitly places the SQLite schema out of its scope and
-- defers to plan.md -- plan.md is the correct citation here, not
-- contract.md.) schema_migrations itself is bootstrapped by the migration
-- runner (migrate.go), not by this file, since it must exist before this
-- file's own checksum can even be looked up.

CREATE TABLE identities (
    id BLOB PRIMARY KEY,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE TABLE model_feedback (
    identity_id BLOB NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    model_key TEXT NOT NULL,
    overall INTEGER NOT NULL CHECK (overall BETWEEN 1 AND 5),
    review TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (identity_id, model_key)
);

CREATE TABLE skill_ratings (
    identity_id BLOB NOT NULL,
    model_key TEXT NOT NULL,
    skill_key TEXT NOT NULL,
    rating INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    PRIMARY KEY (identity_id, model_key, skill_key),
    FOREIGN KEY (identity_id, model_key) REFERENCES model_feedback(identity_id, model_key) ON DELETE CASCADE
);

CREATE TABLE privacy_cleanup_jobs (
    id TEXT PRIMARY KEY,
    state TEXT NOT NULL CHECK (state IN ('cleanup_pending', 'done', 'failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Enforces plan 5.2's "не более одной активной job" at the SQL level, in
-- addition to the application-level check DeleteIdentity/UpsertFeedback
-- already do inside the same transaction: a partial unique index over the
-- constant expression (1), scoped to every row whose state is not 'done',
-- means at most one such row can ever exist -- 'cleanup_pending' and
-- 'failed' both count as "active" (a failed post-commit cleanup still needs
-- a retry to reach 'done'), while any number of historical 'done' rows are
-- unrestricted. Verified directly against sqlite3 3.43.2 before being
-- committed here: a second insert with state IN ('cleanup_pending',
-- 'failed') while one such row already exists fails with "UNIQUE constraint
-- failed: index 'idx_privacy_cleanup_jobs_single_active'"; multiple 'done'
-- rows insert without conflict.
CREATE UNIQUE INDEX idx_privacy_cleanup_jobs_single_active
    ON privacy_cleanup_jobs ((1))
    WHERE state <> 'done';
