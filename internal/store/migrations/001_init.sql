-- CloudBuddy schema v1 (plan section 5).

CREATE TABLE conversations (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    agent            TEXT NOT NULL,
    model            TEXT NOT NULL DEFAULT '',
    workdir          TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'idle',
    agent_session_id TEXT NOT NULL DEFAULT '',
    subtitle         TEXT NOT NULL DEFAULT '',
    result           JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    launch_epoch    TEXT NOT NULL,
    agent_version   TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'running',
    exit_reason     TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ
);
CREATE INDEX runs_conversation ON runs(conversation_id, started_at);

-- Masked, size-capped, append-only (D-015).
CREATE TABLE events (
    id              BIGSERIAL PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    ts              TIMESTAMPTZ NOT NULL DEFAULT now(),
    source          TEXT NOT NULL,
    kind            TEXT NOT NULL,
    payload         JSONB NOT NULL,
    truncated       BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX events_run ON events(run_id, id);

-- Steps are a view built from the events; the document holds the API shape.
CREATE TABLE steps (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    run_id          TEXT NOT NULL,
    tool_key        TEXT NOT NULL,          -- agent's tool call id
    seq             INT NOT NULL,
    doc             JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, tool_key)
);
CREATE INDEX steps_conversation ON steps(conversation_id, seq);

CREATE TABLE chat_items (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    seq             BIGSERIAL,
    doc             JSONB NOT NULL
);
CREATE INDEX chat_conversation ON chat_items(conversation_id, seq);

-- One per "ask" (D-024). Decided and released in separate atomic steps.
CREATE TABLE approvals (
    id              TEXT PRIMARY KEY,
    step_id         TEXT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    launch_epoch    TEXT NOT NULL,
    request_key     TEXT NOT NULL,
    payload_digest  TEXT NOT NULL,
    exec_digest     TEXT NOT NULL DEFAULT '',
    reason          TEXT NOT NULL,
    loss_title      TEXT NOT NULL DEFAULT '',
    loss            JSONB NOT NULL DEFAULT '[]',
    warning         TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL DEFAULT 'pending',
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at      TIMESTAMPTZ,
    consumed_at     TIMESTAMPTZ
);
CREATE INDEX approvals_pending ON approvals(state) WHERE state = 'pending';

-- Single-use tokens that let cloudbuddy-exec run one allowed command (D-036).
CREATE TABLE releases (
    token_hash      TEXT PRIMARY KEY,
    step_id         TEXT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    run_id          TEXT NOT NULL,
    launch_epoch    TEXT NOT NULL,
    approval_id     TEXT,
    command         TEXT NOT NULL,
    cwd             TEXT NOT NULL,
    network_kind    TEXT NOT NULL,
    exec_digest     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at     TIMESTAMPTZ
);

CREATE TABLE link_tokens (
    conversation_id TEXT NOT NULL,
    step_id         TEXT NOT NULL,
    token_hash      TEXT NOT NULL,
    what            TEXT NOT NULL,
    PRIMARY KEY (conversation_id, token_hash, step_id)
);

CREATE TABLE connections (
    id          TEXT PRIMARY KEY,
    doc         JSONB NOT NULL,
    scanned_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE connection_rules (
    connection_id TEXT PRIMARY KEY,
    rule          TEXT NOT NULL
);
