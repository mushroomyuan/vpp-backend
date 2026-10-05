CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    parent_id UUID NULL,
    display_name TEXT NOT NULL,
    type TEXT NOT NULL,
    sub_type TEXT NULL,
    lifecycle_status TEXT NOT NULL DEFAULT 'active',
    description TEXT NULL,
    path TEXT NOT NULL DEFAULT '',
    depth INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NULL,
    version BIGINT NOT NULL DEFAULT 1,
    deleted_at TIMESTAMPTZ NULL,
    deleted_by TEXT NULL,
    delete_job_id TEXT NULL,
    delete_reason TEXT NULL,
    restored_at TIMESTAMPTZ NULL,
    restored_by TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_nodes_parent FOREIGN KEY (parent_id) REFERENCES nodes (id)
);

CREATE INDEX IF NOT EXISTS idx_nodes_tenant_id ON nodes (tenant_id);
CREATE INDEX IF NOT EXISTS idx_nodes_parent_id ON nodes (parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes (type);
CREATE INDEX IF NOT EXISTS idx_nodes_deleted_at ON nodes (deleted_at);

CREATE TABLE IF NOT EXISTS sites (
    node_id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    operating_status SMALLINT NOT NULL DEFAULT 0,
    location JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_sites_node FOREIGN KEY (node_id) REFERENCES nodes (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sites_tenant_id ON sites (tenant_id);

CREATE TABLE IF NOT EXISTS assets (
    node_id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    dispatch_status TEXT NOT NULL DEFAULT 'unknown',
    rated_capacity_kw DOUBLE PRECISION NULL,
    dispatch_mode TEXT NULL,
    energy_type TEXT NULL,
    owner_type TEXT NULL,
    market_enabled BOOLEAN NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_assets_node FOREIGN KEY (node_id) REFERENCES nodes (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_assets_tenant_id ON assets (tenant_id);

CREATE TABLE IF NOT EXISTS cus (
    node_id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    conn_status TEXT NOT NULL DEFAULT 'disconnected',
    provider TEXT NULL,
    external_id TEXT NULL,
    protocol TEXT NULL,
    protocol_config JSONB NULL,
    connection JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cus_node FOREIGN KEY (node_id) REFERENCES nodes (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_cus_tenant_id ON cus (tenant_id);

CREATE TABLE IF NOT EXISTS points (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    node_id UUID NULL,
    asset_id UUID NOT NULL,
    cu_id UUID NOT NULL,
    metric_id TEXT NOT NULL,
    external_address TEXT NOT NULL,
    access_mode TEXT NOT NULL DEFAULT 'read',
    scale DOUBLE PRECISION NOT NULL DEFAULT 1,
    "offset" DOUBLE PRECISION NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT fk_points_node FOREIGN KEY (node_id) REFERENCES nodes (id),
    CONSTRAINT fk_points_asset FOREIGN KEY (asset_id) REFERENCES assets (node_id),
    CONSTRAINT fk_points_cu FOREIGN KEY (cu_id) REFERENCES cus (node_id),
    CONSTRAINT chk_points_access_mode CHECK (access_mode IN ('read', 'write', 'read_write')),
    CONSTRAINT chk_points_scale_nonzero CHECK (scale <> 0),
    CONSTRAINT chk_points_revision_positive CHECK (revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_points_tenant_id ON points (tenant_id);
CREATE INDEX IF NOT EXISTS idx_points_node_id ON points (node_id);
CREATE INDEX IF NOT EXISTS idx_points_asset_id ON points (asset_id);
CREATE INDEX IF NOT EXISTS idx_points_cu_id ON points (cu_id);
CREATE INDEX IF NOT EXISTS idx_points_deleted_at ON points (deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_points_active_metric
    ON points (tenant_id, cu_id, metric_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_points_active_external_address
    ON points (tenant_id, cu_id, external_address) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS point_safety_constraints (
    point_id UUID PRIMARY KEY,
    min_value DOUBLE PRECISION NULL,
    max_value DOUBLE PRECISION NULL,
    max_change_per_second DOUBLE PRECISION NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_point_safety_constraints_point
        FOREIGN KEY (point_id) REFERENCES points (id) ON DELETE CASCADE,
    CONSTRAINT chk_point_safety_min_max
        CHECK (min_value IS NULL OR max_value IS NULL OR min_value <= max_value),
    CONSTRAINT chk_point_safety_change_positive
        CHECK (max_change_per_second IS NULL OR max_change_per_second > 0),
    CONSTRAINT chk_point_safety_version_positive CHECK (version > 0)
);

CREATE TABLE IF NOT EXISTS cu_capabilities (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    cu_id UUID NOT NULL,
    capability_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    spec JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cu_capabilities_cu
        FOREIGN KEY (cu_id) REFERENCES cus (node_id) ON DELETE CASCADE,
    CONSTRAINT chk_cu_capabilities_schema_version CHECK (schema_version > 0),
    CONSTRAINT chk_cu_capabilities_version CHECK (version > 0),
    CONSTRAINT chk_cu_capabilities_spec_object CHECK (jsonb_typeof(spec) = 'object')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cu_capabilities_active
    ON cu_capabilities (tenant_id, cu_id, capability_id);
CREATE INDEX IF NOT EXISTS idx_cu_capabilities_tenant_cu
    ON cu_capabilities (tenant_id, cu_id);
CREATE INDEX IF NOT EXISTS idx_cu_capabilities_id_enabled
    ON cu_capabilities (capability_id, enabled);

CREATE TABLE IF NOT EXISTS import_jobs (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    payload JSONB NOT NULL,
    total INTEGER DEFAULT 0,
    succeeded INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    error_msg TEXT NOT NULL,
    result_json JSONB NULL,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    next_retry_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_import_jobs_tenant_id ON import_jobs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs (status);
