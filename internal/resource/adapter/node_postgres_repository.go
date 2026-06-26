package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

// NodeRepositoryPostgres implements port.NodeRepository via postgres.NodeRepository.
type NodeRepositoryPostgres struct {
	repo *postgres.NodeRepository
}

func NewNodeRepositoryPostgres(repo *postgres.NodeRepository) *NodeRepositoryPostgres {
	if repo == nil {
		panic("NewNodeRepositoryPostgres: repo is nil")
	}
	return &NodeRepositoryPostgres{repo: repo}
}

var _ port.NodeRepository = (*NodeRepositoryPostgres)(nil)

func (r *NodeRepositoryPostgres) TenantIDForNode(ctx context.Context, nodeID string) (string, error) {
	return r.repo.TenantIDByNodeID(ctx, nodeID)
}

func (r *NodeRepositoryPostgres) GetByID(ctx context.Context, tenantID, id string) (*model.Node, error) {
	row, err := r.repo.FindNodeByID(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return nodeRowToDomain(row)
}

func (r *NodeRepositoryPostgres) Exists(ctx context.Context, tenantID, id string) (bool, error) {
	return r.repo.ExistsNode(ctx, tenantID, id)
}

func (r *NodeRepositoryPostgres) Move(ctx context.Context, tenantID string, id string, newParentID string) error {
	return r.repo.RunInTx(func(tx *gorm.DB) error {
		n, err := r.repo.FindNodeByIDForUpdate(ctx, tx, tenantID, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNodeNotFound
			}
			return err
		}

		oldPath := strings.TrimSpace(n.Path)
		if oldPath == "" {
			oldPath = n.ID
		}

		var newParent *postgres.NodeModel
		parentUUID := strings.TrimSpace(newParentID)
		if parentUUID != "" {
			if parentUUID == id {
				return errors.New("cannot move node under itself")
			}
			p, err := r.repo.FindNodeByIDTx(ctx, tx, tenantID, parentUUID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrNodeNotFound
				}
				return err
			}
			pPath := strings.TrimSpace(p.Path)
			if pPath == "" {
				pPath = p.ID
			}
			if strings.HasPrefix(pPath, oldPath+"/") || pPath == oldPath {
				return errors.New("cannot move node under its descendant")
			}
			newParent = p
		}

		newPath, newDepth := computeNodePathDepth(n.ID, newParent)
		depthDelta := newDepth - n.Depth

		desc, err := r.repo.FindDescendantsTx(ctx, tx, tenantID, oldPath, id)
		if err != nil {
			return err
		}
		for i := range desc {
			suffix := strings.TrimPrefix(desc[i].Path, oldPath)
			desc[i].Path = newPath + suffix
			desc[i].Depth = desc[i].Depth + depthDelta
			desc[i].Version++
			if err := r.repo.UpdateNodeFieldsTx(ctx, tx, tenantID, desc[i].ID, map[string]any{
				"path":       desc[i].Path,
				"depth":      desc[i].Depth,
				"version":    desc[i].Version,
				"updated_at": gorm.Expr("NOW()"),
			}); err != nil {
				return err
			}
		}

		var parentID *string
		if parentUUID != "" {
			parentID = &parentUUID
		}
		return r.repo.UpdateNodeFieldsTx(ctx, tx, tenantID, id, map[string]any{
			"parent_id":  parentID,
			"path":       newPath,
			"depth":      newDepth,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("NOW()"),
		})
	})
}

