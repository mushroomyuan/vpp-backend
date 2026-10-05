-- Plan, cooldown, and outbox for the decision database.
-- Applied against the decision database (see 90-decision-db.sh).
-- Keep in sync with migrations/decision/000002_plan_execution.up.sql.

\c decision

-- Keep migrations/initdb/92-decision-plan.sql in sync with this file.

CREATE TABLE IF NOT EXISTS decision_objectives (
    id               UUID        PRIMARY KEY,
    tenant_id        TEXT        NOT NULL,
    kind             TEXT        NOT NULL,
    scope_type       TEXT        NOT NULL,
    scope_id         TEXT        NOT NULL,
    priority         INTEGER     NOT NULL,
    source_type      TEXT        NOT NULL,
    source_id        TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    policy_id        UUID        NULL REFERENCES decision_policies (id),
    policy_version   BIGINT      NOT NULL DEFAULT 0,
    window_start     TIMESTAMPTZ NOT NULL,
    window_end       TIMESTAMPTZ NOT NULL,
    status           TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_decision_objectives_idempotency UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT chk_decision_objectives_tenant CHECK (btrim(tenant_id) <> ''),
    CONSTRAINT chk_decision_objectives_kind CHECK (kind IN ('power')),
    CONSTRAINT chk_decision_objectives_scope
        CHECK (scope_type IN ('site', 'asset', 'cu') AND btrim(scope_id) <> ''),
    CONSTRAINT chk_decision_objectives_source
        CHECK (source_type IN ('policy', 'manual', 'demand_response', 'market', 'external')),
    CONSTRAINT chk_decision_objectives_status CHECK (status IN ('planned')),
    CONSTRAINT chk_decision_objectives_window CHECK (window_end > window_start)
);

CREATE TABLE IF NOT EXISTS decision_power_objectives (
    objective_id    UUID             PRIMARY KEY REFERENCES decision_objectives (id),
    metric_id       TEXT             NOT NULL,
    target_power_kw DOUBLE PRECISION NOT NULL,
    CONSTRAINT chk_decision_power_objectives_metric CHECK (btrim(metric_id) <> ''),
    CONSTRAINT chk_decision_power_objectives_power CHECK (target_power_kw <> 0)
);

CREATE TABLE IF NOT EXISTS decision_plans (
    id                UUID             PRIMARY KEY,
    objective_id      UUID             NOT NULL REFERENCES decision_objectives (id),
    tenant_id         TEXT             NOT NULL,
    policy_id         UUID             NULL REFERENCES decision_policies (id),
    policy_version    BIGINT           NOT NULL DEFAULT 0,
    resource_revision TEXT             NOT NULL,
    planner_id        TEXT             NOT NULL,
    planner_version   TEXT             NOT NULL,
    status            TEXT             NOT NULL,
    feasibility       TEXT             NOT NULL,
    unmet_power_kw    DOUBLE PRECISION NOT NULL,
    window_start      TIMESTAMPTZ      NOT NULL,
    window_end        TIMESTAMPTZ      NOT NULL,
    generated_at      TIMESTAMPTZ      NOT NULL,
    created_at        TIMESTAMPTZ      NOT NULL,
    CONSTRAINT chk_decision_plans_status
        CHECK (status IN ('ready', 'rejected', 'stale', 'submitted')),
    CONSTRAINT chk_decision_plans_feasibility
        CHECK (feasibility IN ('feasible', 'partially_feasible', 'infeasible')),
    CONSTRAINT chk_decision_plans_revision CHECK (btrim(resource_revision) <> ''),
    CONSTRAINT chk_decision_plans_window CHECK (window_end > window_start)
);

CREATE INDEX IF NOT EXISTS idx_decision_plans_objective ON decision_plans (objective_id);

CREATE TABLE IF NOT EXISTS decision_plan_steps (
    id               UUID        PRIMARY KEY,
    plan_id          UUID        NOT NULL REFERENCES decision_plans (id),
    ordinal          INTEGER     NOT NULL,
    execute_at       TIMESTAMPTZ NOT NULL,
    status           TEXT        NOT NULL,
    version          BIGINT      NOT NULL,
    dispatch_task_id TEXT        NOT NULL DEFAULT '',
    CONSTRAINT uq_decision_plan_steps_ordinal UNIQUE (plan_id, ordinal),
    CONSTRAINT chk_decision_plan_steps_status
        CHECK (status IN ('ready', 'rejected', 'stale', 'submitted')),
    CONSTRAINT chk_decision_plan_steps_version CHECK (version > 0)
);

