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

type CURepositoryPostgres struct {
	repo *postgres.CURepository
}

func NewCURepositoryPostgres(repo *postgres.CURepository) *CURepositoryPostgres {
	return &CURepositoryPostgres{repo: repo}
}

var _ port.CURepository = (*CURepositoryPostgres)(nil)

func (r *CURepositoryPostgres) Create(ctx context.Context, cu *model.CU) (*model.CU, error) {
	row, err := CUDomainToDB(cu)
	if err != nil {
		return nil, err
	}
	if err := r.repo.CreateCU(ctx, row); err != nil {
		return nil, err
	}
	res, err := CUDBToDomain(row)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *CURepositoryPostgres) BatchCreate(ctx context.Context, cus []*model.CU) error {
	rows := make([]*postgres.CUModel, 0, len(cus))
	for _, cu := range cus {
		row, err := CUDomainToDB(cu)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return r.repo.BatchCreateCU(ctx, rows)
}

func (r *CURepositoryPostgres) Update(ctx context.Context, cu *model.CU) error {
	row, err := CUDomainToDB(cu)
	if err != nil {
		return err
	}
	err = r.repo.UpdateCU(ctx, row)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCUNotFound
	}
	return err
}

func (r *CURepositoryPostgres) FindByID(ctx context.Context, id string) (*model.CU, error) {
	row, err := r.repo.FindCUByID(ctx,
		builder.NewCU().IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrCUNotFound
	}
	if err != nil {
		return nil, err
	}
	return CUDBToDomain(row)
}

func (r *CURepositoryPostgres) List(ctx context.Context, f port.CUFilter) ([]*model.CU, error) {
	q := builder.NewCU().
		TenantID(f.TenantID).
		SiteID(f.SiteID).
		ResourceID(f.ResourceID).
		ParentCUID(f.ParentCUID).
		IDs(f.IDs...).
		Capabilities(f.Capability...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, err := r.repo.ListCUs(ctx, q)
	if err != nil {
		return nil, err
	}
	return BatchCUDBToDomain(rows)
}

func (r *CURepositoryPostgres) SoftDelete(ctx context.Context, id string) error {
	err := r.repo.SoftDeleteCU(ctx, builder.NewCU().IDs(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCUNotFound
	}
	return err
}

func (r *CURepositoryPostgres) BatchDelete(ctx context.Context, tenantID string, ids []string) error {
	return r.repo.BatchDeleteCU(ctx, tenantID, ids)
}
