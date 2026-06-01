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
)

type PointRepositoryPostgres struct {
	repo     *postgres.PointRepository
	nodeRepo *postgres.NodeRepository
}

func NewPointRepositoryPostgres(repo *postgres.PointRepository, nodeRepo *postgres.NodeRepository) *PointRepositoryPostgres {
	if repo == nil || nodeRepo == nil {
		panic("NewPointRepositoryPostgres: repo and nodeRepo are required")
	}
	return &PointRepositoryPostgres{repo: repo, nodeRepo: nodeRepo}
}

var _ port.PointRepository = (*PointRepositoryPostgres)(nil)

func (r *PointRepositoryPostgres) Create(ctx context.Context, p *model.Point) (*model.Point, error) {
	pcopy := *p
	if pcopy.TenantID == "" {
		tid, err := r.nodeRepo.TenantIDByNodeID(ctx, pcopy.AssetID)
		if err != nil {
			return nil, err
		}
		pcopy.TenantID = tid
	}
	row, err := PointDomainToDB(&pcopy)
	if err != nil {
		return nil, err
	}
	if err := r.repo.CreatePoint(ctx, row); err != nil {
		return nil, err
	}
	return PointDBToDomain(row)
}

func (r *PointRepositoryPostgres) BatchCreate(ctx context.Context, points []*model.Point) error {
	rows := make([]*postgres.PointModel, 0, len(points))
	for _, p := range points {
		pcopy := *p
		if pcopy.TenantID == "" {
			tid, err := r.nodeRepo.TenantIDByNodeID(ctx, pcopy.AssetID)
			if err != nil {
				return err
			}
			pcopy.TenantID = tid
		}
		row, err := PointDomainToDB(&pcopy)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return r.repo.BatchCreatePoint(ctx, rows)
}

func (r *PointRepositoryPostgres) Update(ctx context.Context, p *model.Point) error {
	row, err := PointDomainToDB(p)
	if err != nil {
		return err
	}
	err = r.repo.UpdatePoint(ctx, row)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrPointNotFound
	}
	return err
}

func (r *PointRepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.Point, error) {
	q := builder.NewPoint().IDs(id)
	if tenantID != "" {
		q = q.TenantID(tenantID)
	}
	row, err := r.repo.FindPointByID(ctx, q)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrPointNotFound
	}
	if err != nil {
		return nil, err
	}
	return PointDBToDomain(row)
}

func (r *PointRepositoryPostgres) List(ctx context.Context, f port.PointFilter) (*port.PageResult[*model.Point], error) {
	q := builder.NewPoint().
		TenantID(f.TenantID).
		SiteID(f.SiteID).
		CUID(f.CUID).
		IDs(f.IDs...).
		PointKeys(f.PointKeys...).
		DataTypes(f.DataTypes...).
		Paginate(f.Limit, f.Offset)
	if f.IsVirtual != nil {
		q = q.IsVirtual(*f.IsVirtual)
	}
	rows, totalCount, err := r.repo.ListPoints(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &port.PageResult[*model.Point]{
			Items:      []*model.Point{},
			TotalCount: totalCount,
			Offset:     f.Offset,
			Limit:      f.Limit,
		}, nil
	}
	items, err := BatchPointDBToDomain(rows)
	if err != nil {
		return nil, err
	}
	return &port.PageResult[*model.Point]{
		Items:      items,
		TotalCount: totalCount,
		Offset:     f.Offset,
		Limit:      f.Limit,
	}, nil
}

func (r *PointRepositoryPostgres) SoftDelete(ctx context.Context, tenantID, id string) error {
	q := builder.NewPoint().IDs(id)
	if tenantID != "" {
		q = q.TenantID(tenantID)
	}
	err := r.repo.SoftDeletePoint(ctx, q)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrPointNotFound
	}
	return err
}

func (r *PointRepositoryPostgres) BatchDelete(ctx context.Context, tenantID string, ids []string) error {
	return r.repo.BatchDeletePoint(ctx, tenantID, ids)
}
