package adapters

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CURepositoryPostgres struct {
	repo     *postgres.CURepository
	nodeRepo *postgres.NodeRepository
}

func NewCURepositoryPostgres(repo *postgres.CURepository, nodeRepo *postgres.NodeRepository) *CURepositoryPostgres {
	if repo == nil || nodeRepo == nil {
		panic("NewCURepositoryPostgres: repo and nodeRepo are required")
	}
	return &CURepositoryPostgres{repo: repo, nodeRepo: nodeRepo}
}

var _ port.CURepository = (*CURepositoryPostgres)(nil)

func (r *CURepositoryPostgres) Create(ctx context.Context, cu *model.CU) (*model.CU, error) {
	err := r.nodeRepo.RunInTx(func(tx *gorm.DB) error {
		if err := prepareNodePathDepthForInsert(ctx, tx, r.nodeRepo, &cu.Node); err != nil {
			return err
		}
		nm, err := NodeDomainToDB(&cu.Node)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(nm).Error; err != nil {
			return err
		}
		crow, err := CUDomainToDB(cu)
		if err != nil {
			return err
		}
		return tx.WithContext(ctx).Clauses(clause.Returning{}).Create(crow).Error
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, cu.TenantID, cu.ID)
}

func (r *CURepositoryPostgres) BatchCreate(ctx context.Context, cus []*model.CU) error {
	return r.nodeRepo.RunInTx(func(tx *gorm.DB) error {
		for _, cu := range cus {
			if err := prepareNodePathDepthForInsert(ctx, tx, r.nodeRepo, &cu.Node); err != nil {
				return err
			}
			nm, err := NodeDomainToDB(&cu.Node)
			if err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(nm).Error; err != nil {
				return err
			}
			crow, err := CUDomainToDB(cu)
			if err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(crow).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *CURepositoryPostgres) Update(ctx context.Context, cu *model.CU) error {
	row, err := CUDomainToDB(cu)
	if err != nil {
		return err
	}
	if err := r.repo.UpdateCU(ctx, row); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrCUNotFound
		}
		return err
	}
	nodeUpdates := map[string]any{
		"display_name": cu.DisplayName,
		"version":      gorm.Expr("version + 1"),
		"updated_at":   gorm.Expr("NOW()"),
	}
	if cu.Description != nil {
		nodeUpdates["description"] = cu.Description
	} else {
		nodeUpdates["description"] = nil
	}
	if cu.SubType != nil {
		nodeUpdates["sub_type"] = cu.SubType
	} else {
		nodeUpdates["sub_type"] = nil
	}
	if err := r.nodeRepo.UpdateNodeFields(ctx, cu.TenantID, cu.ID, nodeUpdates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrCUNotFound
		}
		return err
	}
	return nil
}

func (r *CURepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.CU, error) {
	crow, err := r.repo.FindCUByID(ctx,
		builder.NewCU().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrCUNotFound
	}
	if err != nil {
		return nil, err
	}
	nrow, err := r.nodeRepo.FindNodeByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return CUToDomain(nrow, crow)
}

func (r *CURepositoryPostgres) List(ctx context.Context, f port.CUFilter) (*port.PageResult[*model.CU], error) {
	q := builder.NewCU().
		TenantID(f.TenantID).
		SiteID(f.SiteID).
		AssetID(f.AssetID).
		IDs(f.IDs...).
		Capabilities(f.CapabilityTags...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, totalCount, err := r.repo.ListCUs(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &port.PageResult[*model.CU]{
			Items:      []*model.CU{},
			TotalCount: totalCount,
			Offset:     f.Offset,
			Limit:      f.Limit,
		}, nil
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.NodeID
	}
	nodeRows, err := r.nodeRepo.ListByIDs(ctx, f.TenantID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*postgres.NodeModel, len(nodeRows))
	for _, n := range nodeRows {
		byID[n.ID] = n
	}
	items, err := BatchCUToDomain(rows, byID)
	if err != nil {
		return nil, err
	}
	return &port.PageResult[*model.CU]{
		Items:      items,
		TotalCount: totalCount,
		Offset:     f.Offset,
		Limit:      f.Limit,
	}, nil
}
