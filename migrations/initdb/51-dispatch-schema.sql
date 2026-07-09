-- Dispatch service schema: tasks, actions, and control commands.
-- Applied against the dispatch database (see 50-dispatch-db.sh).

\c dispatch

CREATE TABLE IF NOT EXISTS dispatch_tasks (
    id             TEXT        PRIMARY KEY,
    tenant_id      TEXT        NOT NULL,
    name           TEXT        NOT NULL,
    description    TEXT,
    type           TEXT        NOT NULL,
    trigger_type   TEXT        NOT NULL,
    failure_policy TEXT        NOT NULL DEFAULT 'fail_fast',
    status         TEXT        NOT NULL DEFAULT 'pending',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_dispatch_tasks_tenant_status
    ON dispatch_tasks (tenant_id, status);

CREATE TABLE IF NOT EXISTS dispatch_actions (
    id               TEXT PRIMARY KEY,
    task_id          TEXT NOT NULL REFERENCES dispatch_tasks(id),
    tenant_id        TEXT NOT NULL,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL,
    sequence         INT  NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending',
    execution_policy TEXT NOT NULL DEFAULT 'sequential'
);

CREATE INDEX IF NOT EXISTS idx_dispatch_actions_task_id
    ON dispatch_actions (task_id);

CREATE TABLE IF NOT EXISTS control_commands (
    id          TEXT        PRIMARY KEY,
    action_id   TEXT        NOT NULL REFERENCES dispatch_actions(id),
    tenant_id   TEXT        NOT NULL,
    sequence    INT         NOT NULL,
    cu_code     TEXT        NOT NULL,
    point_key   TEXT        NOT NULL,
    value       JSONB       NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending',
    retry_count INT         NOT NULL DEFAULT 0,
    max_retries INT         NOT NULL DEFAULT 3,
    timeout_ms  BIGINT      NOT NULL DEFAULT 30000,
    sent_at     TIMESTAMPTZ,
    deadline_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    result      JSONB
);

CREATE INDEX IF NOT EXISTS idx_control_commands_action_id
    ON control_commands (action_id);

CREATE INDEX IF NOT EXISTS idx_control_commands_timeout_scan
    ON control_commands (deadline_at)
    WHERE status = 'sending';
