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

type SiteRepositoryPostgres struct {
	siteRepo *postgres.SiteRepository
	nodeRepo *postgres.NodeRepository
}

func NewSiteRepositoryPostgres(siteRepo *postgres.SiteRepository, nodeRepo *postgres.NodeRepository) *SiteRepositoryPostgres {
	if siteRepo == nil || nodeRepo == nil {
		panic("NewSiteRepositoryPostgres: siteRepo and nodeRepo are required")
	}
	return &SiteRepositoryPostgres{siteRepo: siteRepo, nodeRepo: nodeRepo}
}

var _ port.SiteRepository = (*SiteRepositoryPostgres)(nil)

func (r *SiteRepositoryPostgres) Create(ctx context.Context, s *model.Site) (*model.Site, error) {
	err := r.nodeRepo.RunInTx(func(tx *gorm.DB) error {
		if err := prepareNodePathDepthForInsert(ctx, tx, r.nodeRepo, &s.Node); err != nil {
			return err
		}
		nm, err := NodeDomainToDB(&s.Node)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Clauses(clause.Returning{}).Create(nm).Error; err != nil {
			return err
		}
		srow, err := SiteDomainToDB(s)
		if err != nil {
			return err
		}
		return tx.WithContext(ctx).Clauses(clause.Returning{}).Create(srow).Error
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, s.TenantID, s.ID)
}

func (r *SiteRepositoryPostgres) Update(ctx context.Context, s *model.Site) error {
	row, err := SiteDomainToDB(s)
	if err != nil {
		return err
	}
	if err := r.siteRepo.UpdateSite(ctx, row); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrSiteNotFound
		}
		return err
	}
	nodeUpdates := map[string]any{
		"display_name": s.DisplayName,
		"version":      gorm.Expr("version + 1"),
		"updated_at":   gorm.Expr("NOW()"),
	}
	if s.Description != nil {
		nodeUpdates["description"] = s.Description
	} else {
		nodeUpdates["description"] = nil
	}
	if err := r.nodeRepo.UpdateNodeFields(ctx, s.TenantID, s.ID, nodeUpdates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrSiteNotFound
		}
		return err
	}
	return nil
}

func (r *SiteRepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.Site, error) {
	srow, err := r.siteRepo.FindSiteByID(ctx,
		builder.NewSite().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSiteNotFound
	}
	if err != nil {
		return nil, err
	}
	nrow, err := r.nodeRepo.FindNodeByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return SiteToDomain(nrow, srow)
}

func (r *SiteRepositoryPostgres) List(ctx context.Context, f port.SiteFilter) (*port.PageResult[*model.Site], error) {
	q := builder.NewSite().
		TenantID(f.TenantID).
		IDs(f.IDs...).
		StatusNames(f.Status...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, totalCount, err := r.siteRepo.ListSites(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &port.PageResult[*model.Site]{
			Items:      []*model.Site{},
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
	out := make([]*model.Site, 0, len(rows))
	for _, row := range rows {
		nm, ok := byID[row.NodeID]
		if !ok {
			return nil, fmt.Errorf("site %s: node row missing", row.NodeID)
		}
		s, err := SiteToDomain(nm, row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return &port.PageResult[*model.Site]{
		Items:      out,
		TotalCount: totalCount,
		Offset:     f.Offset,
		Limit:      f.Limit,
	}, nil
}

// func (r *SiteRepositoryPostgres) SoftDelete(ctx context.Context, tenantID, id string) error {
// 	errSite := r.siteRepo.SoftDeleteSite(ctx,
// 		builder.NewSite().TenantID(tenantID).IDs(id),
// 	)
// 	if errSite != nil && !errors.Is(errSite, gorm.ErrRecordNotFound) {
// 		return errSite
// 	}
// 	err := r.nodeRepo.SoftDeleteNode(ctx, tenantID, id)
// 	if errors.Is(err, gorm.ErrRecordNotFound) {
// 		return domain.ErrSiteNotFound
// 	}
// 	return err
// }
