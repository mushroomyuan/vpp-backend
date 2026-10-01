package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
)

// resolveScopeSQL expands a scope by node path.
//
// Site and asset scopes include every descendant CU, including CUs nested under
// child assets or child CUs. That is path prefix matching, not the direct
// asset parent join used by list filters, which misses those nested CUs.
// A CU scope returns that CU only.
//
// The payload omits points.external_address.
const resolveScopeSQL = `
WITH root AS (
    SELECT
        id,
        tenant_id,
        type,
        version,
        COALESCE(NULLIF(btrim(path), ''), id::text) AS path
    FROM nodes
    WHERE tenant_id = ?
      AND id = CAST(? AS uuid)
      AND deleted_at IS NULL
),
scope_nodes AS (
    SELECT
        n.id,
        n.type,
        n.version,
        n.lifecycle_status,
        n.depth,
        COALESCE(NULLIF(btrim(n.path), ''), n.id::text) AS path
    FROM nodes AS n
    JOIN root AS r ON n.tenant_id = r.tenant_id
    WHERE n.deleted_at IS NULL
      AND n.type IN ('site', 'asset', 'cu')
      AND (
          n.id = r.id
          OR (
              r.type IN ('site', 'asset')
              AND n.path LIKE r.path || '/%'
          )
      )
),
scope_cus AS (
    SELECT DISTINCT ON (cu.id)
        cu.id AS cu_id,
        cu.lifecycle_status,
        cu.version AS node_version,
        a.id AS asset_id
    FROM scope_nodes AS cu
    LEFT JOIN nodes AS a
        ON a.tenant_id = (SELECT tenant_id FROM root)
       AND a.deleted_at IS NULL
       AND a.type = 'asset'
       AND cu.path LIKE COALESCE(NULLIF(btrim(a.path), ''), a.id::text) || '/%'
    WHERE cu.type = 'cu'
    ORDER BY cu.id, a.depth DESC NULLS LAST, a.id ASC
)
SELECT jsonb_build_object(
    'found', EXISTS (SELECT 1 FROM root),
    'root_id', (SELECT id::text FROM root),
    'root_type', (SELECT type FROM root),
    'nodes', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'entity_type', sn.type,
            'entity_id', sn.id::text,
            'version', sn.version::text
        ))
        FROM scope_nodes AS sn
    ), '[]'::jsonb),
    'cus', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'cu_id', c.cu_id::text,
            'asset_id', c.asset_id::text,
            'lifecycle_status', c.lifecycle_status,
            'node_version', c.node_version::text,
            'capabilities', COALESCE((
                SELECT jsonb_agg(jsonb_build_object(
                    'capability_id', cc.capability_id,
                    'schema_version', cc.schema_version,
                    'spec', cc.spec,
                    'enabled', cc.enabled,
                    'version', cc.version::text
                ))
                FROM cu_capabilities AS cc
                WHERE cc.tenant_id = (SELECT tenant_id FROM root)
                  AND cc.cu_id = c.cu_id
            ), '[]'::jsonb),
            'bindings', COALESCE((
                SELECT jsonb_agg(jsonb_build_object(
                    'metric_id', p.metric_id,
                    'access_mode', p.access_mode,
                    'enabled', p.enabled,
                    'revision', p.revision::text,
                    'min_value', sc.min_value,
                    'max_value', sc.max_value,
                    'max_change_per_second', sc.max_change_per_second,
                    'safety_version', CASE
                        WHEN sc.point_id IS NULL THEN NULL
                        ELSE sc.version::text
                    END
                ))
                FROM points AS p
                LEFT JOIN point_safety_constraints AS sc ON sc.point_id = p.id
                WHERE p.tenant_id = (SELECT tenant_id FROM root)
                  AND p.cu_id = c.cu_id
                  AND p.deleted_at IS NULL
            ), '[]'::jsonb)
        ))
        FROM scope_cus AS c
    ), '[]'::jsonb)
)
`

const scopeQueryReadOnly = "SET TRANSACTION READ ONLY"

// ScopeRepository loads a scope snapshot with one read-only statement.
type ScopeRepository struct {
	pg *Postgres
}

func NewScopeRepository(pg *Postgres) *ScopeRepository {
	return &ScopeRepository{pg: pg}
}

func (r *ScopeRepository) Query(ctx context.Context, tenantID, scopeID string) (payload []byte, err error) {
	_, deferLog := logging.WhenDB(ctx, "ScopeRepository.Query", scopeID)
	defer func() { deferLog(len(payload), &err) }()

	err = r.pg.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(scopeQueryReadOnly).Error; err != nil {
			return err
		}
		return tx.Raw(resolveScopeSQL, tenantID, scopeID).Row().Scan(&payload)
	})
	return payload, err
}