CREATE TABLE IF NOT EXISTS decision_planned_commands (
    id                            UUID             PRIMARY KEY,
    step_id                       UUID             NOT NULL REFERENCES decision_plan_steps (id),
    cu_code                       TEXT             NOT NULL,
    metric_id                     TEXT             NOT NULL,
    value_kind                    TEXT             NOT NULL,
    bool_value                    BOOLEAN          NULL,
    int_value                     BIGINT           NULL,
    float_value                   DOUBLE PRECISION NULL,
    string_value                  TEXT             NULL,
    binding_revision              BIGINT           NOT NULL,
    safety_min                    DOUBLE PRECISION NULL,
    safety_max                    DOUBLE PRECISION NULL,
    safety_max_change_per_second  DOUBLE PRECISION NULL,
    safety_version                BIGINT           NULL,
    dispatch_task_id              TEXT             NOT NULL DEFAULT '',
    CONSTRAINT chk_decision_planned_commands_kind
        CHECK (value_kind IN ('bool', 'int', 'float', 'string')),
    CONSTRAINT chk_decision_planned_commands_value CHECK (
        (CASE WHEN bool_value IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN int_value IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN float_value IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN string_value IS NOT NULL THEN 1 ELSE 0 END) = 1
    ),
    CONSTRAINT chk_decision_planned_commands_binding CHECK (binding_revision > 0),
    CONSTRAINT uq_decision_planned_commands_target UNIQUE (step_id, cu_code, metric_id)
);

CREATE TABLE IF NOT EXISTS decision_plan_executions (
    id               UUID        PRIMARY KEY,
    tenant_id        TEXT        NOT NULL,
    plan_id          UUID        NOT NULL REFERENCES decision_plans (id),
    step_id          UUID        NOT NULL REFERENCES decision_plan_steps (id),
    idempotency_key  TEXT        NOT NULL,
    attempt          BIGINT      NOT NULL,
    status           TEXT        NOT NULL,
    dispatch_task_id TEXT        NOT NULL DEFAULT '',
    error            TEXT        NOT NULL DEFAULT '',
    claimed_by       TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_decision_plan_executions_attempt UNIQUE (step_id, attempt),
    CONSTRAINT chk_decision_plan_executions_attempt CHECK (attempt > 0),
    CONSTRAINT chk_decision_plan_executions_key CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_decision_plan_executions_status
        CHECK (status IN ('claimed', 'submitted', 'uncertain', 'failed', 'stale'))
);

CREATE TABLE IF NOT EXISTS decision_policy_cooldowns (
    tenant_id         TEXT        NOT NULL,
    policy_id         UUID        NOT NULL REFERENCES decision_policies (id),
    direction         TEXT        NOT NULL,
    cooldown_until    TIMESTAMPTZ NOT NULL,
    last_objective_id UUID        NULL,
    last_plan_id      UUID        NULL,
    version           BIGINT      NOT NULL DEFAULT 1,
    updated_at        TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, policy_id, direction),
    CONSTRAINT chk_decision_policy_cooldowns_direction
        CHECK (direction IN ('charge', 'discharge')),
    CONSTRAINT chk_decision_policy_cooldowns_version CHECK (version > 0)
);

CREATE TABLE IF NOT EXISTS decision_outbox (
    id           UUID        PRIMARY KEY,
    tenant_id    TEXT        NOT NULL,
    plan_id      UUID        NOT NULL REFERENCES decision_plans (id),
    step_id      UUID        NOT NULL UNIQUE REFERENCES decision_plan_steps (id),
    available_at TIMESTAMPTZ NOT NULL,
    status       TEXT        NOT NULL,
    claimed_by   TEXT        NOT NULL DEFAULT '',
    lease_until  TIMESTAMPTZ NULL,
    lease_token  BIGINT      NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    CONSTRAINT chk_decision_outbox_status
        CHECK (status IN ('pending', 'claimed', 'done', 'stale', 'failed')),
    CONSTRAINT chk_decision_outbox_token CHECK (lease_token >= 0)
);

CREATE INDEX IF NOT EXISTS idx_decision_outbox_due
    ON decision_outbox (available_at, id)
    WHERE status IN ('pending', 'claimed');