func (r *NodeRepositoryPostgres) UpdateTopology(ctx context.Context, tenantID string, id string, path string, depth int) error {
	err := r.repo.UpdateNodeFields(ctx, tenantID, id, map[string]any{
		"path":       path,
		"depth":      depth,
		"version":    gorm.Expr("version + 1"),
		"updated_at": gorm.Expr("NOW()"),
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNodeNotFound
	}
	return err
}

func (r *NodeRepositoryPostgres) ListChildren(ctx context.Context, tenantID, parentID string) (*port.PageResult[*model.Node], error) {
	rows, err := r.repo.ListChildren(ctx, tenantID, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Node, 0, len(rows))
	for i := range rows {
		n, err := nodeRowToDomain(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return nodeListPageResult(out), nil
}

func (r *NodeRepositoryPostgres) ListDescendants(ctx context.Context, tenantID, rootID string) (*port.PageResult[*model.Node], error) {
	root, err := r.getNodeRow(ctx, tenantID, rootID)
	if err != nil {
		return nil, err
	}
	basePath := strings.TrimSpace(root.Path)
	if basePath == "" {
		basePath = root.ID
	}
	rows, err := r.repo.FindDescendants(ctx, tenantID, basePath)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Node, 0, len(rows))
	for i := range rows {
		n, err := nodeRowToDomain(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return nodeListPageResult(out), nil
}

func (r *NodeRepositoryPostgres) GetAncestors(ctx context.Context, tenantID, id string) (*port.PageResult[*model.Node], error) {
	chain, err := r.repo.GetAncestorChain(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNodeNotFound
		}
		return nil, err
	}
	out := make([]*model.Node, 0, len(chain))
	for _, row := range chain {
		n, err := nodeRowToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return nodeListPageResult(out), nil
}

func nodeListPageResult(nodes []*model.Node) *port.PageResult[*model.Node] {
	n := len(nodes)
	return &port.PageResult[*model.Node]{
		Items:      nodes,
		TotalCount: int64(n),
		Offset:     0,
		Limit:      n,
	}
}

func (r *NodeRepositoryPostgres) UpdateDisplayName(ctx context.Context, tenantID, id string, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("display name cannot be empty")
	}
	err := r.repo.UpdateNodeFields(ctx, tenantID, id, map[string]any{
		"display_name": newName,
		"version":      gorm.Expr("version + 1"),
		"updated_at":   gorm.Expr("NOW()"),
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNodeNotFound
	}
	return err
}

func (r *NodeRepositoryPostgres) UpdateStatus(ctx context.Context, tenantID, id string, status model.NodeLifecycleStatus) error {
	err := r.repo.UpdateNodeFields(ctx, tenantID, id, map[string]any{
		"lifecycle_status": string(status),
		"version":          gorm.Expr("version + 1"),
		"updated_at":       gorm.Expr("NOW()"),
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNodeNotFound
	}
	return err
}

func (r *NodeRepositoryPostgres) SoftDelete(ctx context.Context, tenantID, id string) error {
	err := r.repo.SoftDeleteNode(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNodeNotFound
	}
	return err
}

func (r *NodeRepositoryPostgres) SoftDeleteSubtree(ctx context.Context, tenantID, rootID string) error {
	root, err := r.getNodeRow(ctx, tenantID, rootID)
	if err != nil {
		return err
	}
	basePath := strings.TrimSpace(root.Path)
	if basePath == "" {
		basePath = root.ID
	}
	return r.repo.SoftDeleteSubtree(ctx, tenantID, rootID, basePath)
}

func (r *NodeRepositoryPostgres) getNodeRow(ctx context.Context, tenantID, id string) (*postgres.NodeModel, error) {
	row, err := r.repo.FindNodeByID(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func nodeRowToDomain(row *postgres.NodeModel) (*model.Node, error) {
	var meta map[string]any
	if len(row.Metadata) > 0 && string(row.Metadata) != "null" {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return nil, err
		}
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	var deletedAt *time.Time
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		deletedAt = &t
	}
	return &model.Node{
		ID:              row.ID,
		TenantID:        row.TenantID,
		ParentID:        row.ParentID,
		DisplayName:     row.DisplayName,
		Type:            row.Type,
		SubType:         row.SubType,
		LifecycleStatus: model.NodeLifecycleStatus(row.LifecycleStatus),
		Description:     row.Description,
		Path:            row.Path,
		Depth:           row.Depth,
		Metadata:        meta,
		Version:         row.Version,
		DeletedAt:       deletedAt,
		DeletedBy:       row.DeletedBy,
		DeleteJobID:     row.DeleteJobID,
		DeleteReason:    row.DeleteReason,
		RestoredAt:      row.RestoredAt,
		RestoredBy:      row.RestoredBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}
