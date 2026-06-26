package postgres

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NodeRepository wraps low-level GORM operations for the nodes table.
// It has no knowledge of domain types or domain errors; those concerns belong
// to the adapter layer.
type NodeRepository struct {
	pg *Postgres
}

func NewNodeRepository(pg *Postgres) *NodeRepository {
	return &NodeRepository{pg: pg}
}

// CreateNode inserts a new node row and back-fills auto-generated columns via
// RETURNING *.
func (r *NodeRepository) CreateNode(ctx context.Context, m *NodeModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.CreateNode", m)
	defer func() { deferLog(m, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

// FindNodeByID returns the node matching tenantID + id.
// Returns gorm.ErrRecordNotFound when no row matches.
func (r *NodeRepository) FindNodeByID(ctx context.Context, tenantID, id string) (result *NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.FindNodeByID", id)
	defer func() { deferLog(result, &err) }()
	var m NodeModel
	err = r.pg.DB().WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindNodeByIDForUpdate is identical to FindNodeByID but acquires a FOR UPDATE
// row-level lock. Must be called inside a transaction (pass the *gorm.DB from
// the transaction callback).
func (r *NodeRepository) FindNodeByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (result *NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.FindNodeByIDForUpdate", id)
	defer func() { deferLog(result, &err) }()
	var m NodeModel
	err = r.pg.UseTransaction(tx).WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindNodeByIDTx reads a node inside an existing transaction (no row lock).
// Use this for related rows while a transaction is already open.
func (r *NodeRepository) FindNodeByIDTx(ctx context.Context, tx *gorm.DB, tenantID, id string) (result *NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.FindNodeByIDTx", id)
	defer func() { deferLog(result, &err) }()
	var m NodeModel
	err = r.pg.UseTransaction(tx).WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ExistsNode returns true when a non-deleted node row exists for tenantID + id.
func (r *NodeRepository) ExistsNode(ctx context.Context, tenantID, id string) (exists bool, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.ExistsNode", id)
	defer func() { deferLog(exists, &err) }()
	var count int64
	err = r.pg.DB().WithContext(ctx).
		Model(&NodeModel{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Count(&count).Error
	return count > 0, err
}

// UpdateNodeFields applies a partial update (map of column → value) to the
// node identified by tenantID + id.
// Returns gorm.ErrRecordNotFound when no row was modified.
func (r *NodeRepository) UpdateNodeFields(ctx context.Context, tenantID, id string, updates map[string]any) (err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.UpdateNodeFields", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&NodeModel{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateNodeFieldsTx is the transactional variant of UpdateNodeFields for use
// inside a transaction started by RunInTx.
func (r *NodeRepository) UpdateNodeFieldsTx(ctx context.Context, tx *gorm.DB, tenantID, id string, updates map[string]any) (err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.UpdateNodeFieldsTx", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.UseTransaction(tx).WithContext(ctx).
		Model(&NodeModel{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListChildren returns direct children of parentID ordered by display_name.
// Pass an empty string for parentID to list root-level nodes (parent_id IS NULL).
func (r *NodeRepository) ListChildren(ctx context.Context, tenantID, parentID string) (results []*NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.ListChildren", parentID)
	defer func() { deferLog(results, &err) }()
	q := r.pg.DB().WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("display_name ASC")
	if strings.TrimSpace(parentID) == "" {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", parentID)
	}
	err = q.Find(&results).Error
	return
}

// FindDescendants returns all non-deleted nodes whose path starts with
// pathPrefix+"/" (i.e. all proper descendants of a node), ordered by depth
// then path. The caller is responsible for computing pathPrefix (typically
// the node's own Path column value; fall back to the node's ID when Path is
// empty).
func (r *NodeRepository) FindDescendants(ctx context.Context, tenantID, pathPrefix string) (results []*NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.FindDescendants", pathPrefix)
	defer func() { deferLog(results, &err) }()
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND path LIKE ?", tenantID, pathPrefix+"/%").
		Order("depth ASC").
		Order("path ASC").
		Find(&results).Error
	return
}

// FindDescendantsTx is the transactional variant of FindDescendants.
// excludeID may be passed to skip one specific node (typically the subtree root
// being moved, which is updated separately).
func (r *NodeRepository) FindDescendantsTx(ctx context.Context, tx *gorm.DB, tenantID, pathPrefix, excludeID string) (results []*NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.FindDescendantsTx", pathPrefix)
	defer func() { deferLog(results, &err) }()
	q := r.pg.UseTransaction(tx).WithContext(ctx).
		Where("tenant_id = ? AND path LIKE ?", tenantID, pathPrefix+"/%")
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	err = q.Find(&results).Error
	return
}

// TenantIDByNodeID returns tenant_id for a node primary key (any type).
func (r *NodeRepository) TenantIDByNodeID(ctx context.Context, nodeID string) (tenantID string, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.TenantIDByNodeID", nodeID)
	defer func() { deferLog(tenantID, &err) }()
	err = r.pg.DB().WithContext(ctx).
		Model(&NodeModel{}).
		Select("tenant_id").
		Where("id = ?", nodeID).
		Limit(1).
		Scan(&tenantID).Error
	if err != nil {
		return "", err
	}
	if tenantID == "" {
		return "", gorm.ErrRecordNotFound
	}
	return tenantID, nil
}

// ListByIDs returns all non-deleted nodes matching ids within a tenant.
func (r *NodeRepository) ListByIDs(ctx context.Context, tenantID string, ids []string) (results []*NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.ListByIDs", ids)
	defer func() { deferLog(results, &err) }()
	if len(ids) == 0 {
		return nil, nil
	}
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&results).Error
	return
}

// GetAncestorChain walks parent_id links from id up to the root and returns
// the chain in root-first order (i.e. root at index 0, id at the last index).
// Note: this issues one SQL round-trip per level; use only for shallow trees
// or breadcrumb rendering.
func (r *NodeRepository) GetAncestorChain(ctx context.Context, tenantID, id string) (results []*NodeModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.GetAncestorChain", id)
	defer func() { deferLog(results, &err) }()
	var chain []*NodeModel
	curID := id
	for i := 0; i < 1024; i++ {
		var row NodeModel
		if err = r.pg.DB().WithContext(ctx).
			Where("id = ? AND tenant_id = ?", curID, tenantID).
			First(&row).Error; err != nil {
			return nil, err
		}
		chain = append(chain, &row)
		if row.ParentID == nil || strings.TrimSpace(*row.ParentID) == "" {
			break
		}
		curID = strings.TrimSpace(*row.ParentID)
	}
	// reverse to root-first order
	for l, rIdx := 0, len(chain)-1; l < rIdx; l, rIdx = l+1, rIdx-1 {
		chain[l], chain[rIdx] = chain[rIdx], chain[l]
	}
	return chain, nil
}

// SoftDeleteNode sets deleted_at on the single node identified by tenantID + id.
// Returns gorm.ErrRecordNotFound when no row was deleted.
func (r *NodeRepository) SoftDeleteNode(ctx context.Context, tenantID, id string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.SoftDeleteNode", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&NodeModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SoftDeleteSubtree soft-deletes the root node (matched by id) and all
// descendants (matched by pathPrefix+"/%") in a single DELETE statement.
func (r *NodeRepository) SoftDeleteSubtree(ctx context.Context, tenantID, id, pathPrefix string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "NodeRepository.SoftDeleteSubtree", id)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("id = ? OR path LIKE ?", id, pathPrefix+"/%").
		Delete(&NodeModel{}).Error
}

// RunInTx starts a database transaction and passes the *gorm.DB handle to f.
// The transaction is committed on nil return and rolled back on any error.
func (r *NodeRepository) RunInTx(f func(tx *gorm.DB) error) error {
	return r.pg.StartTransaction(f)
}
