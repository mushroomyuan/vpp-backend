-- Decision policy storage. Specs are typed columns, not an unchecked JSONB blob.
-- Applied against the decision database (see 90-decision-db.sh).
-- Keep in sync with migrations/decision/000001_init.up.sql.

\c decision

CREATE TABLE IF NOT EXISTS decision_policies (
    id            UUID        PRIMARY KEY,
    tenant_id     TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    kind          TEXT        NOT NULL,
    scope_type    TEXT        NOT NULL,
    scope_id      TEXT        NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT FALSE,
    cooldown_ns   BIGINT      NOT NULL DEFAULT 0,
    version       BIGINT      NOT NULL DEFAULT 1,
    created_by    TEXT        NOT NULL DEFAULT '',
    updated_by    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ NULL,
    deleted_by    TEXT        NULL,
    CONSTRAINT chk_decision_policies_tenant_name
        CHECK (btrim(tenant_id) <> '' AND btrim(name) <> ''),
    CONSTRAINT chk_decision_policies_kind
        CHECK (kind IN ('soc_threshold')),
    CONSTRAINT chk_decision_policies_scope
        CHECK (scope_type IN ('site', 'asset', 'cu') AND btrim(scope_id) <> ''),
    CONSTRAINT chk_decision_policies_cooldown
        CHECK (cooldown_ns >= 0),
    CONSTRAINT chk_decision_policies_version
        CHECK (version > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_decision_policies_tenant_name
    ON decision_policies (tenant_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_decision_policies_tenant
    ON decision_policies (tenant_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS decision_soc_threshold_specs (
    policy_id          UUID             PRIMARY KEY,
    min_soc            DOUBLE PRECISION NOT NULL,
    max_soc            DOUBLE PRECISION NOT NULL,
    charge_power_kw    DOUBLE PRECISION NOT NULL,
    discharge_power_kw DOUBLE PRECISION NOT NULL,
    CONSTRAINT fk_decision_soc_threshold_specs_policy
        FOREIGN KEY (policy_id) REFERENCES decision_policies (id),
    CONSTRAINT chk_decision_soc_bounds
        CHECK (min_soc >= 0 AND max_soc <= 100 AND min_soc < max_soc),
    CONSTRAINT chk_decision_soc_power
        CHECK (charge_power_kw > 0 AND discharge_power_kw > 0)
);
