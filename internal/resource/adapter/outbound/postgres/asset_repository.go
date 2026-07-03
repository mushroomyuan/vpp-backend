package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssetRepositoryPostgres struct {
	repo     *postgres.AssetRepository
	nodeRepo *postgres.NodeRepository
}

func NewAssetRepositoryPostgres(repo *postgres.AssetRepository, nodeRepo *postgres.NodeRepository) *AssetRepositoryPostgres {
	if repo == nil || nodeRepo == nil {
		panic("NewAssetRepositoryPostgres: repo and nodeRepo are required")
	}
	return &AssetRepositoryPostgres{repo: repo, nodeRepo: nodeRepo}
}

var _ port.AssetRepository = (*AssetRepositoryPostgres)(nil)

func (r *AssetRepositoryPostgres) Create(ctx context.Context, a *model.Asset) (*model.Asset, error) {
	err := r.nodeRepo.RunInTx(func(tx *gorm.DB) error {
		if err := prepareNodePathDepthForInsert(ctx, tx, r.nodeRepo, &a.Node); err != nil {
			return err
		}
		nm, err := NodeDomainToDB(&a.Node)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(nm).Error; err != nil {
			return err
		}
		arow, err := AssetDomainToDB(a)
		if err != nil {
			return err
		}
		return tx.WithContext(ctx).Clauses(clause.Returning{}).Create(arow).Error
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, a.TenantID, a.ID)
}

func (r *AssetRepositoryPostgres) BatchCreate(ctx context.Context, assets []*model.Asset) error {
	return r.nodeRepo.RunInTx(func(tx *gorm.DB) error {
		for _, a := range assets {
			if err := prepareNodePathDepthForInsert(ctx, tx, r.nodeRepo, &a.Node); err != nil {
				return err
			}
			nm, err := NodeDomainToDB(&a.Node)
			if err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(nm).Error; err != nil {
				return err
			}
			arow, err := AssetDomainToDB(a)
			if err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(arow).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AssetRepositoryPostgres) Update(ctx context.Context, a *model.Asset) error {
	row, err := AssetDomainToDB(a)
	if err != nil {
		return err
	}
	if err := r.repo.UpdateAsset(ctx, row); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrResourceNotFound
		}
		return err
	}
	nodeUpdates := map[string]any{
		"display_name": a.DisplayName,
		"version":      gorm.Expr("version + 1"),
		"updated_at":   gorm.Expr("NOW()"),
	}
	if a.Description != nil {
		nodeUpdates["description"] = a.Description
	} else {
		nodeUpdates["description"] = nil
	}
	if a.SubType != nil {
		nodeUpdates["sub_type"] = a.SubType
	} else {
		nodeUpdates["sub_type"] = nil
	}
	if err := r.nodeRepo.UpdateNodeFields(ctx, a.TenantID, a.ID, nodeUpdates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrResourceNotFound
		}
		return err
	}
	return nil
}

func (r *AssetRepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.Asset, error) {
	arow, err := r.repo.FindAssetByID(ctx,
		builder.NewAsset().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	nrow, err := r.nodeRepo.FindNodeByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return AssetToDomain(nrow, arow)
}

func (r *AssetRepositoryPostgres) List(ctx context.Context, f port.AssetFilter) (*port.PageResult[*model.Asset], error) {
	q := builder.NewAsset().
		TenantID(f.TenantID).
		SiteID(f.SiteID).
		IDs(f.IDs...).
		Types(f.Types...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, totalCount, err := r.repo.ListAssets(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &port.PageResult[*model.Asset]{
			Items:      []*model.Asset{},
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
	out := make([]*model.Asset, 0, len(rows))
	for _, row := range rows {
		nm, ok := byID[row.NodeID]
		if !ok {
			return nil, fmt.Errorf("asset %s: node row missing", row.NodeID)
		}
		a, err := AssetToDomain(nm, row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return &port.PageResult[*model.Asset]{
		Items:      out,
		TotalCount: totalCount,
		Offset:     f.Offset,
		Limit:      f.Limit,
	}, nil
}
