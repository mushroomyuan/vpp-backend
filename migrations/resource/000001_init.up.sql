-- Resource service: initial schema (aligns with internal/resource/.../postgres/models.go)
-- Apply to a dedicated test database, e.g. CREATE DATABASE resource;

BEGIN;

CREATE TABLE sites (
    id           UUID PRIMARY KEY,
    tenant_id    TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    location     JSONB,
    description  TEXT,
    status       SMALLINT    NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX idx_sites_tenant_id  ON sites (tenant_id);
CREATE INDEX idx_sites_deleted_at ON sites (deleted_at);

CREATE TABLE resources (
    id            UUID PRIMARY KEY,
    tenant_id     TEXT        NOT NULL,
    site_id       UUID        NOT NULL REFERENCES sites (id),
    name          TEXT        NOT NULL,
    type          TEXT        NOT NULL,
    capacity      DOUBLE PRECISION,
    manufacturer  TEXT,
    model         TEXT,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX idx_resources_tenant_id ON resources (tenant_id);
CREATE INDEX idx_resources_site_id   ON resources (site_id);
CREATE INDEX idx_resources_deleted_at ON resources (deleted_at);

CREATE TABLE cus (
    id               UUID PRIMARY KEY,
    resource_id      UUID        NOT NULL REFERENCES resources (id),
    parent_cu_id     UUID REFERENCES cus (id),
    name             TEXT        NOT NULL,
    type             TEXT,
    capability_tags  JSONB,
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX idx_cus_resource_id  ON cus (resource_id);
CREATE INDEX idx_cus_deleted_at   ON cus (deleted_at);

CREATE TABLE points (
    id                 UUID PRIMARY KEY,
    resource_id        UUID        NOT NULL REFERENCES resources (id),
    cu_id              UUID        NOT NULL REFERENCES cus (id),
    point_key          TEXT        NOT NULL,
    external_address   TEXT,
    data_type          TEXT        NOT NULL,
    ext_config         JSONB,
    description        TEXT,
    control_flag       BOOLEAN     NOT NULL DEFAULT FALSE,
    is_virtual         BOOLEAN     NOT NULL DEFAULT FALSE,
    safety_thresholds  JSONB,
    cache_key_alias    TEXT,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    deleted_at         TIMESTAMPTZ
);

CREATE INDEX idx_points_resource_id ON points (resource_id);
CREATE INDEX idx_points_cu_id       ON points (cu_id);
CREATE INDEX idx_points_deleted_at  ON points (deleted_at);

CREATE TABLE import_jobs (
    id            UUID PRIMARY KEY,
    tenant_id     TEXT        NOT NULL,
    type          TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    payload       JSONB       NOT NULL,
    total         INTEGER     NOT NULL DEFAULT 0,
    succeeded     INTEGER     NOT NULL DEFAULT 0,
    failed_count  INTEGER     NOT NULL DEFAULT 0,
    error_msg     TEXT,
    result_json   JSONB,
    attempts      INTEGER     NOT NULL DEFAULT 0,
    max_attempts  INTEGER     NOT NULL DEFAULT 3,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ
);

CREATE INDEX idx_import_jobs_tenant_id ON import_jobs (tenant_id);
CREATE INDEX idx_import_jobs_status    ON import_jobs (status);
-- Helps ClaimPendingJob: filter by status + order by created_at
CREATE INDEX idx_import_jobs_status_created_at ON import_jobs (status, created_at);

COMMIT;
