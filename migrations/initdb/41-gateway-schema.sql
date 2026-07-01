-- Gateway service schema: device_mappings table.
-- Applied against the gateway database (see 40-gateway-db.sh).

\c gateway

CREATE TABLE IF NOT EXISTS device_mappings (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       VARCHAR(64)  NOT NULL,
    external_system VARCHAR(64)  NOT NULL,
    external_id     VARCHAR(128) NOT NULL,
    cu_code         VARCHAR(128) NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, external_system, external_id)
);

CREATE INDEX IF NOT EXISTS idx_device_mappings_cu
    ON device_mappings(tenant_id, cu_code);

CREATE INDEX IF NOT EXISTS idx_device_mappings_status
    ON device_mappings(tenant_id, status);
