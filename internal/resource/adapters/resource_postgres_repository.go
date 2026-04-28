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

type ResourceRepositoryPostgres struct {
	repo *postgres.ResourceRepository
}

func NewResourceRepositoryPostgres(repo *postgres.ResourceRepository) *ResourceRepositoryPostgres {
	return &ResourceRepositoryPostgres{repo: repo}
}

var _ port.ResourceRepository = (*ResourceRepositoryPostgres)(nil)

func (r *ResourceRepositoryPostgres) Create(ctx context.Context, resource *model.Resource) (*model.Resource, error) {
	row, err := ResourceDomainToDB(resource)
	if err != nil {
		return nil, err
	}
	if err := r.repo.CreateResource(ctx, row); err != nil {
		return nil, err
	}
	res, err := ResourceDBToDomain(row)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *ResourceRepositoryPostgres) BatchCreate(ctx context.Context, resources []*model.Resource) error {
	rows := make([]*postgres.ResourceModel, 0, len(resources))
	for _, res := range resources {
		row, err := ResourceDomainToDB(res)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return r.repo.BatchCreateResource(ctx, rows)
}

func (r *ResourceRepositoryPostgres) Update(ctx context.Context, resource *model.Resource) error {
	row, err := ResourceDomainToDB(resource)
	if err != nil {
		return err
	}
	err = r.repo.UpdateResource(ctx, row)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrResourceNotFound
	}
	return err
}

func (r *ResourceRepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.Resource, error) {
	row, err := r.repo.FindResourceByID(ctx,
		builder.NewResource().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	return ResourceDBToDomain(row)
}

func (r *ResourceRepositoryPostgres) List(ctx context.Context, f port.ResourceFilter) ([]*model.Resource, error) {
	q := builder.NewResource().
		TenantID(f.TenantID).
		SiteID(f.SiteID).
		IDs(f.IDs...).
		Types(f.Types...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, err := r.repo.ListResources(ctx, q)
	if err != nil {
		return nil, err
	}
	return BatchResourceDBToDomain(rows)
}

func (r *ResourceRepositoryPostgres) SoftDelete(ctx context.Context, tenantID, id string) error {
	err := r.repo.SoftDeleteResource(ctx,
		builder.NewResource().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrResourceNotFound
	}
	return err
}

func (r *ResourceRepositoryPostgres) BatchDelete(ctx context.Context, tenantID string, ids []string) error {
	return r.repo.BatchDeleteResource(ctx, tenantID, ids)
}
