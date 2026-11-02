-- Dashboards (D-067, D-068): built once in a chat, then refreshed by
-- CloudBuddy on a timer.

-- One dashboard. spec holds its sources (commands) and tiles as the agent
-- saved them, after CloudBuddy checked them.
CREATE TABLE dashboards (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    -- the chat that built it, or last changed it
    conversation_id  TEXT REFERENCES conversations(id) ON DELETE SET NULL,
    workdir          TEXT NOT NULL,
    folders          TEXT[] NOT NULL DEFAULT '{}',
    refresh_seconds  INT NOT NULL,
    paused           BOOLEAN NOT NULL DEFAULT false,
    spec             JSONB NOT NULL,
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The user's answer for a command that isn't a read: allowed or not, tied to
-- the exact command, its folder and a hash of the files it runs. A changed
-- command or file needs a new answer.
CREATE TABLE dashboard_approvals (
    dashboard_id TEXT NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    source_key   TEXT NOT NULL,
    command      TEXT NOT NULL,
    cwd          TEXT NOT NULL,
    exec_digest  TEXT NOT NULL,
    allowed      BOOLEAN NOT NULL,
    decided_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (dashboard_id, source_key)
);

-- One run of one command: masked, capped output, kept for the latest runs only.
CREATE TABLE dashboard_runs (
    id           BIGSERIAL PRIMARY KEY,
    dashboard_id TEXT NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    source_key   TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    duration_ms  INT NOT NULL DEFAULT 0,
    -- ok | failed | waiting (needs the user's OK) | blocked | declined
    state        TEXT NOT NULL,
    exit_code    INT,
    output       TEXT NOT NULL DEFAULT '',
    truncated    BOOLEAN NOT NULL DEFAULT false,
    note         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX dashboard_runs_latest ON dashboard_runs(dashboard_id, source_key, id DESC);

-- Numbers over time, one series per number or line tile.
CREATE TABLE dashboard_points (
    dashboard_id TEXT NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    series       TEXT NOT NULL,
    ts           TIMESTAMPTZ NOT NULL,
    value        DOUBLE PRECISION NOT NULL
);
CREATE INDEX dashboard_points_series ON dashboard_points(dashboard_id, series, ts);
