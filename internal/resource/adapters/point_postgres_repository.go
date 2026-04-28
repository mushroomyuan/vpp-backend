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
	repo *postgres.PointRepository
}

func NewPointRepositoryPostgres(repo *postgres.PointRepository) *PointRepositoryPostgres {
	return &PointRepositoryPostgres{repo: repo}
}

var _ port.PointRepository = (*PointRepositoryPostgres)(nil)

func (r *PointRepositoryPostgres) Create(ctx context.Context, p *model.Point) (*model.Point, error) {
	row, err := PointDomainToDB(p)
	if err != nil {
		return nil, err
	}
	if err := r.repo.CreatePoint(ctx, row); err != nil {
		return nil, err
	}
	res, err := PointDBToDomain(row)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *PointRepositoryPostgres) BatchCreate(ctx context.Context, points []*model.Point) error {
	rows := make([]*postgres.PointModel, 0, len(points))
	for _, p := range points {
		row, err := PointDomainToDB(p)
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

func (r *PointRepositoryPostgres) FindByID(ctx context.Context, id string) (*model.Point, error) {
	row, err := r.repo.FindPointByID(ctx,
		builder.NewPoint().IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrPointNotFound
	}
	if err != nil {
		return nil, err
	}
	return PointDBToDomain(row)
}

func (r *PointRepositoryPostgres) List(ctx context.Context, f port.PointFilter) ([]*model.Point, error) {
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
	rows, err := r.repo.ListPoints(ctx, q)
	if err != nil {
		return nil, err
	}
	return BatchPointDBToDomain(rows)
}

func (r *PointRepositoryPostgres) SoftDelete(ctx context.Context, id string) error {
	err := r.repo.SoftDeletePoint(ctx, builder.NewPoint().IDs(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrPointNotFound
	}
	return err
}

func (r *PointRepositoryPostgres) BatchDelete(ctx context.Context, tenantID string, ids []string) error {
	return r.repo.BatchDeletePoint(ctx, tenantID, ids)
}
