-- Alarm service schema: open tickets + append-only ingest dedup.
-- Applied against the alarm database (see 70-alarm-db.sh).

\c alarm

CREATE TABLE IF NOT EXISTS alarms (
    id                 UUID        PRIMARY KEY,
    tenant_id          TEXT        NOT NULL,
    fingerprint        TEXT        NOT NULL,
    source             TEXT        NOT NULL,
    status             TEXT        NOT NULL,
    severity           TEXT        NOT NULL,
    rule_id            TEXT        NOT NULL,
    title              TEXT        NOT NULL,
    summary            TEXT        NOT NULL DEFAULT '',
    source_ref         TEXT        NOT NULL DEFAULT '',
    attributes         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    attributes_schema  SMALLINT    NOT NULL DEFAULT 1,
    count              INT         NOT NULL DEFAULT 1,
    first_occurred_at  TIMESTAMPTZ NOT NULL,
    last_occurred_at   TIMESTAMPTZ NOT NULL,
    last_event_id      TEXT        NOT NULL,
    acknowledged_at    TIMESTAMPTZ,
    acknowledged_by    TEXT        NOT NULL DEFAULT '',
    closed_at          TIMESTAMPTZ,
    closed_by          TEXT        NOT NULL DEFAULT '',
    version            INT         NOT NULL DEFAULT 1,
    CONSTRAINT alarms_status_chk CHECK (status IN ('open', 'acknowledged', 'closed')),
    CONSTRAINT alarms_severity_chk CHECK (severity IN ('critical', 'warning', 'info')),
    -- source is intentionally not an enum: new alarm business types register a
    -- new model.Source in Go (see model.ParseSource), not a schema migration.
    CONSTRAINT alarms_source_chk CHECK (source <> ''),
    CONSTRAINT alarms_count_chk CHECK (count >= 1),
    CONSTRAINT alarms_version_chk CHECK (version >= 1)
);

CREATE INDEX IF NOT EXISTS alarms_tenant_status_last_idx
    ON alarms (tenant_id, status, last_occurred_at DESC);

CREATE INDEX IF NOT EXISTS alarms_tenant_fingerprint_idx
    ON alarms (tenant_id, fingerprint);

CREATE UNIQUE INDEX IF NOT EXISTS alarms_open_fingerprint_uidx
    ON alarms (tenant_id, fingerprint)
    WHERE status <> 'closed';

CREATE TABLE IF NOT EXISTS alarm_event_dedup (
    tenant_id   TEXT        NOT NULL,
    event_id    TEXT        NOT NULL,
    alarm_id    UUID        NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, event_id)
);

CREATE INDEX IF NOT EXISTS alarm_event_dedup_ingested_at_idx
    ON alarm_event_dedup (ingested_at);
