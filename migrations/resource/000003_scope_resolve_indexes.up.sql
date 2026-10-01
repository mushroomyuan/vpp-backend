-- ResolveScope reads the tree by path prefix and looks up capabilities and enabled bindings in bulk.
CREATE INDEX IF NOT EXISTS idx_nodes_tenant_path
    ON nodes (tenant_id, path text_pattern_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_cu_capabilities_tenant_capability
    ON cu_capabilities (tenant_id, capability_id)
    WHERE enabled;

CREATE INDEX IF NOT EXISTS idx_points_enabled_binding
    ON points (tenant_id, cu_id, metric_id, access_mode)
    WHERE deleted_at IS NULL AND enabled;
